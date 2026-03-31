// Package claudecode provides an AI provider that invokes the Claude Code CLI
// as a subprocess, reusing the user's Claude Pro/Max subscription instead of
// requiring separate API tokens.
package claudecode

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/agent/base"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/ai/types"
	"github.com/cloudposse/atmos/pkg/dependencies"
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
	mcpConfigPath string                            // Pre-generated MCP config file path.
}

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
				WithHint(fmt.Sprintf("Install Claude Code: brew install %s", DefaultBinary)).
				Err()
		}
		client.binaryPath = resolved
	}

	// Capture MCP servers for pass-through (only if configured).
	if len(atmosConfig.MCP.Servers) > 0 {
		client.mcpServers = atmosConfig.MCP.Servers
		client.toolchainPATH = resolveToolchainPATH(atmosConfig)
		// Pre-generate MCP config so we can show the path before "Thinking...".
		mcpConfigPath, mcpErr := mcpclient.WriteMCPConfigToTempFile(client.mcpServers, client.toolchainPATH)
		if mcpErr != nil {
			log.Debug("Failed to generate MCP config for Claude Code", "error", mcpErr)
		} else {
			client.mcpConfigPath = mcpConfigPath
			ui.Info(fmt.Sprintf("MCP servers configured: %d (config: %s)", len(client.mcpServers), mcpConfigPath))
		}
	}

	return client, nil
}

// resolveToolchainPATH extracts the toolchain bin PATH for MCP server subprocesses.
func resolveToolchainPATH(atmosConfig *schema.AtmosConfiguration) string {
	deps, err := dependencies.LoadToolVersionsDependencies(atmosConfig)
	if err != nil || len(deps) == 0 {
		return ""
	}
	tenv, err := dependencies.NewEnvironmentFromDeps(atmosConfig, deps)
	if err != nil || tenv == nil {
		return ""
	}
	// Extract PATH from toolchain env vars.
	for _, envVar := range tenv.EnvVars() {
		if strings.HasPrefix(envVar, "PATH=") {
			return envVar[len("PATH="):]
		}
	}
	return ""
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

	prompt := formatMessages(messages)
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

	prompt := formatMessages(messages)
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

// GetModel returns the provider name.
func (c *Client) GetModel() string {
	return c.model
}

// GetMaxTokens returns 0 — managed by Claude Code internally.
func (c *Client) GetMaxTokens() int {
	return 0
}

// execClaude runs the claude CLI and returns the result text.
func (c *Client) execClaude(ctx context.Context, prompt, systemPrompt string) (string, error) {
	args := []string{
		"-p",
		"--output-format", "json",
		"--max-turns", strconv.Itoa(c.maxTurns),
	}

	if c.maxBudget > 0 {
		args = append(args, "--max-budget-usd", fmt.Sprintf("%.2f", c.maxBudget))
	}

	if systemPrompt != "" {
		args = append(args, "--append-system-prompt", systemPrompt)
	}

	for _, tool := range c.allowedTools {
		args = append(args, "--allowedTools", tool)
	}

	// MCP pass-through: use pre-generated config file.
	if c.mcpConfigPath != "" {
		args = append(args, "--mcp-config", c.mcpConfigPath)
	}

	cmd := exec.CommandContext(ctx, c.binaryPath, args...) //nolint:gosec // Binary path is from user config or exec.LookPath.
	cmd.Stdin = strings.NewReader(prompt)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		stderrStr := stderr.String()
		if stderrStr != "" {
			return "", fmt.Errorf("%w: %s: %s", errUtils.ErrCLIProviderExecFailed, ProviderName, stderrStr)
		}
		return "", fmt.Errorf("%w: %s: %w", errUtils.ErrCLIProviderExecFailed, ProviderName, err)
	}

	return parseResponse(stdout.Bytes())
}

// claudeResponse is the JSON output from `claude -p --output-format json`.
type claudeResponse struct {
	Type         string  `json:"type"`
	Subtype      string  `json:"subtype"`
	Result       string  `json:"result"`
	CostUSD      float64 `json:"cost_usd"`
	TotalCostUSD float64 `json:"total_cost_usd"`
	DurationMS   int     `json:"duration_ms"`
	IsError      bool    `json:"is_error"`
	SessionID    string  `json:"session_id"`
	NumTurns     int     `json:"num_turns"`
}

// parseResponse extracts the result text from Claude Code JSON output.
func parseResponse(output []byte) (string, error) {
	var resp claudeResponse
	if err := json.Unmarshal(output, &resp); err != nil {
		// If not valid JSON, return raw text (Claude Code may output plain text on some errors).
		trimmed := strings.TrimSpace(string(output))
		if trimmed != "" {
			return trimmed, nil
		}
		return "", fmt.Errorf("%w: %w", errUtils.ErrCLIProviderParseResponse, err)
	}

	if resp.IsError {
		return "", fmt.Errorf("%w: %s: %s", errUtils.ErrCLIProviderExecFailed, ProviderName, resp.Result)
	}

	return resp.Result, nil
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

// formatMessages concatenates conversation messages into a single prompt.
func formatMessages(messages []types.Message) string {
	var parts []string
	for _, msg := range messages {
		switch msg.Role {
		case types.RoleUser:
			parts = append(parts, msg.Content)
		case types.RoleAssistant:
			parts = append(parts, "Assistant: "+msg.Content)
		}
	}
	return strings.Join(parts, "\n\n")
}
