// Package copilotcli provides an AI provider that invokes the GitHub Copilot CLI
// as a subprocess, reusing the user's GitHub Copilot subscription.
package copilotcli

import (
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
	"github.com/cloudposse/atmos/pkg/config/homedir"
	log "github.com/cloudposse/atmos/pkg/logger"
	mcpclient "github.com/cloudposse/atmos/pkg/mcp/client"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

const (
	// ProviderName is the name of this provider for configuration lookup.
	ProviderName = "copilot-cli"
	// DefaultBinary is the default binary name for the GitHub Copilot CLI.
	DefaultBinary = "copilot"
	// CopilotHomeEnvVar overrides the Copilot CLI configuration directory (~/.copilot by default).
	CopilotHomeEnvVar = "COPILOT_HOME"
	// File permissions for MCP config.
	configDirPerms  = 0o700
	configFilePerms = 0o600
)

// Client invokes the GitHub Copilot CLI in non-interactive (programmatic) mode.
// Authentication is handled by the Copilot CLI itself: `copilot /login`, or the
// COPILOT_GITHUB_TOKEN / GH_TOKEN / GITHUB_TOKEN environment variables (in that
// precedence order) with a token that carries a Copilot subscription.
type Client struct {
	binaryPath     string
	model          string
	fullAuto       bool
	mcpServers     map[string]schema.MCPServerConfig
	toolchainPATH  string
	hasMCPServers  bool   // True if MCP servers were written to ~/.copilot/mcp-config.json.
	originalConfig []byte // Original mcp-config.json content for restore.
	configBackedUp bool   // True if original config was backed up.
}

// NewClient creates a new Copilot CLI client from Atmos configuration.
func NewClient(atmosConfig *schema.AtmosConfiguration) (*Client, error) {
	defer perf.Track(atmosConfig, "copilotcli.NewClient")()

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
				WithHint("Install Copilot CLI: npm install -g @github/copilot").
				Err()
		}
		client.binaryPath = resolved
	}

	// Capture MCP servers for pass-through (only if configured).
	// Copilot CLI reads MCP servers from mcp-config.json in its config
	// directory (~/.copilot by default, overridable via COPILOT_HOME).
	// We merge our servers into that file and restore it after the session.
	if len(atmosConfig.MCP.Servers) > 0 {
		client.mcpServers = atmosConfig.MCP.Servers
		client.toolchainPATH = base.ResolveToolchainPATH(atmosConfig)
		if err := client.writeMCPConfig(); err != nil {
			ui.Warning(fmt.Sprintf("Failed to write MCP config: %s", err))
		} else {
			client.hasMCPServers = true
			ui.Info(fmt.Sprintf("MCP servers configured: %d (in %s)", len(client.mcpServers), copilotMCPConfigPath()))
		}
	}

	return client, nil
}

// buildArgs constructs the CLI arguments for a programmatic copilot invocation.
func (c *Client) buildArgs(message string) []string {
	// -p runs a single prompt non-interactively; -s suppresses stats and
	// decoration so stdout contains only the agent's response; --no-ask-user
	// prevents the agent from pausing for clarifying questions (no TTY).
	args := []string{"-p", message, "-s", "--no-ask-user"}
	if c.model != "" && c.model != ProviderName {
		args = append(args, "--model", c.model)
	}
	// Tools (including MCP tools) require explicit approval, which is impossible
	// in non-interactive mode. Auto-approve when MCP servers are configured or
	// the user opted into full_auto.
	if c.hasMCPServers || c.fullAuto {
		args = append(args, "--allow-all-tools")
	}
	return args
}

// SendMessage sends a prompt to Copilot CLI and returns the response.
func (c *Client) SendMessage(ctx context.Context, message string) (string, error) {
	defer perf.Track(nil, "copilotcli.Client.SendMessage")()

	args := c.buildArgs(message)

	cmd := exec.CommandContext(ctx, c.binaryPath, args...) //nolint:gosec // Binary path is from user config or exec.LookPath.

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Restore original MCP config after Copilot exits (regardless of success/failure).
	if c.hasMCPServers {
		defer c.restoreMCPConfig()
	}

	if err := cmd.Run(); err != nil {
		stderrStr := strings.TrimSpace(stderr.String())
		if stderrStr != "" {
			return "", fmt.Errorf("%w: %s: %s: %w", errUtils.ErrCLIProviderExecFailed, ProviderName, stderrStr, err)
		}
		return "", fmt.Errorf("%w: %s: %w", errUtils.ErrCLIProviderExecFailed, ProviderName, err)
	}

	return ExtractResult(stdout.Bytes())
}

