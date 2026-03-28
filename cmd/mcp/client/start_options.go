package client

import (
	"github.com/cloudposse/atmos/pkg/dependencies"
	log "github.com/cloudposse/atmos/pkg/logger"
	mcpclient "github.com/cloudposse/atmos/pkg/mcp/client"
	"github.com/cloudposse/atmos/pkg/schema"
)

// buildStartOptions creates StartOption slice with toolchain resolution for CLI commands.
// This ensures uvx/npx are available from the Atmos toolchain when starting MCP servers
// from management commands (tools, test, status, restart).
//
// It loads tool dependencies from .tool-versions and creates a ToolchainEnvironment
// that resolves binary paths and provides the toolchain PATH.
func buildStartOptions(atmosConfig *schema.AtmosConfiguration) []mcpclient.StartOption {
	var opts []mcpclient.StartOption

	// Load dependencies from .tool-versions (the standard toolchain source).
	deps, err := dependencies.LoadToolVersionsDependencies(atmosConfig)
	if err != nil {
		log.Debug("Failed to load .tool-versions for MCP toolchain resolution", "error", err)
		return opts
	}

	if len(deps) == 0 {
		// Fall back to component-based resolution.
		tenv, tenvErr := dependencies.ForComponent(atmosConfig, "terraform", nil, nil)
		if tenvErr == nil && tenv != nil {
			opts = append(opts, mcpclient.WithToolchain(tenv))
		}
		return opts
	}

	tenv, err := dependencies.NewEnvironmentFromDeps(atmosConfig, deps)
	if err != nil {
		log.Debug("Failed to create toolchain environment for MCP", "error", err)
		return opts
	}

	opts = append(opts, mcpclient.WithToolchain(tenv))
	return opts
}
