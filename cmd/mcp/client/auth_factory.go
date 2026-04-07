package client

import (
	"context"
	"fmt"

	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	mcpclient "github.com/cloudposse/atmos/pkg/mcp/client"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// PerServerAuthManager is an auth provider that constructs the underlying
// auth manager per-server, applying the server's `env:` block (specifically
// ATMOS_* variables) to the parent process environment so that ATMOS_PROFILE,
// ATMOS_CLI_CONFIG_PATH, ATMOS_BASE_PATH, etc. influence atmos config loading
// and identity resolution.
//
// It implements both mcpclient.AuthEnvProvider and mcpclient.PerServerAuthProvider.
// The Atmos MCP client checks for the latter inside WithAuthManager and prefers
// the per-server path when available.
type PerServerAuthManager struct {
	// baseConfig is the parent's already-loaded atmos configuration.
	// Used as a fallback when no server-specific env override is in effect.
	baseConfig *schema.AtmosConfiguration
	// initConfig is overridable for tests.
	initConfig func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error)
	// createAuthManager is overridable for tests.
	createAuthManager func(string, *schema.AuthConfig, string, *schema.AtmosConfiguration) (auth.AuthManager, error)
}

// NewPerServerAuthManager creates a per-server auth manager that uses the
// given base config when no server env override is in effect.
func NewPerServerAuthManager(baseConfig *schema.AtmosConfiguration) *PerServerAuthManager {
	return &PerServerAuthManager{
		baseConfig:        baseConfig,
		initConfig:        cfg.InitCliConfig,
		createAuthManager: auth.CreateAndAuthenticateManagerWithAtmosConfig,
	}
}

// PrepareShellEnvironment satisfies AuthEnvProvider for callers that don't go
// through ForServer (e.g., direct WithAuthManager users without per-server
// awareness). It uses the base config without any env overrides.
func (p *PerServerAuthManager) PrepareShellEnvironment(ctx context.Context, identityName string, currentEnv []string) ([]string, error) {
	defer perf.Track(nil, "cmd.mcp.client.PerServerAuthManager.PrepareShellEnvironment")()

	mgr, err := p.buildManager(nil)
	if err != nil {
		return nil, err
	}
	return mgr.PrepareShellEnvironment(ctx, identityName, currentEnv)
}

// ForServer constructs an auth manager for the given server, applying the
// server's `env:` block to ATMOS_* environment variables in the parent process
// for the duration of manager construction.
func (p *PerServerAuthManager) ForServer(_ context.Context, config *mcpclient.ParsedConfig) (mcpclient.AuthEnvProvider, error) {
	defer perf.Track(nil, "cmd.mcp.client.PerServerAuthManager.ForServer")()

	mgr, err := p.buildManager(config.Env)
	if err != nil {
		return nil, fmt.Errorf("failed to build auth manager for MCP server %q: %w", config.Name, err)
	}
	return mgr, nil
}

// buildManager applies ATMOS_* env overrides, re-loads atmos config, and
// constructs an auth manager from it. The env overrides are restored before
// returning, but the constructed manager already has its identity map populated.
func (p *PerServerAuthManager) buildManager(serverEnv map[string]string) (auth.AuthManager, error) {
	restore := mcpclient.ApplyAtmosEnvOverrides(serverEnv)
	defer restore()

	loadedConfig, err := p.initConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return nil, err
	}

	return p.createAuthManager(
		"", &loadedConfig.Auth, cfg.IdentityFlagSelectValue, &loadedConfig,
	)
}
