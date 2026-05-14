// Package geminicli provides an AI provider that invokes the Gemini CLI
// as a subprocess, reusing the user's Google account (free tier or API key).
package geminicli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/agent/base"
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
	ProviderName = "gemini-cli"
	// DefaultBinary is the default binary name for Gemini CLI.
	DefaultBinary = "gemini"
	// Dir and file permissions for MCP settings.
	settingsDirPerms  = 0o700
	settingsFilePerms = 0o600
)

// Client invokes the Gemini CLI in non-interactive mode.
type Client struct {
	binaryPath       string
	model            string
	mcpServers       map[string]schema.MCPServerConfig
	toolchainPATH    string
	originalSettings []byte // Original .gemini/settings.json content for restore.
	settingsBackedUp bool   // True if original settings were backed up.
}

// NewClient creates a new Gemini CLI client from Atmos configuration.
func NewClient(atmosConfig *schema.AtmosConfiguration) (*Client, error) {
	defer perf.Track(atmosConfig, "geminicli.NewClient")()

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
	}

	// Resolve binary path.
	if client.binaryPath == "" {
		resolved, err := exec.LookPath(DefaultBinary)
		if err != nil {
			return nil, errUtils.Build(errUtils.ErrCLIProviderBinaryNotFound).
				WithContext("provider", ProviderName).
				WithContext("binary", DefaultBinary).
				WithHint("Install Gemini CLI: npm install -g @google/gemini-cli").
				Err()
		}
		client.binaryPath = resolved
	}

	// Capture MCP servers for pass-through (only if configured).
	// Write .gemini/settings.json in the current working directory (not a temp dir)
	// because Gemini CLI's Trusted Folders feature blocks MCP in untrusted directories.
	if len(atmosConfig.MCP.Servers) > 0 {
		client.mcpServers = atmosConfig.MCP.Servers
		client.toolchainPATH = base.ResolveToolchainPATH(atmosConfig)
		settingsFile, err := client.writeMCPSettingsInCwd()
		if err != nil {
			log.Debug("Failed to generate Gemini MCP settings", "error", err)
		} else {
			ui.Info(fmt.Sprintf("MCP servers configured: %d (settings: %s)", len(client.mcpServers), settingsFile))
		}
	}

	return client, nil
}

// SendMessage sends a prompt to Gemini CLI and returns the response.
// Constructs the CLI arguments for Gemini invocation.
func (c *Client) buildArgs(message string) []string {
	args := []string{
		"--prompt", message,
		"--output-format", "json",
	}
	if c.model != "" && c.model != ProviderName {
		args = append(args, "-m", c.model)
	}
	args = append(args, "--approval-mode", "auto_edit")

	// Explicitly allow configured MCP servers by name (sorted for deterministic output).
	if len(c.mcpServers) > 0 {
		var serverNames []string
		for name := range c.mcpServers {
			serverNames = append(serverNames, name)
		}
		sort.Strings(serverNames)
		args = append(args, "--allowed-mcp-server-names", strings.Join(serverNames, ","))
	}
	return args
}

func (c *Client) SendMessage(ctx context.Context, message string) (string, error) {
	defer perf.Track(nil, "geminicli.Client.SendMessage")()

	args := c.buildArgs(message)

	cmd := exec.CommandContext(ctx, c.binaryPath, args...) //nolint:gosec // Binary path is from user config or exec.LookPath.

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Restore original .gemini/settings.json after Gemini exits.
	if c.settingsBackedUp || len(c.mcpServers) > 0 {
		defer c.restoreSettings()
	}

	if err := cmd.Run(); err != nil {
		// Filter stderr to find meaningful error lines (skip deprecation warnings).
		errMsg := filterStderr(stderr.String())
		if errMsg != "" {
			return "", fmt.Errorf("%w: %s: %s: %w", errUtils.ErrCLIProviderExecFailed, ProviderName, errMsg, err)
		}
		return "", fmt.Errorf("%w: %s: %w", errUtils.ErrCLIProviderExecFailed, ProviderName, err)
	}

	// If stdout is empty but stderr has content, Gemini may have succeeded
	// but only wrote to stderr (e.g., deprecation warnings). Try stdout first.
	return parseResponse(stdout.Bytes())
}

// SendMessageWithTools is not supported — Gemini CLI manages its own tools.
func (c *Client) SendMessageWithTools(_ context.Context, _ string, _ []tools.Tool) (*types.Response, error) {
	return nil, errUtils.ErrCLIProviderToolsNotSupported
}

// SendMessageWithHistory concatenates history into a single prompt.
func (c *Client) SendMessageWithHistory(ctx context.Context, messages []types.Message) (string, error) {
	defer perf.Track(nil, "geminicli.Client.SendMessageWithHistory")()

	return c.SendMessage(ctx, base.FormatMessagesAsPrompt(messages))
}

