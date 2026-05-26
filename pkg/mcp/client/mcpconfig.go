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
	Type    string            `json:"type,omitempty"`
	URL     string            `json:"url,omitempty"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

// BuildMCPJSONEntry creates a .mcp.json entry for a server.
// Servers with identity are wrapped with 'atmos auth exec' for credential injection.
// If toolchainPATH is non-empty, it is prepended to the server's PATH env var.
func BuildMCPJSONEntry(serverCfg *schema.MCPServerConfig, toolchainPATH string) MCPJSONServer {
	if serverCfg.TransportType() == schema.MCPTransportHTTP {
		return MCPJSONServer{
			Type:    schema.MCPTransportHTTP,
			URL:     serverCfg.URL,
			Headers: copyStringMap(serverCfg.Headers),
		}
	}

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
//
// Each invocation gets a unique path via os.CreateTemp's `*` pattern
// substitution. Earlier versions used a fixed path
// (os.TempDir()/atmos-mcp-config.json) which raced when two concurrent
// `atmos ai ask` (or any other consumer) invocations ran on the same
// machine — the slower writer's content would be silently overwritten
// while the slower reader could see partial JSON.
func WriteMCPConfigToTempFile(servers map[string]schema.MCPServerConfig, toolchainPATH string) (string, error) {
	config := GenerateMCPConfig(servers, toolchainPATH)

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", fmt.Errorf("%w: %w", errUtils.ErrMCPConfigMarshalFailed, err)
	}

	const tempFilePerms = 0o600

	// os.CreateTemp gives us a unique path per invocation. The `*` in the
	// pattern is replaced with a random suffix; the .json extension is
	// preserved so editors / file-browsers detect the file type correctly.
	tmpFile, err := os.CreateTemp("", "atmos-mcp-config-*.json")
	if err != nil {
		return "", fmt.Errorf("%w: %w", errUtils.ErrMCPConfigWriteFailed, err)
	}
	tmpPath := tmpFile.Name()

	// CreateTemp opens with 0600 by default on Unix and the closest
	// equivalent on Windows, but be explicit so the contract is
	// preserved regardless of the OS umask.
	//
	// gosec G703 (path traversal) is a false positive on the os.Chmod /
	// os.Remove calls below: tmpPath comes directly from os.CreateTemp,
	// which returns a path it just constructed with a random suffix in
	// a directory it controls. There's no untrusted-input chain.
	if err := os.Chmod(tmpPath, tempFilePerms); err != nil { //nolint:gosec // tmpPath came from os.CreateTemp; no untrusted input.
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath) //nolint:gosec // tmpPath came from os.CreateTemp; no untrusted input.
		return "", fmt.Errorf("%w: %s: %w", errUtils.ErrMCPConfigWriteFailed, tmpPath, err)
	}

	if _, err := tmpFile.Write(append(data, '\n')); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath) //nolint:gosec // tmpPath came from os.CreateTemp; no untrusted input.
		return "", fmt.Errorf("%w: %s: %w", errUtils.ErrMCPConfigWriteFailed, tmpPath, err)
	}
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpPath) //nolint:gosec // tmpPath came from os.CreateTemp; no untrusted input.
		return "", fmt.Errorf("%w: %s: %w", errUtils.ErrMCPConfigWriteFailed, tmpPath, err)
	}

	return tmpPath, nil
}

// copyEnv returns a copy of the env map with keys uppercased.
// Viper lowercases all YAML map keys, but env vars are conventionally UPPERCASE.
// This restores the expected casing (e.g., aws_region → AWS_REGION).
func copyEnv(env map[string]string) map[string]string {
	return copyStringMapWithKeyFunc(env, strings.ToUpper)
}

func copyStringMap(m map[string]string) map[string]string {
	return copyStringMapWithKeyFunc(m, func(k string) string { return k })
}

func copyStringMapWithKeyFunc(m map[string]string, keyFunc func(string) string) map[string]string {
	result := make(map[string]string, len(m))
	for k, v := range m {
		result[keyFunc(k)] = v
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
