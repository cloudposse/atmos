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
	tenv := resolveToolchainEnvironment(atmosConfig)
	if tenv == nil {
		return nil
	}
	return []mcpclient.StartOption{mcpclient.WithToolchain(tenv)}
}

// buildToolchainPATH returns the toolchain-augmented PATH string for the
// configured atmos toolchain, or "" when no toolchain dependencies are
// resolved. Used by `atmos mcp export` so the exported .mcp.json carries the
// toolchain PATH into the IDE-spawned subprocess (otherwise IDEs can't find
// `uvx` / `npx` when those are only on the Atmos toolchain PATH).
//
// This mirrors buildToolchainOption's resolution chain (`.tool-versions`
// first, then a terraform-component fallback) so the two `mcp` paths agree
// on what "the toolchain" means for a given project.
func buildToolchainPATH(atmosConfig *schema.AtmosConfiguration) string {
	tenv := resolveToolchainEnvironment(atmosConfig)
	if tenv == nil {
		return ""
	}
	return tenv.PATH()
}

// resolveToolchainEnvironment loads the Atmos toolchain environment for the
// current project, trying `.tool-versions` first and falling back to a
// terraform-component resolution. Returns nil when no toolchain is
// configured (callers should treat this as a no-op rather than an error).
func resolveToolchainEnvironment(atmosConfig *schema.AtmosConfiguration) *dependencies.ToolchainEnvironment {
	// Load dependencies from .tool-versions (the standard toolchain source).
	deps, err := dependencies.LoadToolVersionsDependencies(atmosConfig)
	if err != nil {
		log.Debug("Failed to load .tool-versions for MCP toolchain resolution", "error", err)
	} else if len(deps) > 0 {
		tenv, tenvErr := dependencies.NewEnvironmentFromDeps(atmosConfig, deps)
		if tenvErr == nil && tenv != nil {
			return tenv
		}
		log.Debug("Failed to create toolchain environment for MCP", "error", tenvErr)
	}

	// Fall back to component-based resolution.
	tenv, tenvErr := dependencies.ForComponent(atmosConfig, "terraform", nil, nil)
	if tenvErr == nil && tenv != nil {
		return tenv
	}
	return nil
}

// buildAuthOption creates an auth StartOption if any configured server needs
// credentials. The returned option delegates to mcpclient.NewScopedAuthProvider
// which rebuilds the auth manager per-server, applying each server's `env:`
// block (specifically ATMOS_* variables) before loading atmos config and
// resolving identities.
func buildAuthOption(atmosConfig *schema.AtmosConfiguration) []mcpclient.StartOption {
	if !mcpServersNeedAuth(atmosConfig.MCP.Servers) {
		return nil
	}
	return []mcpclient.StartOption{mcpclient.WithAuthManager(mcpclient.NewScopedAuthProvider())}
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
