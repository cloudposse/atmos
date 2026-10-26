// Package claudecode provides an AI provider that invokes the Claude Code CLI
// as a subprocess, reusing the user's Claude Pro/Max subscription instead of
// requiring separate API tokens.
package claudecode

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/agent/base"
	"github.com/cloudposse/atmos/pkg/ai/approval"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/ai/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	mcpclient "github.com/cloudposse/atmos/pkg/mcp/client"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

const (
	// ProviderName is the name of this provider for configuration lookup.
	ProviderName = "claude-code"
	// DefaultBinary is the default binary name for Claude Code.
	DefaultBinary = "claude"
	// DefaultMaxTurns is the default maximum agentic turns per invocation.
	DefaultMaxTurns = 5

	// The askUserQuestionGuidance constant tells the model to use the AskUserQuestion tool for choices.
	askUserQuestionGuidance = "When you need the user to choose between options or to confirm something before you " +
		"continue, use the AskUserQuestion tool instead of asking in prose."
)

// Client invokes the Claude Code CLI in non-interactive mode.
type Client struct {
	binaryPath    string
	maxTurns      int
	maxBudget     float64
	allowedTools  []string
	model         string
	mcpServers    map[string]schema.MCPServerConfig // MCP servers to pass through via --mcp-config.
	toolchainPATH string                            // Toolchain bin PATH for MCP server subprocesses.
	mcpConfigPath string                            // MCP config file path for the invocation in progress.
	approver      approval.Approver                 // Optional; asked before tools that need permission.
	progress      func(approval.Event)              // Optional; receives tool progress events.
	timeout       time.Duration                     // Run timeout excluding approval waits; 0 = none.
}

var (
	_ approval.ProgressReporter = (*Client)(nil)
	_ approval.Approvable       = (*Client)(nil)
	_ approval.TimeoutManaged   = (*Client)(nil)
)

// NewClient creates a new Claude Code CLI client from Atmos configuration.
func NewClient(atmosConfig *schema.AtmosConfiguration) (*Client, error) {
	defer perf.Track(atmosConfig, "claudecode.NewClient")()

	config := base.ExtractConfig(atmosConfig, ProviderName, base.ProviderDefaults{
		Model: ProviderName,
	})

	if !config.Enabled {
		return nil, errUtils.ErrAIDisabledInConfiguration
	}

	providerConfig := base.GetProviderConfig(atmosConfig, ProviderName)

	client := &Client{
		maxTurns: DefaultMaxTurns,
		model:    config.Model,
	}

	// Apply provider-specific settings.
	applyProviderConfig(client, providerConfig)

	// Resolve binary path.
	if client.binaryPath == "" {
		resolved, err := exec.LookPath(DefaultBinary)
		if err != nil {
			return nil, errUtils.Build(errUtils.ErrCLIProviderBinaryNotFound).
				WithContext("provider", ProviderName).
				WithContext("binary", DefaultBinary).
				WithHint("Install Claude Code: brew install --cask claude-code").
				Err()
		}
		client.binaryPath = resolved
	}

	// Capture MCP servers for pass-through (only if configured).
	if len(atmosConfig.MCP.Servers) > 0 {
		client.mcpServers = atmosConfig.MCP.Servers
		client.toolchainPATH = base.ResolveToolchainPATH(atmosConfig)
		ui.Info(fmt.Sprintf("MCP servers configured: %d", len(client.mcpServers)))
	}

	return client, nil
}

// SendMessage sends a prompt to Claude Code and returns the response.
func (c *Client) SendMessage(ctx context.Context, message string) (string, error) {
	defer perf.Track(nil, "claudecode.Client.SendMessage")()

	return c.execClaude(ctx, message, "")
}

// SendMessageWithTools is not supported — Claude Code manages its own tools.
func (c *Client) SendMessageWithTools(_ context.Context, _ string, _ []tools.Tool) (*types.Response, error) {
	return nil, errUtils.ErrCLIProviderToolsNotSupported
}

// SendMessageWithHistory concatenates history into a single prompt.
func (c *Client) SendMessageWithHistory(ctx context.Context, messages []types.Message) (string, error) {
	defer perf.Track(nil, "claudecode.Client.SendMessageWithHistory")()

	prompt := base.FormatMessagesAsPrompt(messages)
	return c.execClaude(ctx, prompt, "")
}

// SendMessageWithToolsAndHistory is not supported — Claude Code manages its own tools.
func (c *Client) SendMessageWithToolsAndHistory(_ context.Context, _ []types.Message, _ []tools.Tool) (*types.Response, error) {
	return nil, errUtils.ErrCLIProviderToolsNotSupported
}

