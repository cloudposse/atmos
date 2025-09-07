package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/charmbracelet/log"

	"github.com/cloudposse/atmos/internal/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// oidcProvider implements GitHub OIDC authentication
type oidcProvider struct {
	name   string
	config *schema.Provider
}

// NewOIDCProvider creates a new GitHub OIDC provider
func NewOIDCProvider(name string, config *schema.Provider) (types.Provider, error) {
	if config == nil {
		return nil, fmt.Errorf("provider config is required")
	}

	if name == "" {
		return nil, fmt.Errorf("provider name is required")
	}

	return &oidcProvider{
		name:   name,
		config: config,
	}, nil
}

// Name returns the provider name
func (p *oidcProvider) Name() string {
	return p.name
}

// Kind returns the provider kind
func (p *oidcProvider) Kind() string {
	return "github/oidc"
}

// Authenticate performs GitHub OIDC authentication
func (p *oidcProvider) Authenticate(ctx context.Context) (*schema.Credentials, error) {
	log.Info("Starting GitHub OIDC authentication", "provider", p.name)

	// Check if we're running in GitHub Actions
	if !p.isGitHubActions() {
		return nil, fmt.Errorf("GitHub OIDC authentication is only available in GitHub Actions environment")
	}

	// Get the OIDC token from GitHub Actions environment
	token := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("ACTIONS_ID_TOKEN_REQUEST_TOKEN not found - ensure job has 'id-token: write' permission")
	}

	requestURL := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_URL")
	if requestURL == "" {
		return nil, fmt.Errorf("ACTIONS_ID_TOKEN_REQUEST_URL not found - ensure job has 'id-token: write' permission")
	}

	var aud string
	if p.config != nil && p.config.Spec != nil {
		if v, ok := p.config.Spec["audience"].(string); ok && v != "" {
			aud = v
		} else {
			return nil, fmt.Errorf("audience is required in provider spec")
		}
	}

	// Get the JWT token from GitHub's OIDC endpoint.
	jwtToken, err := p.getOIDCToken(ctx, requestURL, token, aud)
	if err != nil {
		return nil, fmt.Errorf("failed to get OIDC token: %w", err)
	}

	log.Info("GitHub OIDC authentication successful", "provider", p.name)

	// Return the JWT token as credentials
	// This will be used by downstream AWS assume role identity
	return &schema.Credentials{
		OIDC: &schema.OIDCCredentials{
			Token:    jwtToken,
			Provider: "github",
			Audience: aud,
		},
	}, nil
}

// isGitHubActions checks if we're running in GitHub Actions environment
func (p *oidcProvider) isGitHubActions() bool {
	return os.Getenv("GITHUB_ACTIONS") == "true"
}

// getOIDCToken retrieves the JWT token from GitHub's OIDC endpoint
func (p *oidcProvider) getOIDCToken(ctx context.Context, requestURL, requestToken, audience string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return "", fmt.Errorf("create OIDC request: %w", err)
	}
	q := req.URL.Query()
	if audience != "" {
		q.Set("audience", audience)
	}
	req.URL.RawQuery = q.Encode()
	req.Header.Set("Authorization", "bearer "+requestToken)
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("call OIDC endpoint: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("OIDC endpoint returned status %s", resp.Status)
	}
	var out struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode OIDC response: %w", err)
	}
	if out.Value == "" {
		return "", fmt.Errorf("empty token in OIDC response")
	}
	return out.Value, nil
}

// Validate validates the provider configuration
func (p *oidcProvider) Validate() error {
	// GitHub OIDC provider doesn't require additional configuration
	// It relies on GitHub Actions environment variables
	return nil
}

// Environment returns environment variables for this provider
func (p *oidcProvider) Environment() (map[string]string, error) {
	// GitHub OIDC provider doesn't set additional environment variables
	// The OIDC token is passed to downstream identities via credentials
	return map[string]string{}, nil
}
