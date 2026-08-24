// Package codexcli provides an AI provider that invokes the OpenAI Codex CLI
// as a subprocess, reusing the user's ChatGPT Plus/Pro subscription.
package codexcli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/agent/base"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/ai/types"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/config/homedir"
	log "github.com/cloudposse/atmos/pkg/logger"
	mcpclient "github.com/cloudposse/atmos/pkg/mcp/client"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

const (
	// ProviderName is the name of this provider for configuration lookup.
	ProviderName = "codex-cli"
	// DefaultBinary is the default binary name for Codex CLI.
	DefaultBinary = "codex"
	// File permissions for MCP config.
	configDirPerms  = 0o700
	configFilePerms = 0o600
)

// Client invokes the OpenAI Codex CLI in non-interactive mode.
type Client struct {
	binaryPath     string
	model          string
	fullAuto       bool
	mcpServers     map[string]schema.MCPServerConfig
	toolchainPATH  string
	hasMCPServers  bool   // True if MCP servers were written to ~/.codex/config.toml.
	originalConfig []byte // Original ~/.codex/config.toml content for restore.
	configBackedUp bool   // True if original config was backed up.
}

// NewClient creates a new Codex CLI client from Atmos configuration.
func NewClient(atmosConfig *schema.AtmosConfiguration) (*Client, error) {
	defer perf.Track(atmosConfig, "codexcli.NewClient")()

	config := base.ExtractConfig(atmosConfig, ProviderName, base.ProviderDefaults{
		Model: ProviderName,
	})

	if !config.Enabled {
		return nil, errUtils.ErrAIDisabledInConfiguration
	}

	providerConfig := base.GetProviderConfig(atmosConfig, ProviderName)

	client := &Client{
		model: config.Model,
	}

	if providerConfig != nil {
		if providerConfig.Binary != "" {
			client.binaryPath = providerConfig.Binary
		}
		if providerConfig.Model != "" {
			client.model = providerConfig.Model
		}
		client.fullAuto = providerConfig.FullAuto
	}

	// Resolve binary path.
	if client.binaryPath == "" {
		resolved, err := exec.LookPath(DefaultBinary)
		if err != nil {
			return nil, errUtils.Build(errUtils.ErrCLIProviderBinaryNotFound).
				WithContext("provider", ProviderName).
				WithContext("binary", DefaultBinary).
				WithHint("Install Codex CLI: npm install -g @openai/codex").
				Err()
		}
		client.binaryPath = resolved
	}

	// Capture MCP servers for pass-through (only if configured).
	// Codex CLI only reads MCP servers from ~/.codex/config.toml (global config).
	// -c flag overrides do NOT register MCP servers as tools. We must write to
	// the global config and restore it after the session.
	if len(atmosConfig.MCP.Servers) > 0 {
		client.mcpServers = atmosConfig.MCP.Servers
		client.toolchainPATH = base.ResolveToolchainPATH(atmosConfig)
		if err := client.writeMCPToGlobalConfig(); err != nil {
			ui.Warning(fmt.Sprintf("Failed to write MCP config: %s", err))
		} else {
			client.hasMCPServers = true
			ui.Info(fmt.Sprintf("MCP servers configured: %d (in ~/.codex/config.toml)", len(client.mcpServers)))
		}
	}

	return client, nil
}

// buildArgs constructs the CLI arguments for codex exec invocation.
func (c *Client) buildArgs() []string {
	args := []string{"exec", "--json"}
	if c.model != "" && c.model != ProviderName {
		args = append(args, "-m", c.model)
	}
	// When MCP servers are configured, use --dangerously-bypass-approvals-and-sandbox
	// because --full-auto only auto-approves file writes, not MCP tool calls.
	if c.hasMCPServers {
		args = append(args, "--dangerously-bypass-approvals-and-sandbox")
	} else if c.fullAuto {
		args = append(args, "--full-auto")
	}
	return args
}

// SendMessage sends a prompt to Codex CLI and returns the response.
func (c *Client) SendMessage(ctx context.Context, message string) (string, error) {
	defer perf.Track(nil, "codexcli.Client.SendMessage")()

	args := c.buildArgs()

	cmd := exec.CommandContext(ctx, c.binaryPath, args...) //nolint:gosec // Binary path is from user config or exec.LookPath.
	cmd.Stdin = strings.NewReader(message)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Restore original config after Codex exits (regardless of success/failure).
	if c.hasMCPServers {
		defer c.restoreGlobalConfig()
	}

	if err := cmd.Run(); err != nil {
		stderrStr := stderr.String()
		if stderrStr != "" {
			return "", fmt.Errorf("%w: %s: %s: %w", errUtils.ErrCLIProviderExecFailed, ProviderName, stderrStr, err)
		}
		return "", fmt.Errorf("%w: %s: %w", errUtils.ErrCLIProviderExecFailed, ProviderName, err)
	}

	return ExtractResult(stdout.Bytes())
}