// SendMessageWithSystemPromptAndTools sends with system prompt via --append-system-prompt.
func (c *Client) SendMessageWithSystemPromptAndTools(
	ctx context.Context,
	systemPrompt string,
	atmosMemory string,
	messages []types.Message,
	_ []tools.Tool,
) (*types.Response, error) {
	defer perf.Track(nil, "claudecode.Client.SendMessageWithSystemPromptAndTools")()

	prompt := base.FormatMessagesAsPrompt(messages)
	combined := systemPrompt
	if atmosMemory != "" {
		combined += "\n\n" + atmosMemory
	}

	result, err := c.execClaude(ctx, prompt, combined)
	if err != nil {
		return nil, err
	}

	return &types.Response{
		Content:    result,
		StopReason: types.StopReasonEndTurn,
	}, nil
}

// SendMessageWithSystemPrompt sends messages with a system prompt via --append-system-prompt, without tools.
func (c *Client) SendMessageWithSystemPrompt(ctx context.Context, systemPrompt string, messages []types.Message) (string, error) {
	defer perf.Track(nil, "claudecode.Client.SendMessageWithSystemPrompt")()

	return c.execClaude(ctx, base.FormatMessagesAsPrompt(messages), systemPrompt)
}

// GetModel returns the provider name.
func (c *Client) GetModel() string {
	return c.model
}

// GetMaxTokens returns 0 — managed by Claude Code internally.
func (c *Client) GetMaxTokens() int {
	return 0
}

// buildArgs constructs the CLI arguments for claude -p invocation.
//
// Output is always streamed as NDJSON (stream-json, which requires --verbose in -p mode) so
// tool progress can be reported. When an Approver is set, Claude additionally runs the SDK
// stdio control protocol (--input-format stream-json --permission-prompt-tool stdio): it asks
// Atmos before running a tool that needs permission and blocks until answered.
//
// --dangerously-skip-permissions accompanies --mcp-config only when no Approver is set, so
// MCP pass-through tools can run unattended. With an Approver, MCP tools go through the
// Atmos permission system instead.
func (c *Client) buildArgs(systemPrompt string) []string {
	args := []string{
		"-p",
		"--output-format", "stream-json",
		"--verbose",
		"--max-turns", strconv.Itoa(c.maxTurns),
	}

	if c.approver != nil {
		args = append(args, "--input-format", "stream-json", "--permission-prompt-tool", "stdio")
	}

	if c.maxBudget > 0 {
		args = append(args, "--max-budget-usd", fmt.Sprintf("%.2f", c.maxBudget))
	}

	// When the user can be asked, steer the model toward Claude Code's multiple-choice tool
	// instead of asking in prose, where a short reply like "yes" has no structure.
	if c.approver != nil {
		systemPrompt = strings.TrimSpace(systemPrompt + "\n\n" + askUserQuestionGuidance)
	}

	if systemPrompt != "" {
		args = append(args, "--append-system-prompt", systemPrompt)
	}

	for _, tool := range c.allowedTools {
		args = append(args, "--allowedTools", tool)
	}

	// MCP pass-through: use the config file written for this invocation.
	if c.mcpConfigPath != "" {
		args = append(args, "--mcp-config", c.mcpConfigPath)
		if c.approver == nil {
			args = append(args, "--dangerously-skip-permissions")
		}
	}

	return args
}

// prepareMCPConfig writes the MCP config file for one invocation and returns a function that
// removes it again. The file can hold environment values and headers, so it never outlives the run.
func (c *Client) prepareMCPConfig() func() {
	if len(c.mcpServers) == 0 {
		return func() {}
	}
	path, err := mcpclient.WriteMCPConfigToTempFile(c.mcpServers, c.toolchainPATH)
	if err != nil {
		log.Debug("Failed to generate MCP config for Claude Code", "error", err)
		return func() {}
	}
	c.mcpConfigPath = path
	return func() {
		_ = os.Remove(path)
		c.mcpConfigPath = ""
	}
}

// SetProgressHandler registers a callback for tool progress events. The callback runs on the
// goroutine that called SendMessage*, never concurrently with it.
func (c *Client) SetProgressHandler(handler func(approval.Event)) {
	c.progress = handler
}

// SetApprover registers the approver consulted before Claude runs a tool that needs permission.
// Passing nil restores unattended operation.
func (c *Client) SetApprover(approver approval.Approver) {
	c.approver = approver
}

// SetTimeout sets the run timeout. Time spent inside the approver does not count. Zero disables
// the timeout (the caller's context still applies).
func (c *Client) SetTimeout(timeout time.Duration) {
	c.timeout = timeout
}

// applyProviderConfig applies provider-specific settings to the client.
func applyProviderConfig(client *Client, providerConfig *schema.AIProviderConfig) {
	if providerConfig == nil {
		return
	}
	if providerConfig.Binary != "" {
		client.binaryPath = providerConfig.Binary
	}
	if providerConfig.MaxTurns > 0 {
		client.maxTurns = providerConfig.MaxTurns
	}
	if providerConfig.MaxBudgetUSD > 0 {
		client.maxBudget = providerConfig.MaxBudgetUSD
	}
	if len(providerConfig.AllowedTools) > 0 {
		client.allowedTools = providerConfig.AllowedTools
	}
}
