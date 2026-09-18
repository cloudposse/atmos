// Package opencode provides an AI provider that invokes the opencode CLI
// (https://opencode.ai) as a subprocess, reusing the user's opencode setup and
// whichever model provider they have configured/authenticated there.
package opencode

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
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
	ProviderName = "opencode"
	// DefaultBinary is the default binary name for the opencode CLI.
	DefaultBinary = "opencode"
	// ConfigEnvVar points opencode at a specific config file. Its precedence sits between
	// the global and project configs, and opencode deep-merges it, so pointing it at a
	// temp file that only defines `mcp` servers adds those servers without disturbing the
	// user's own opencode.json.
	ConfigEnvVar = "OPENCODE_CONFIG"
	// Schema reference embedded in the generated opencode config file.
	configSchemaURL = "https://opencode.ai/config.json"
	// File mode for the temp MCP config (owner-only read/write).
	configFilePerms = 0o600
)

// Client invokes the opencode CLI in non-interactive ("run") mode. Authentication and
// model-provider selection are handled by opencode itself (`opencode auth login`), the
// same way Atmos relies on the user's existing `terraform`/`tofu` installation.
type Client struct {
	binaryPath    string
	model         string
	fullAuto      bool
	mcpServers    map[string]schema.MCPServerConfig
	toolchainPATH string
	hasMCPServers bool // True if MCP servers were configured for pass-through.
}

// NewClient creates a new opencode CLI client from Atmos configuration.
func NewClient(atmosConfig *schema.AtmosConfiguration) (*Client, error) {
	defer perf.Track(atmosConfig, "opencode.NewClient")()

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
				WithHint("Install opencode: npm install -g opencode-ai (see https://opencode.ai/docs/#install)").
				Err()
		}
		client.binaryPath = resolved
	}

	// Capture MCP servers for pass-through (only if configured). opencode reads MCP servers
	// from its config file; we hand it a temp config via OPENCODE_CONFIG at invocation time
	// (see SendMessage), so no user file is ever modified.
	if len(atmosConfig.MCP.Servers) > 0 {
		client.mcpServers = atmosConfig.MCP.Servers
		client.toolchainPATH = base.ResolveToolchainPATH(atmosConfig)
		client.hasMCPServers = true
		ui.Info(fmt.Sprintf("MCP servers configured: %d (via %s)", len(client.mcpServers), ConfigEnvVar))
	}

	return client, nil
}

// buildArgs constructs the CLI arguments for a non-interactive `opencode run` invocation.
func (c *Client) buildArgs(message string) []string {
	// `run` executes a single prompt non-interactively and prints the assistant's response
	// to stdout. (We deliberately avoid `--format json`: its JSONL stream has known issues
	// dropping the final event, whereas the default output reliably carries the answer.)
	args := []string{"run", message}
	if c.model != "" && c.model != ProviderName {
		args = append(args, "-m", c.model)
	}
	// opencode allows tool calls (built-in and MCP) by default, so they run non-interactively
	// without any approval flag. We pass --auto ONLY when the user explicitly opts in via
	// full_auto: --auto blanket-approves every permission that isn't explicitly denied (file,
	// shell, network) and overrides any `ask` rules in the user's opencode config, so we must
	// not enable it implicitly just because MCP servers are configured. Explicit `deny` rules
	// are always still enforced by opencode even under --auto.
	if c.fullAuto {
		args = append(args, "--auto")
	}
	return args
}

// SendMessage sends a prompt to opencode and returns the response.
func (c *Client) SendMessage(ctx context.Context, message string) (string, error) {
	defer perf.Track(nil, "opencode.Client.SendMessage")()

	args := c.buildArgs(message)

	cmd := exec.CommandContext(ctx, c.binaryPath, args...) //nolint:gosec // Binary path is from user config or exec.LookPath.
	cmd.Env = os.Environ()

	// Point opencode at a temp config containing the pass-through MCP servers, cleaned up
	// after the subprocess exits. Writing per-call keeps multi-turn sessions correct and
	// leaves no state behind.
	cleanup := c.applyMCPConfig(cmd)
	defer cleanup()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		stderrStr := strings.TrimSpace(stderr.String())
		if stderrStr != "" {
			return "", fmt.Errorf("%w: %s: %s: %w", errUtils.ErrCLIProviderExecFailed, ProviderName, stderrStr, err)
		}
		return "", fmt.Errorf("%w: %s: %w", errUtils.ErrCLIProviderExecFailed, ProviderName, err)
	}

	return ExtractResult(stdout.Bytes())
}

// SendMessageWithTools is not supported — opencode manages its own tools.
func (c *Client) SendMessageWithTools(_ context.Context, _ string, _ []tools.Tool) (*types.Response, error) {
	return nil, errUtils.ErrCLIProviderToolsNotSupported
}

