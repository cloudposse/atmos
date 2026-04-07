package client

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ScopedAuthProvider is a thin MCP adapter over the generic
// auth.CreateAndAuthenticateManagerWithEnvOverrides primitive. It implements
// both AuthEnvProvider and PerServerAuthProvider so WithAuthManager will
// dispatch through ForServer and build a server-scoped auth manager.
//
// The actual per-server rebuild logic — applying ATMOS_* env vars,
// re-loading the atmos config, and constructing+authenticating the manager —
// lives in pkg/auth. This type only contributes the MCP-specific glue:
// plumbing ParsedConfig.Env into the primitive and wrapping failures with
// server + identity context so callers can errors.Is-match the sentinel.
//
// MCP is the only current consumer with an N:1 need (multiple servers per
// invocation, each potentially loading a different atmos profile), but if
// another subsystem ever needs scoped auth it calls the pkg/auth primitive
// directly — not this adapter.
type ScopedAuthProvider struct {
	// baseConfig is retained for future extensibility (e.g., fallback when no
	// env override is in effect). Currently unused because every call path
	// goes through the env-overrides primitive.
	baseConfig *schema.AtmosConfiguration

	// buildManagerFn is overridable for tests. Production code always uses
	// auth.CreateAndAuthenticateManagerWithEnvOverrides.
	buildManagerFn func(map[string]string) (auth.AuthManager, error)
}

// NewScopedAuthProvider creates a ScopedAuthProvider using the given base
// config as future fallback context.
func NewScopedAuthProvider(baseConfig *schema.AtmosConfiguration) *ScopedAuthProvider {
	return &ScopedAuthProvider{
		baseConfig:     baseConfig,
		buildManagerFn: auth.CreateAndAuthenticateManagerWithEnvOverrides,
	}
}

// PrepareShellEnvironment satisfies AuthEnvProvider as a fallback path for
// callers that don't dispatch through ForServer. It builds an auth manager
// with no env overrides (i.e., using the parent's current environment).
func (p *ScopedAuthProvider) PrepareShellEnvironment(ctx context.Context, identityName string, currentEnv []string) ([]string, error) {
	defer perf.Track(nil, "mcp.client.ScopedAuthProvider.PrepareShellEnvironment")()

	mgr, err := p.buildManagerFn(nil)
	if err != nil {
		return nil, err
	}
	// CreateAndAuthenticateManagerWithEnvOverrides returns (nil, nil) when
	// auth is disabled or no default identity is configured. Guard the
	// nil-pointer dereference with the sentinel error instead.
	if mgr == nil {
		return nil, fmt.Errorf("%w: identity %q", errUtils.ErrMCPServerAuthUnavailable, identityName)
	}
	return mgr.PrepareShellEnvironment(ctx, identityName, currentEnv)
}

// ForServer implements PerServerAuthProvider. It asks pkg/auth to construct
// an auth manager with the server's `env:` block applied to ATMOS_* variables.
func (p *ScopedAuthProvider) ForServer(_ context.Context, config *ParsedConfig) (AuthEnvProvider, error) {
	defer perf.Track(nil, "mcp.client.ScopedAuthProvider.ForServer")()

	mgr, err := p.buildManagerFn(config.Env)
	if err != nil {
		return nil, fmt.Errorf("failed to build auth manager for MCP server %q: %w", config.Name, err)
	}
	// (nil, nil) means auth disabled or no identity resolved — a server with
	// an explicit identity cannot proceed. Surface the sentinel error with
	// server+identity context for errors.Is-matching and display.
	if mgr == nil {
		return nil, fmt.Errorf("%w: server %q, identity %q", errUtils.ErrMCPServerAuthUnavailable, config.Name, config.Identity)
	}
	return mgr, nil
}
