package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	log "github.com/charmbracelet/log"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/auth/types"
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

	// Get the OIDC token from GitHub Actions environment
	_ = viper.BindEnv("github.oidc.request_token", "ACTIONS_ID_TOKEN_REQUEST_TOKEN")
	token := viper.GetString("github.oidc.request_token")
	if token == "" {
		return nil, fmt.Errorf("%w: ACTIONS_ID_TOKEN_REQUEST_TOKEN not found - ensure job has 'id-token: write' permission", errUtils.ErrAuthenticationFailed)
	}

	_ = viper.BindEnv("github.oidc.request_url", "ACTIONS_ID_TOKEN_REQUEST_URL")
	requestURL := viper.GetString("github.oidc.request_url")
	if requestURL == "" {
		return nil, fmt.Errorf("%w: ACTIONS_ID_TOKEN_REQUEST_URL not found - ensure job has 'id-token: write' permission", errUtils.ErrAuthenticationFailed)
	}

	var aud string
	if p.config != nil && p.config.Spec != nil {
		v, ok := p.config.Spec["audience"].(string)
		if !ok || v == "" {
			return nil, fmt.Errorf("%w: audience is required in provider spec", errUtils.ErrInvalidProviderConfig)
		}
		aud = v
	}

	// Prefer provided ACTIONS_ID_TOKEN if present (avoids external calls in tests/CI),
	// otherwise retrieve a token from the OIDC endpoint.
	_ = viper.BindEnv("github.oidc.id_token", "ACTIONS_ID_TOKEN")
	jwtToken := viper.GetString("github.oidc.id_token")
	if jwtToken == "" {
		var err error
		jwtToken, err = p.getOIDCToken(ctx, requestURL, token, aud)
		if err != nil {
			return nil, fmt.Errorf("%w: failed to get OIDC token: %w", errUtils.ErrAuthenticationFailed, err)
		}
	}

	log.Info("GitHub OIDC authentication successful", "provider", p.name)

	// Return the JWT token as credentials (used by downstream identities)
	return &types.OIDCCredentials{
		Token:    jwtToken,
		Provider: "github",
		Audience: aud,
	}, nil
}

// isGitHubActions checks if we're running in GitHub Actions environment.
func (p *oidcProvider) isGitHubActions() bool {
	_ = viper.BindEnv("github.actions", "GITHUB_ACTIONS")
	return viper.GetString("github.actions") == "true"
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
	// GitHub OIDC provider doesn't require additional configuration
	// It relies on GitHub Actions environment variables
	return nil
}

// Environment returns environment variables for this provider.
func (p *oidcProvider) Environment() (map[string]string, error) {
	// GitHub OIDC provider doesn't set additional environment variables
	// The OIDC token is passed to downstream identities via credentials
	return map[string]string{}, nil
}
