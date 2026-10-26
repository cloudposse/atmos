// Package atmospro implements the `atmos/pro` auth provider, which authenticates the
// Atmos CLI *to Atmos Pro* by federating a GitHub Actions OIDC token through the
// Atmos Pro auth endpoint and caching the resulting session JWT.
//
// v1 is OIDC-only: it requires a GitHub Actions environment. The session JWT it
// returns is reusable for all Atmos Pro API calls (e.g., the github/sts integration).
package atmospro

import (
	"context"
	"fmt"

	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	githubProviders "github.com/cloudposse/atmos/pkg/auth/providers/github"
	"github.com/cloudposse/atmos/pkg/auth/types"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/pro"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Kind is the provider kind for the Atmos Pro provider.
const Kind = "atmos/pro"

// mintOIDCToken mints a GitHub Actions OIDC token for the given audience.
// Overridable in tests. It reuses the github/oidc provider, which handles env
// reading (ACTIONS_ID_TOKEN_REQUEST_*), SSRF validation, and the not-in-Actions error.
var mintOIDCToken = func(ctx context.Context, audience string) (string, error) {
	inner, err := githubProviders.NewOIDCProvider("atmos-pro-oidc", &schema.Provider{
		Kind: "github/oidc",
		Spec: map[string]interface{}{"audience": audience},
	})
	if err != nil {
		return "", err
	}
	creds, err := inner.Authenticate(ctx)
	if err != nil {
		return "", err
	}
	oidc, ok := creds.(*types.OIDCCredentials)
	if !ok || oidc.Token == "" {
		return "", fmt.Errorf("%w: empty OIDC token", errUtils.ErrProAuthFailed)
	}
	return oidc.Token, nil
}

// exchangeOIDCToken exchanges a GitHub OIDC token for an Atmos Pro session JWT.
// Overridable in tests.
var exchangeOIDCToken = pro.ExchangeOIDCToken

// proProvider implements the atmos/pro provider.
type proProvider struct {
	name   string
	config *schema.Provider
	realm  string
}

// NewProvider creates a new atmos/pro provider.
func NewProvider(name string, config *schema.Provider) (types.Provider, error) {
	if config == nil {
		return nil, fmt.Errorf("%w: provider config is required", errUtils.ErrInvalidProviderConfig)
	}
	if name == "" {
		return nil, fmt.Errorf("%w: provider name is required", errUtils.ErrInvalidProviderConfig)
	}
	return &proProvider{name: name, config: config}, nil
}

// Kind returns the provider kind.
func (p *proProvider) Kind() string { return Kind }

// Name returns the provider name.
func (p *proProvider) Name() string { return p.name }

// SetRealm sets the credential isolation realm for this provider.
func (p *proProvider) SetRealm(realm string) { p.realm = realm }

// PreAuthenticate is a no-op for the Atmos Pro provider.
func (p *proProvider) PreAuthenticate(_ types.AuthManager) error { return nil }

// Authenticate federates a GitHub Actions OIDC token through Atmos Pro and returns
// the resulting session JWT as ProCredentials.
func (p *proProvider) Authenticate(ctx context.Context) (types.ICredentials, error) {
	defer perf.Track(nil, "atmospro.proProvider.Authenticate")()

	audience := p.audience()
	baseURL := p.baseURL()
	endpoint := p.endpoint()
	workspaceID := p.workspaceID()

	if workspaceID == "" {
		return nil, errUtils.ErrProWorkspaceIDMissing
	}

	log.Debug("Starting Atmos Pro authentication", "provider", p.name, "audience", audience, "base_url", baseURL)

	// The OIDC token is single-use server-side — mint exactly once, do not retry the exchange with it.
	oidcToken, err := mintOIDCToken(ctx, audience)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrProAuthFailed, err)
	}

	sessionJWT, err := exchangeOIDCToken(baseURL, endpoint, oidcToken, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrProAuthFailed, err)
	}

	log.Debug("Atmos Pro authentication successful", "provider", p.name)

	return &types.ProCredentials{
		Token:       sessionJWT,
		BaseURL:     baseURL,
		Endpoint:    endpoint,
		WorkspaceID: workspaceID,
		Provider:    "atmos-pro",
	}, nil
}

// Validate validates the provider configuration. The workspace_id is required but may be
// supplied via env (ATMOS_PRO_WORKSPACE_ID), so it cannot be fully validated at config time.
func (p *proProvider) Validate() error { return nil }

// Environment returns environment variables for this provider (none).
func (p *proProvider) Environment() (map[string]string, error) {
	return map[string]string{}, nil
}

// Paths returns credential files/directories used by this provider (none).
func (p *proProvider) Paths() ([]types.Path, error) {
	return []types.Path{}, nil
}

// PrepareEnvironment returns the environment unchanged.
func (p *proProvider) PrepareEnvironment(_ context.Context, environ map[string]string) (map[string]string, error) {
	defer perf.Track(nil, "atmospro.proProvider.PrepareEnvironment")()
	return environ, nil
}

// Logout is a no-op (session JWT lives only in the keyring, handled by the manager).
func (p *proProvider) Logout(_ context.Context) error {
	log.Debug("Logout not supported for Atmos Pro provider (no files to clean up)", "provider", p.name)
	return errUtils.ErrLogoutNotSupported
}

// GetFilesDisplayPath returns the display path for credential files (none).
func (p *proProvider) GetFilesDisplayPath() string { return "" }

// audience resolves the OIDC audience from spec, defaulting to the Atmos Pro audience.
func (p *proProvider) audience() string {
	if v := p.specString("audience"); v != "" {
		return v
	}
	return pro.DefaultProAudience
}

// baseURL resolves the Atmos Pro base URL: spec.base_url → ATMOS_PRO_BASE_URL → default.
func (p *proProvider) baseURL() string {
	if v := p.specString("base_url"); v != "" {
		return v
	}
	if v := envString(cfg.AtmosProBaseUrlEnvVarName); v != "" {
		return v
	}
	return cfg.AtmosProDefaultBaseUrl
}

// endpoint resolves the Atmos Pro API endpoint: spec.endpoint → ATMOS_PRO_ENDPOINT → default.
func (p *proProvider) endpoint() string {
	if v := p.specString("endpoint"); v != "" {
		return v
	}
	if v := envString(cfg.AtmosProEndpointEnvVarName); v != "" {
		return v
	}
	return cfg.AtmosProDefaultEndpoint
}

// workspaceID resolves the workspace ID: spec.workspace_id → ATMOS_PRO_WORKSPACE_ID.
func (p *proProvider) workspaceID() string {
	if v := p.specString("workspace_id"); v != "" {
		return v
	}
	return envString(cfg.AtmosProWorkspaceIDEnvVarName)
}

// specString returns a string value from the provider spec, or "" if absent.
func (p *proProvider) specString(key string) string {
	if p.config == nil || p.config.Spec == nil {
		return ""
	}
	if v, ok := p.config.Spec[key].(string); ok {
		return v
	}
	return ""
}

// envString reads an environment variable via viper (consistent with sibling providers).
func envString(name string) string {
	if err := viper.BindEnv(name, name); err != nil {
		log.Trace("Failed to bind environment variable", "var", name, "error", err)
	}
	return viper.GetString(name)
}