// SendMessageWithToolsAndHistory is not supported.
func (c *Client) SendMessageWithToolsAndHistory(_ context.Context, _ []types.Message, _ []tools.Tool) (*types.Response, error) {
	return nil, errUtils.ErrCLIProviderToolsNotSupported
}

// SendMessageWithSystemPromptAndTools sends with system prompt prepended to the prompt.
func (c *Client) SendMessageWithSystemPromptAndTools(
	ctx context.Context,
	systemPrompt string,
	atmosMemory string,
	messages []types.Message,
	_ []tools.Tool,
) (*types.Response, error) {
	defer perf.Track(nil, "geminicli.Client.SendMessageWithSystemPromptAndTools")()

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

// GetMaxTokens returns 0 — managed by Gemini CLI internally.
func (c *Client) GetMaxTokens() int { return 0 }

// geminiResponse is the JSON output from `gemini -p --output-format json`.
type geminiResponse struct {
	SessionID string `json:"session_id"`
	Response  string `json:"response"`
}

// parseResponse extracts the response text from Gemini CLI JSON output.
func parseResponse(output []byte) (string, error) {
	var resp geminiResponse
	if err := json.Unmarshal(output, &resp); err != nil {
		// Gemini CLI may return plain text in some modes.
		trimmed := strings.TrimSpace(string(output))
		if trimmed != "" {
			return trimmed, nil
		}
		return "", fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrCLIProviderParseResponse, err)
	}
	if resp.Response == "" {
		trimmed := strings.TrimSpace(string(output))
		if trimmed != "" {
			return trimmed, nil
		}
		return "", errUtils.ErrCLIProviderParseResponse
	}
	return resp.Response, nil
}

// geminiSettings is the .gemini/settings.json format for MCP server configuration.
type geminiSettings struct {
	MCPServers map[string]mcpclient.MCPJSONServer `json:"mcpServers"`
}

// writeMCPSettingsInCwd writes .gemini/settings.json in the current working directory.
// Gemini CLI's Trusted Folders feature blocks MCP servers in untrusted directories,
// so we write to cwd (which the user has already trusted) instead of a temp dir.
// Returns the settings file path.
// Writes .gemini/settings.json to the current working directory.
// Backs up the existing file (if any) for later restore via restoreSettings().
func (c *Client) writeMCPSettingsInCwd() (string, error) {
	config := mcpclient.GenerateMCPConfig(c.mcpServers, c.toolchainPATH)

	settings := geminiSettings{
		MCPServers: config.MCPServers,
	}

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return "", fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrMCPConfigMarshalFailed, err)
	}

	geminiDir := ".gemini"
	if err := os.MkdirAll(geminiDir, settingsDirPerms); err != nil {
		return "", fmt.Errorf("%w: mkdir %s: %w", errUtils.ErrMCPConfigWriteFailed, geminiDir, err)
	}

	settingsFile := filepath.Join(geminiDir, "settings.json")

	// Back up existing settings file if present.
	if existing, readErr := os.ReadFile(settingsFile); readErr == nil {
		c.originalSettings = existing
		c.settingsBackedUp = true
	}

	if err := os.WriteFile(settingsFile, append(data, '\n'), settingsFilePerms); err != nil {
		return "", fmt.Errorf("%w: %s: %w", errUtils.ErrMCPConfigWriteFailed, settingsFile, err)
	}

	return settingsFile, nil
}

// restoreSettings restores the original .gemini/settings.json content.
func (c *Client) restoreSettings() {
	settingsFile := filepath.Join(".gemini", "settings.json")
	if c.settingsBackedUp {
		if err := os.WriteFile(settingsFile, c.originalSettings, settingsFilePerms); err != nil {
			log.Debug("Failed to restore gemini settings", "path", settingsFile, "error", err)
		}
	} else {
		// No original file existed — remove the one we created.
		if err := os.Remove(settingsFile); err != nil && !os.IsNotExist(err) {
			log.Debug("Failed to remove gemini settings", "path", settingsFile, "error", err)
		}
	}
}

// filterStderr removes common non-error lines from stderr (deprecation warnings, info messages).
func filterStderr(stderr string) string {
	var meaningful []string
	for _, line := range strings.Split(stderr, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		// Skip Node.js deprecation warnings.
		if strings.Contains(trimmed, "DeprecationWarning") || strings.Contains(trimmed, "--trace-deprecation") {
			continue
		}
		// Skip YOLO mode info messages.
		if strings.Contains(trimmed, "YOLO mode") {
			continue
		}
		// Skip credential cache messages.
		if strings.Contains(trimmed, "Loaded cached credentials") {
			continue
		}
		meaningful = append(meaningful, trimmed)
	}
	return strings.Join(meaningful, "\n")
}
