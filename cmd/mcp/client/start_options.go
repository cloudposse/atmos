package client

import (
	"fmt"

	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/dependencies"
	log "github.com/cloudposse/atmos/pkg/logger"
	mcpclient "github.com/cloudposse/atmos/pkg/mcp/client"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// buildStartOptions creates StartOption slice with toolchain and auth resolution
// for CLI management commands (tools, test, status, restart).
// This ensures uvx/npx are available from the Atmos toolchain and credentials
// are injected for servers with auth_identity.
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
		return nil
	}

	if len(deps) == 0 {
		// Fall back to component-based resolution.
		tenv, tenvErr := dependencies.ForComponent(atmosConfig, "terraform", nil, nil)
		if tenvErr == nil && tenv != nil {
			return []mcpclient.StartOption{mcpclient.WithToolchain(tenv)}
		}
		return nil
	}

	tenv, err := dependencies.NewEnvironmentFromDeps(atmosConfig, deps)
	if err != nil {
		log.Debug("Failed to create toolchain environment for MCP", "error", err)
		return nil
	}

	return []mcpclient.StartOption{mcpclient.WithToolchain(tenv)}
}

// buildAuthOption creates an auth StartOption if any configured server needs credentials.
func buildAuthOption(atmosConfig *schema.AtmosConfiguration) []mcpclient.StartOption {
	// Check if any server needs auth.
	needsAuth := false
	for _, s := range atmosConfig.MCP.Servers {
		if s.AuthIdentity != "" {
			needsAuth = true
			break
		}
	}
	if !needsAuth {
		return nil
	}

	mgr, err := auth.CreateAndAuthenticateManagerWithAtmosConfig(
		"", &atmosConfig.Auth, cfg.IdentityFlagSelectValue, atmosConfig,
	)
	if err != nil {
		ui.Error(fmt.Sprintf("Failed to create auth manager for MCP servers: %v", err))
		return nil
	}
	if mgr == nil {
		return nil
	}

	return []mcpclient.StartOption{mcpclient.WithAuthManager(mgr)}
}
