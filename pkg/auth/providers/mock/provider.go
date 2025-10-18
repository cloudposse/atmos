package mock

import (
	"context"
	"time"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Provider is a mock authentication provider for testing purposes only.
// It simulates authentication without requiring real cloud credentials.
type Provider struct {
	name   string
	config *schema.Provider
}

// NewProvider creates a new mock provider.
func NewProvider(name string, config *schema.Provider) *Provider {
	return &Provider{
		name:   name,
		config: config,
	}
}

// Kind returns the provider kind.
func (p *Provider) Kind() string {
	return p.config.Kind
}

// Name returns the provider name.
func (p *Provider) Name() string {
	return p.name
}

// PreAuthenticate is a no-op for the mock provider.
func (p *Provider) PreAuthenticate(manager types.AuthManager) error {
	return nil
}

// Authenticate returns mock credentials.
func (p *Provider) Authenticate(ctx context.Context) (types.ICredentials, error) {
	// Use a fixed timestamp for deterministic testing.
	// Use 2025-10-18 17:00:00 UTC which is 12:00:00 CDT.
	expiration := time.Date(2025, 10, 18, 17, 0, 0, 0, time.UTC)

	return &Credentials{
		AccessKeyID:     "MOCK_ACCESS_KEY_ID",
		SecretAccessKey: "MOCK_SECRET_ACCESS_KEY",
		SessionToken:    "MOCK_SESSION_TOKEN",
		Region:          "us-east-1",
		Expiration:      expiration,
	}, nil
}

// Validate validates the provider configuration.
func (p *Provider) Validate() error {
	return nil
}

// Environment returns mock environment variables.
func (p *Provider) Environment() (map[string]string, error) {
	return map[string]string{
		"MOCK_PROVIDER": p.name,
	}, nil
}