// SendMessageWithTools is not supported — Codex CLI manages its own tools.
func (c *Client) SendMessageWithTools(_ context.Context, _ string, _ []tools.Tool) (*types.Response, error) {
	return nil, errUtils.ErrCLIProviderToolsNotSupported
}

// SendMessageWithHistory concatenates history into a single prompt.
func (c *Client) SendMessageWithHistory(ctx context.Context, messages []types.Message) (string, error) {
	defer perf.Track(nil, "codexcli.Client.SendMessageWithHistory")()

	return c.SendMessage(ctx, base.FormatMessagesAsPrompt(messages))
}

// SendMessageWithToolsAndHistory is not supported.
func (c *Client) SendMessageWithToolsAndHistory(_ context.Context, _ []types.Message, _ []tools.Tool) (*types.Response, error) {
	return nil, errUtils.ErrCLIProviderToolsNotSupported
}

// SendMessageWithSystemPromptAndTools sends with system prompt prepended.
func (c *Client) SendMessageWithSystemPromptAndTools(
	ctx context.Context,
	systemPrompt string,
	atmosMemory string,
	messages []types.Message,
	_ []tools.Tool,
) (*types.Response, error) {
	defer perf.Track(nil, "codexcli.Client.SendMessageWithSystemPromptAndTools")()

	prompt := base.FormatMessagesAsPrompt(messages)
	if systemPrompt != "" {
		prompt = systemPrompt + "\n\n" + prompt
	}
	if atmosMemory != "" {
		prompt = atmosMemory + "\n\n" + prompt
	}

	result, err := c.SendMessage(ctx, prompt)
	if err != nil {
		return nil, err
	}

	return &types.Response{
		Content:    result,
		StopReason: types.StopReasonEndTurn,
	}, nil
}

// GetModel returns the configured model name.
func (c *Client) GetModel() string { return c.model }

// GetMaxTokens returns 0 — managed by Codex CLI internally.
func (c *Client) GetMaxTokens() int { return 0 }

// ExtractResult parses JSONL output and extracts the final text response.
// Codex CLI emits JSONL events. The response text is in "item.completed" events
// where item.type is "agent_message" (text in item.text directly) or "message"
// (text in item.content[].text array).
func ExtractResult(output []byte) (string, error) {
	var lastText string
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		if text := extractTextFromEvent(scanner.Bytes()); text != "" {
			lastText = text
		}
	}
	if lastText == "" {
		// Try plain text fallback.
		trimmed := strings.TrimSpace(string(output))
		if trimmed != "" {
			return trimmed, nil
		}
		return "", errUtils.ErrCLIProviderParseResponse
	}
	return lastText, nil
}

// extractTextFromEvent extracts text from a single JSONL event line.
// Returns empty string if the event is not an item.completed with text content.
func extractTextFromEvent(line []byte) string {
	var event codexEvent
	if err := json.Unmarshal(line, &event); err != nil {
		return ""
	}
	if event.Type != "item.completed" {
		return ""
	}
	// Codex CLI uses "agent_message" type with text directly on the item.
	if event.Item.Type == "agent_message" && event.Item.Text != "" {
		return event.Item.Text
	}
	// Also handle "message" type with nested content array (API format).
	if event.Item.Type == "message" {
		for _, content := range event.Item.Content {
			if content.Type == "text" {
				return content.Text
			}
		}
	}
	return ""
}

// injectAtmosEnvVars adds ATMOS_* environment variables from the current process
// into each MCP server's env. Codex CLI MCP servers don't inherit the parent
// environment, so auth-related vars (ATMOS_PROFILE, ATMOS_BASE_PATH, etc.)
// must be explicitly passed.
func injectAtmosEnvVars(config *mcpclient.MCPJSONConfig) {
	atmosVars := collectAtmosEnvVars()
	if len(atmosVars) == 0 {
		return
	}
	for name, srv := range config.MCPServers {
		if srv.Env == nil {
			srv.Env = make(map[string]string)
		}
		for k, v := range atmosVars {
			// Don't overwrite explicitly configured values.
			if _, exists := srv.Env[k]; !exists {
				srv.Env[k] = v
			}
		}
		config.MCPServers[name] = srv
	}
}

// collectAtmosEnvVars returns all ATMOS_* env vars from the current process.
func collectAtmosEnvVars() map[string]string {
	result := make(map[string]string)
	for _, env := range os.Environ() {
		if strings.HasPrefix(env, cfg.AtmosEnvVarPrefix) {
			if idx := strings.IndexByte(env, '='); idx > 0 {
				result[env[:idx]] = env[idx+1:]
			}
		}
	}
	return result
}

