package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

const OidcTimeout = 10

// oidcProvider implements GitHub OIDC authentication.
type oidcProvider struct {
	name   string
	config *schema.Provider
}

// NewOIDCProvider creates a new GitHub OIDC provider.
func NewOIDCProvider(name string, config *schema.Provider) (types.Provider, error) {
	if config == nil {
		return nil, fmt.Errorf("%w: provider config is required", errUtils.ErrInvalidProviderConfig)
	}

	if name == "" {
		return nil, fmt.Errorf("%w: provider name is required", errUtils.ErrInvalidProviderConfig)
	}

	return &oidcProvider{
		name:   name,
		config: config,
	}, nil
}

// Name returns the provider name.
func (p *oidcProvider) Name() string {
	return p.name
}

// PreAuthenticate is a no-op for GitHub OIDC provider.
func (p *oidcProvider) PreAuthenticate(_ types.AuthManager) error {
	return nil
}

// Kind returns the provider kind.
func (p *oidcProvider) Kind() string {
	return "github/oidc"
}

// Authenticate performs GitHub OIDC authentication.
func (p *oidcProvider) Authenticate(ctx context.Context) (types.ICredentials, error) {
	log.Info("Starting GitHub OIDC authentication", "provider", p.name)

	// Validate provider configuration early.
	if err := p.Validate(); err != nil {
		return nil, err
	}

	// Check if we're running in GitHub Actions.
	if !p.isGitHubActions() {
		return nil, fmt.Errorf("%w: GitHub OIDC authentication is only available in GitHub Actions environment", errUtils.ErrAuthenticationFailed)
	}

	requestURL, token, err := p.requestParams()
	if err != nil {
		return nil, err
	}

	aud, err := p.audience()
	if err != nil {
		return nil, err
	}

	jwtToken, err := p.resolveJWT(ctx, requestURL, token, aud)
	if err != nil {
		return nil, err
	}

	log.Info("GitHub OIDC authentication successful", "provider", p.name)

	// Return the JWT token as credentials (used by downstream identities).
	return &types.OIDCCredentials{
		Token:    jwtToken,
		Provider: "github",
		Audience: aud,
	}, nil
}

// isGitHubActions checks if we're running in GitHub Actions environment.
func (p *oidcProvider) isGitHubActions() bool {
	if err := viper.BindEnv("github.actions", "GITHUB_ACTIONS"); err != nil {
		log.Trace("Failed to bind github.actions environment variable", "error", err)
	}
	return viper.GetString("github.actions") == "true"
}

// requestParams loads the request URL and token from the GitHub Actions environment.
func (p *oidcProvider) requestParams() (string, string, error) {
	if err := viper.BindEnv("github.oidc.request_token", "ACTIONS_ID_TOKEN_REQUEST_TOKEN"); err != nil {
		log.Trace("Failed to bind github.oidc.request_token environment variable", "error", err)
	}
	token := viper.GetString("github.oidc.request_token")
	if token == "" {
		return "", "", fmt.Errorf("%w: ACTIONS_ID_TOKEN_REQUEST_TOKEN not found - ensure job has 'id-token: write' permission", errUtils.ErrAuthenticationFailed)
	}

	if err := viper.BindEnv("github.oidc.request_url", "ACTIONS_ID_TOKEN_REQUEST_URL"); err != nil {
		log.Trace("Failed to bind github.oidc.request_url environment variable", "error", err)
	}
	requestURL := viper.GetString("github.oidc.request_url")
	if requestURL == "" {
		return "", "", fmt.Errorf("%w: ACTIONS_ID_TOKEN_REQUEST_URL not found - ensure job has 'id-token: write' permission", errUtils.ErrAuthenticationFailed)
	}
	return requestURL, token, nil
}

// audience extracts the required audience from provider config.
func (p *oidcProvider) audience() (string, error) {
	if p.config != nil && p.config.Spec != nil {
		if v, ok := p.config.Spec["audience"].(string); ok && v != "" {
			return v, nil
		}
		return "", fmt.Errorf("%w: audience is required in provider spec", errUtils.ErrInvalidProviderConfig)
	}
	return "", nil
}

// resolveJWT returns an ACTIONS_ID_TOKEN if present, otherwise fetches a token from the endpoint.
func (p *oidcProvider) resolveJWT(ctx context.Context, requestURL, token, aud string) (string, error) {
	if err := viper.BindEnv("github.oidc.id_token", "ACTIONS_ID_TOKEN"); err != nil {
		log.Trace("Failed to bind github.oidc.id_token environment variable", "error", err)
	}
	if jwt := viper.GetString("github.oidc.id_token"); jwt != "" {
		return jwt, nil
	}
	jwtToken, err := p.getOIDCToken(ctx, requestURL, token, aud)
	if err != nil {
		return "", errors.Join(errUtils.ErrAuthenticationFailed, err)
	}
	return jwtToken, nil
}

// getOIDCToken retrieves the JWT token from GitHub's OIDC endpoint.
func (p *oidcProvider) getOIDCToken(ctx context.Context, requestURL, requestToken, audience string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return "", fmt.Errorf("%w: create OIDC request: %w", errUtils.ErrAuthenticationFailed, err)
	}
	q := req.URL.Query()
	if audience != "" {
		q.Set("audience", audience)
	}
	req.URL.RawQuery = q.Encode()
	req.Header.Set("Authorization", "bearer "+requestToken)
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: OidcTimeout * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: call OIDC endpoint: %w", errUtils.ErrAuthenticationFailed, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w: OIDC endpoint returned status %s", errUtils.ErrAuthenticationFailed, resp.Status)
	}
	var out struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("%w: decode OIDC response: %w", errUtils.ErrAuthenticationFailed, err)
	}
	if out.Value == "" {
		return "", fmt.Errorf("%w: empty token in OIDC response", errUtils.ErrAuthenticationFailed)
	}
	return out.Value, nil
}

// Validate validates the provider configuration.
func (p *oidcProvider) Validate() error {
	audience, err := p.audience()
	if err != nil {
		return err
	}
	if audience == "" {
		return fmt.Errorf("%w: audience is required in provider spec", errUtils.ErrInvalidProviderConfig)
	}
	return nil
}

// Environment returns environment variables for this provider.
func (p *oidcProvider) Environment() (map[string]string, error) {
	// GitHub OIDC provider doesn't set additional environment variables.
	// The OIDC token is passed to downstream identities via credentials.
	return map[string]string{}, nil
}

// Logout removes provider-specific credential storage.
func (p *oidcProvider) Logout(ctx context.Context) error {
	// GitHub OIDC provider has no logout concept - tokens come from GitHub Actions environment.
	// Credentials are only stored in keyring (handled by AuthManager).
	// Return ErrLogoutNotSupported to indicate successful no-op (exit 0).
	log.Debug("Logout not supported for GitHub OIDC provider (no files to clean up)", "provider", p.name)
	return errUtils.ErrLogoutNotSupported
}

// GetFilesDisplayPath returns the display path for credential files.
// GitHub OIDC provider doesn't use file-based credentials.
func (p *oidcProvider) GetFilesDisplayPath() string {
	return "" // No files for GitHub OIDC provider
}
