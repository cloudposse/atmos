package client

import (
	"github.com/cloudposse/atmos/pkg/dependencies"
	log "github.com/cloudposse/atmos/pkg/logger"
	mcpclient "github.com/cloudposse/atmos/pkg/mcp/client"
	"github.com/cloudposse/atmos/pkg/schema"
)

// buildStartOptions creates StartOption slice with toolchain and auth resolution
// for CLI management commands (tools, test, status, restart).
// This ensures uvx/npx are available from the Atmos toolchain and credentials
// are injected for servers with identity.
func buildStartOptions(atmosConfig *schema.AtmosConfiguration) []mcpclient.StartOption {
	var opts []mcpclient.StartOption

	// Toolchain resolution.
	opts = append(opts, buildToolchainOption(atmosConfig)...)

	// Auth credential injection.
	opts = append(opts, buildAuthOption(atmosConfig)...)

	return opts
}

// buildToolchainOption creates a toolchain StartOption if .tool-versions or component deps are available.
func buildToolchainOption(atmosConfig *schema.AtmosConfiguration) []mcpclient.StartOption {
	// Load dependencies from .tool-versions (the standard toolchain source).
	deps, err := dependencies.LoadToolVersionsDependencies(atmosConfig)
	if err != nil {
		log.Debug("Failed to load .tool-versions for MCP toolchain resolution", "error", err)
	} else if len(deps) > 0 {
		tenv, tenvErr := dependencies.NewEnvironmentFromDeps(atmosConfig, deps)
		if tenvErr == nil && tenv != nil {
			return []mcpclient.StartOption{mcpclient.WithToolchain(tenv)}
		}
		log.Debug("Failed to create toolchain environment for MCP", "error", tenvErr)
	}

	// Fall back to component-based resolution.
	tenv, tenvErr := dependencies.ForComponent(atmosConfig, "terraform", nil, nil)
	if tenvErr == nil && tenv != nil {
		return []mcpclient.StartOption{mcpclient.WithToolchain(tenv)}
	}
	return nil
}

// buildAuthOption creates an auth StartOption if any configured server needs
// credentials. The returned option uses a per-server auth manager that
// applies each server's `env:` block (specifically ATMOS_* variables) before
// loading atmos config and resolving identities — see
// PerServerAuthManager.ForServer.
func buildAuthOption(atmosConfig *schema.AtmosConfiguration) []mcpclient.StartOption {
	if !mcpServersNeedAuth(atmosConfig.MCP.Servers) {
		return nil
	}

	provider := NewPerServerAuthManager(atmosConfig)
	return []mcpclient.StartOption{mcpclient.WithAuthManager(provider)}
}

// mcpServersNeedAuth returns true if any configured MCP server has identity set.
func mcpServersNeedAuth(servers map[string]schema.MCPServerConfig) bool {
	for _, s := range servers {
		if s.Identity != "" {
			return true
		}
	}
	return false
}