// codexConfigPath returns the path to ~/.codex/config.toml.
func codexConfigPath() string {
	home, _ := homedir.Dir()
	return filepath.Join(home, ".codex", "config.toml")
}

// writeMCPToGlobalConfig writes MCP servers to ~/.codex/config.toml.
// Backs up the original content for later restore.
func (c *Client) writeMCPToGlobalConfig() error {
	configPath := codexConfigPath()

	// Backup existing config.
	if data, err := os.ReadFile(configPath); err == nil {
		c.originalConfig = data
		c.configBackedUp = true
	}

	// Generate TOML content with MCP servers.
	mcpConfig := mcpclient.GenerateMCPConfig(c.mcpServers, c.toolchainPATH)

	// Codex CLI MCP servers don't inherit the parent process environment.
	// Inject ATMOS_* env vars so auth and config discovery work correctly.
	injectAtmosEnvVars(mcpConfig)

	var buf bytes.Buffer
	// Preserve existing non-MCP config.
	if c.configBackedUp {
		buf.Write(c.originalConfig)
		buf.WriteString("\n")
	}
	for name, srv := range mcpConfig.MCPServers {
		writeTOMLServer(&buf, name, srv)
	}

	// Ensure ~/.codex/ directory exists.
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, configDirPerms); err != nil {
		return fmt.Errorf("%w: mkdir %s: %w", errUtils.ErrMCPConfigWriteFailed, configDir, err)
	}

	if err := os.WriteFile(configPath, buf.Bytes(), configFilePerms); err != nil {
		return fmt.Errorf("%w: %s: %w", errUtils.ErrMCPConfigWriteFailed, configPath, err)
	}
	return nil
}

// restoreGlobalConfig restores the original ~/.codex/config.toml content.
func (c *Client) restoreGlobalConfig() {
	configPath := codexConfigPath()
	if c.configBackedUp {
		if err := os.WriteFile(configPath, c.originalConfig, configFilePerms); err != nil {
			log.Debug("Failed to restore codex config", "path", configPath, "error", err)
		}
	} else {
		// No original config existed — remove the file we created.
		if err := os.Remove(configPath); err != nil && !os.IsNotExist(err) {
			log.Debug("Failed to remove codex config", "path", configPath, "error", err)
		}
	}
}

type codexEvent struct {
	Type string    `json:"type"`
	Item codexItem `json:"item"`
}

type codexItem struct {
	Type    string         `json:"type"`
	Text    string         `json:"text,omitempty"`    // Direct text for agent_message type.
	Content []codexContent `json:"content,omitempty"` // Nested content for message type.
}

type codexContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// writeMCPConfigTOML creates a temp directory with .codex/config.toml containing MCP config.
// Codex CLI reads MCP servers from [mcp_servers.<name>] tables in config.toml.
// Returns the temp directory path. Caller should clean up with os.RemoveAll.
func writeMCPConfigTOML(servers map[string]schema.MCPServerConfig, toolchainPATH string) (string, error) {
	mcpConfig := mcpclient.GenerateMCPConfig(servers, toolchainPATH)

	var buf bytes.Buffer
	for name, srv := range mcpConfig.MCPServers {
		writeTOMLServer(&buf, name, srv)
	}

	tmpDir, err := os.MkdirTemp("", "atmos-codex-*")
	if err != nil {
		return "", fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrMCPConfigWriteFailed, err)
	}

	codexDir := filepath.Join(tmpDir, ".codex")
	if err := os.MkdirAll(codexDir, configDirPerms); err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", fmt.Errorf("%w: mkdir %s: %w", errUtils.ErrMCPConfigWriteFailed, codexDir, err)
	}

	configFile := filepath.Join(codexDir, "config.toml")
	if err := os.WriteFile(configFile, buf.Bytes(), configFilePerms); err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", fmt.Errorf("%w: %s: %w", errUtils.ErrMCPConfigWriteFailed, configFile, err)
	}

	return tmpDir, nil
}

// writeTOMLServer writes a single [mcp_servers.<name>] section.
func writeTOMLServer(buf *bytes.Buffer, name string, srv mcpclient.MCPJSONServer) {
	fmt.Fprintf(buf, "[mcp_servers.%s]\n", name)
	fmt.Fprintf(buf, "command = %q\n", srv.Command)
	if len(srv.Args) > 0 {
		fmt.Fprintf(buf, "args = [")
		for i, arg := range srv.Args {
			if i > 0 {
				fmt.Fprint(buf, ", ")
			}
			fmt.Fprintf(buf, "%q", arg)
		}
		fmt.Fprint(buf, "]\n")
	}
	if len(srv.Env) > 0 {
		fmt.Fprintf(buf, "\n[mcp_servers.%s.env]\n", name)
		for k, v := range srv.Env {
			fmt.Fprintf(buf, "%s = %q\n", k, v)
		}
	}
	fmt.Fprint(buf, "\n")
}