// SendMessageWithTools is not supported — Copilot CLI manages its own tools.
func (c *Client) SendMessageWithTools(_ context.Context, _ string, _ []tools.Tool) (*types.Response, error) {
	return nil, errUtils.ErrCLIProviderToolsNotSupported
}

// SendMessageWithHistory concatenates history into a single prompt.
func (c *Client) SendMessageWithHistory(ctx context.Context, messages []types.Message) (string, error) {
	defer perf.Track(nil, "copilotcli.Client.SendMessageWithHistory")()

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
	defer perf.Track(nil, "copilotcli.Client.SendMessageWithSystemPromptAndTools")()

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

// GetMaxTokens returns 0 — managed by Copilot CLI internally.
func (c *Client) GetMaxTokens() int { return 0 }

// ExtractResult extracts the final text response from Copilot CLI output.
// With the -s (silent) flag, stdout contains only the agent's plain-text response.
func ExtractResult(output []byte) (string, error) {
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		return "", errUtils.ErrCLIProviderParseResponse
	}
	return trimmed, nil
}

// copilotMCPServer is a single MCP server entry in Copilot CLI's mcp-config.json.
// It extends the shared MCP JSON server shape with Copilot-specific fields.
type copilotMCPServer struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env,omitempty"`
	Type    string            `json:"type"`  // Copilot CLI requires "local" for stdio servers.
	Tools   []string          `json:"tools"` // Tools to expose; "*" exposes all.
}

// copilotConfigDir returns the Copilot CLI configuration directory.
// Defaults to ~/.copilot, overridable via COPILOT_HOME.
func copilotConfigDir() string {
	if dir := os.Getenv(CopilotHomeEnvVar); dir != "" { //nolint:forbidigo // COPILOT_HOME is Copilot CLI's own env var, not Atmos configuration.
		return dir
	}
	home, _ := homedir.Dir()
	return filepath.Join(home, ".copilot")
}

// copilotMCPConfigPath returns the path to Copilot CLI's mcp-config.json.
func copilotMCPConfigPath() string {
	return filepath.Join(copilotConfigDir(), "mcp-config.json")
}

// writeMCPConfig merges the Atmos MCP servers into Copilot CLI's mcp-config.json.
// Backs up the original content for later restore. Existing server entries are
// preserved; Atmos-managed entries with the same name are overwritten.
func (c *Client) writeMCPConfig() error {
	configPath := copilotMCPConfigPath()

	// Backup existing config.
	servers := map[string]json.RawMessage{}
	if data, err := os.ReadFile(configPath); err == nil {
		c.originalConfig = data
		c.configBackedUp = true
		// Preserve existing non-Atmos servers.
		var existing struct {
			MCPServers map[string]json.RawMessage `json:"mcpServers"`
		}
		if unmarshalErr := json.Unmarshal(data, &existing); unmarshalErr == nil {
			for name, srv := range existing.MCPServers {
				servers[name] = srv
			}
		}
	}

	// Generate the shared MCP config (wraps auth-requiring servers with
	// `atmos auth exec` and injects the toolchain PATH).
	mcpConfig := mcpclient.GenerateMCPConfig(c.mcpServers, c.toolchainPATH)
	for name, srv := range mcpConfig.MCPServers {
		entry := copilotMCPServer{
			Command: srv.Command,
			Args:    srv.Args,
			Env:     srv.Env,
			Type:    "local",
			Tools:   []string{"*"},
		}
		raw, err := json.Marshal(entry)
		if err != nil {
			return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrMCPConfigMarshalFailed, err)
		}
		servers[name] = raw
	}

	out, err := json.MarshalIndent(map[string]map[string]json.RawMessage{"mcpServers": servers}, "", "  ")
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrMCPConfigMarshalFailed, err)
	}

	// Ensure the Copilot config directory exists.
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, configDirPerms); err != nil {
		return fmt.Errorf("%w: mkdir %s: %w", errUtils.ErrMCPConfigWriteFailed, configDir, err)
	}

	if err := os.WriteFile(configPath, append(out, '\n'), configFilePerms); err != nil {
		return fmt.Errorf("%w: %s: %w", errUtils.ErrMCPConfigWriteFailed, configPath, err)
	}
	return nil
}

// restoreMCPConfig restores the original mcp-config.json content.
func (c *Client) restoreMCPConfig() {
	configPath := copilotMCPConfigPath()
	if c.configBackedUp {
		if err := os.WriteFile(configPath, c.originalConfig, configFilePerms); err != nil {
			log.Debug("Failed to restore copilot MCP config", "path", configPath, "error", err)
		}
	} else {
		// No original config existed — remove the file we created.
		if err := os.Remove(configPath); err != nil && !os.IsNotExist(err) {
			log.Debug("Failed to remove copilot MCP config", "path", configPath, "error", err)
		}
	}
}