// SendMessageWithHistory concatenates history into a single prompt.
func (c *Client) SendMessageWithHistory(ctx context.Context, messages []types.Message) (string, error) {
	defer perf.Track(nil, "opencode.Client.SendMessageWithHistory")()

	return c.SendMessage(ctx, base.FormatMessagesAsPrompt(messages))
}

// SendMessageWithToolsAndHistory is not supported.
func (c *Client) SendMessageWithToolsAndHistory(_ context.Context, _ []types.Message, _ []tools.Tool) (*types.Response, error) {
	return nil, errUtils.ErrCLIProviderToolsNotSupported
}

// SendMessageWithSystemPromptAndTools sends with system prompt and memory prepended.
func (c *Client) SendMessageWithSystemPromptAndTools(
	ctx context.Context,
	systemPrompt string,
	atmosMemory string,
	messages []types.Message,
	_ []tools.Tool,
) (*types.Response, error) {
	defer perf.Track(nil, "opencode.Client.SendMessageWithSystemPromptAndTools")()

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

// GetMaxTokens returns 0 — managed by opencode internally.
func (c *Client) GetMaxTokens() int { return 0 }

// ExtractResult extracts the final text response from opencode's output. The default
// `run` output carries the assistant's plain-text response on stdout.
func ExtractResult(output []byte) (string, error) {
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		return "", errUtils.ErrCLIProviderParseResponse
	}
	return trimmed, nil
}

// opencodeMCPServer is a single local MCP server entry in opencode's config `mcp` map.
// The tool expects the command and its arguments as a single array, environment variables
// under `environment`, and a `type` of "local" for stdio servers.
type opencodeMCPServer struct {
	Type        string            `json:"type"`
	Command     []string          `json:"command"`
	Enabled     bool              `json:"enabled"`
	Environment map[string]string `json:"environment,omitempty"`
}

// opencodeConfig is the minimal opencode config file we generate for MCP pass-through.
type opencodeConfig struct {
	Schema string                       `json:"$schema"`
	MCP    map[string]opencodeMCPServer `json:"mcp"`
}

// writeTempMCPConfig writes a temp opencode config file describing the pass-through MCP
// servers and returns its path. The caller is responsible for removing it.
func (c *Client) writeTempMCPConfig() (string, error) {
	// Generate the shared MCP config (wraps auth-requiring servers with `atmos auth exec`
	// and injects the toolchain PATH), then translate it into opencode's schema.
	shared := mcpclient.GenerateMCPConfig(c.mcpServers, c.toolchainPATH)

	cfg := opencodeConfig{
		Schema: configSchemaURL,
		MCP:    make(map[string]opencodeMCPServer, len(shared.MCPServers)),
	}
	for name, srv := range shared.MCPServers {
		command := append([]string{srv.Command}, srv.Args...)
		cfg.MCP[name] = opencodeMCPServer{
			Type:        "local",
			Command:     command,
			Enabled:     true,
			Environment: srv.Env,
		}
	}

	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrMCPConfigMarshalFailed, err)
	}

	f, err := os.CreateTemp("", "atmos-opencode-*.json")
	if err != nil {
		return "", fmt.Errorf("%w: %w", errUtils.ErrMCPConfigWriteFailed, err)
	}
	path := f.Name()

	_, writeErr := f.Write(append(out, '\n'))
	if closeErr := f.Close(); closeErr != nil && writeErr == nil {
		writeErr = closeErr
	}
	if writeErr != nil {
		removeTempConfig(path)
		return "", fmt.Errorf("%w: %s: %w", errUtils.ErrMCPConfigWriteFailed, path, writeErr)
	}

	if err := os.Chmod(path, configFilePerms); err != nil {
		log.Debug("Failed to chmod opencode MCP config", "path", path, "error", err)
	}

	return path, nil
}

// applyMCPConfig writes a temp MCP config (when servers are configured) and points the
// subprocess at it via OPENCODE_CONFIG. It returns a cleanup func the caller must defer.
func (c *Client) applyMCPConfig(cmd *exec.Cmd) func() {
	if !c.hasMCPServers {
		return func() {}
	}
	configPath, err := c.writeTempMCPConfig()
	if err != nil {
		ui.Warning(fmt.Sprintf("Failed to write MCP config: %s", err))
		return func() {}
	}
	cmd.Env = append(cmd.Env, ConfigEnvVar+"="+configPath)
	return func() { removeTempConfig(configPath) }
}

// removeTempConfig removes a generated temp config file, ignoring a missing file.
func removeTempConfig(path string) {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		log.Debug("Failed to remove opencode MCP config", "path", path, "error", err)
	}
}
