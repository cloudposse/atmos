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
	"github.com/cloudposse/atmos/pkg/dependencies"
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
	binaryPath    string
	model         string
	fullAuto      bool
	mcpServers    map[string]schema.MCPServerConfig
	toolchainPATH string
	mcpConfigDir  string // Temp dir containing .codex/config.toml for MCP config.
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
	if len(atmosConfig.MCP.Servers) > 0 {
		client.mcpServers = atmosConfig.MCP.Servers
		client.toolchainPATH = resolveToolchainPATH(atmosConfig)
		// Generate .codex/config.toml in a temp directory.
		configDir, err := writeMCPConfigTOML(client.mcpServers, client.toolchainPATH)
		if err != nil {
			log.Debug("Failed to generate Codex MCP config", "error", err)
		} else {
			client.mcpConfigDir = configDir
			ui.Info(fmt.Sprintf("MCP servers configured: %d (config: %s)", len(client.mcpServers),
				filepath.Join(configDir, ".codex", "config.toml")))
		}
	}

	return client, nil
}

// SendMessage sends a prompt to Codex CLI and returns the response.
func (c *Client) SendMessage(ctx context.Context, message string) (string, error) {
	defer perf.Track(nil, "codexcli.Client.SendMessage")()

	args := []string{"exec", "--json"}
	if c.model != "" && c.model != ProviderName {
		args = append(args, "-m", c.model)
	}
	if c.fullAuto {
		args = append(args, "--full-auto")
	}

	cmd := exec.CommandContext(ctx, c.binaryPath, args...) //nolint:gosec // Binary path is from user config or exec.LookPath.
	cmd.Stdin = strings.NewReader(message)

	// If MCP config dir exists, set it as working directory so Codex CLI
	// picks up .codex/config.toml from the project-level config.
	if c.mcpConfigDir != "" {
		cmd.Dir = c.mcpConfigDir
	}

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

	return ExtractResult(stdout.Bytes())
}

// SendMessageWithTools is not supported — Codex CLI manages its own tools.
func (c *Client) SendMessageWithTools(_ context.Context, _ string, _ []tools.Tool) (*types.Response, error) {
	return nil, errUtils.ErrCLIProviderToolsNotSupported
}

// SendMessageWithHistory concatenates history into a single prompt.
func (c *Client) SendMessageWithHistory(ctx context.Context, messages []types.Message) (string, error) {
	defer perf.Track(nil, "codexcli.Client.SendMessageWithHistory")()

	return c.SendMessage(ctx, formatMessages(messages))
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

	prompt := formatMessages(messages)
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
func ExtractResult(output []byte) (string, error) {
	var lastText string
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		var event codexEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			continue
		}
		if event.Type == "item.completed" && event.Item.Type == "message" {
			for _, content := range event.Item.Content {
				if content.Type == "text" {
					lastText = content.Text
				}
			}
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

type codexEvent struct {
	Type string    `json:"type"`
	Item codexItem `json:"item"`
}

type codexItem struct {
	Type    string         `json:"type"`
	Content []codexContent `json:"content"`
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
	for _, envVar := range tenv.EnvVars() {
		if strings.HasPrefix(envVar, "PATH=") {
			return envVar[len("PATH="):]
		}
	}
	return ""
}

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
