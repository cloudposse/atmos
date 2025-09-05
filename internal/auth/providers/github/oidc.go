package github

import (
	"context"
	"fmt"
	"os"

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

	// Get the JWT token from GitHub's OIDC endpoint
	jwtToken, err := p.getOIDCToken(ctx, requestURL, token)
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
			Audience: "sts.amazonaws.com", // Standard audience for AWS
		},
	}, nil
}

// isGitHubActions checks if we're running in GitHub Actions environment
func (p *oidcProvider) isGitHubActions() bool {
	return os.Getenv("GITHUB_ACTIONS") == "true"
}

// getOIDCToken retrieves the JWT token from GitHub's OIDC endpoint
func (p *oidcProvider) getOIDCToken(ctx context.Context, requestURL, requestToken string) (string, error) {
	// This would typically make an HTTP request to GitHub's OIDC endpoint
	// For now, we'll check if the token is available directly from the environment
	// In a real implementation, we'd make a request to requestURL with the requestToken
	
	// GitHub Actions provides the token directly in some cases
	if directToken := os.Getenv("ACTIONS_ID_TOKEN"); directToken != "" {
		return directToken, nil
	}

	// TODO: Implement HTTP request to requestURL with requestToken
	// This requires making a POST request to the GitHub OIDC endpoint
	return "", fmt.Errorf("OIDC token retrieval not yet implemented - need to make HTTP request to %s", requestURL)
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
