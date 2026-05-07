// Package client provides MCP client infrastructure for external server management.

package client

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// MCPJSONConfig represents the .mcp.json file format used by Claude Code, Codex CLI, and IDEs.
type MCPJSONConfig struct {
	MCPServers map[string]MCPJSONServer `json:"mcpServers"`
}

// MCPJSONServer represents a single MCP server entry in .mcp.json.
type MCPJSONServer struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env,omitempty"`
}

// BuildMCPJSONEntry creates a .mcp.json entry for a server.
// Servers with identity are wrapped with 'atmos auth exec' for credential injection.
// If toolchainPATH is non-empty, it is prepended to the server's PATH env var.
func BuildMCPJSONEntry(serverCfg *schema.MCPServerConfig, toolchainPATH string) MCPJSONServer {
	env := copyEnv(serverCfg.Env)

	// Inject toolchain PATH so the CLI tool's MCP subprocess can find uvx/npx.
	if toolchainPATH != "" {
		injectToolchainPATH(env, toolchainPATH)
	}

	if serverCfg.Identity != "" {
		// Wrap with atmos auth exec for credential injection.
		args := []string{"auth", "exec", "-i", serverCfg.Identity, "--", serverCfg.Command}
		args = append(args, serverCfg.Args...)
		return MCPJSONServer{
			Command: "atmos",
			Args:    args,
			Env:     env,
		}
	}

	// No auth — use command directly.
	return MCPJSONServer{
		Command: serverCfg.Command,
		Args:    serverCfg.Args,
		Env:     env,
	}
}

// GenerateMCPConfig builds a MCPJSONConfig from the given servers.
// ToolchainPATH is injected into each server's env if non-empty.
func GenerateMCPConfig(servers map[string]schema.MCPServerConfig, toolchainPATH string) *MCPJSONConfig {
	config := &MCPJSONConfig{
		MCPServers: make(map[string]MCPJSONServer, len(servers)),
	}
	for name, serverCfg := range servers {
		config.MCPServers[name] = BuildMCPJSONEntry(&serverCfg, toolchainPATH)
	}
	return config
}

// WriteMCPConfigToTempFile generates an MCP config and writes it to a temp file.
// Returns the file path. Caller must clean up the file when done.
func WriteMCPConfigToTempFile(servers map[string]schema.MCPServerConfig, toolchainPATH string) (string, error) {
	config := GenerateMCPConfig(servers, toolchainPATH)

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", fmt.Errorf("%w: %w", errUtils.ErrMCPConfigMarshalFailed, err)
	}

	const tempFilePerms = 0o600

	tmpDir := os.TempDir()
	tmpFile := filepath.Join(tmpDir, "atmos-mcp-config.json")

	if err := os.WriteFile(tmpFile, append(data, '\n'), tempFilePerms); err != nil {
		return "", fmt.Errorf("%w: %s: %w", errUtils.ErrMCPConfigWriteFailed, tmpFile, err)
	}

	return tmpFile, nil
}

// copyEnv returns a copy of the env map with keys uppercased.
// Viper lowercases all YAML map keys, but env vars are conventionally UPPERCASE.
// This restores the expected casing (e.g., aws_region → AWS_REGION).
func copyEnv(env map[string]string) map[string]string {
	result := make(map[string]string, len(env))
	for k, v := range env {
		result[strings.ToUpper(k)] = v
	}
	return result
}

const envPATH = "PATH"

// injectToolchainPATH prepends the toolchain PATH to the existing PATH in env.
// Deduplicates PATH entries to avoid bloated env variables.
func injectToolchainPATH(env map[string]string, toolchainPATH string) {
	var basePATH string
	if existing, ok := env[envPATH]; ok && existing != "" {
		basePATH = existing
	} else {
		basePATH = os.Getenv(envPATH) //nolint:forbidigo // Need system PATH as base for toolchain prepend.
	}

	// Combine toolchain PATH + base PATH and deduplicate.
	combined := toolchainPATH + string(os.PathListSeparator) + basePATH
	env[envPATH] = deduplicatePATH(combined)
}

// deduplicatePATH removes duplicate entries from a PATH string while preserving order.
func deduplicatePATH(pathStr string) string {
	seen := make(map[string]bool)
	var unique []string
	for _, dir := range filepath.SplitList(pathStr) {
		if dir == "" {
			continue
		}
		if !seen[dir] {
			seen[dir] = true
			unique = append(unique, dir)
		}
	}
	return strings.Join(unique, string(os.PathListSeparator))
}
