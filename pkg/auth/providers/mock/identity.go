package mock

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Identity is a mock authentication identity for testing purposes only.
type Identity struct {
	name   string
	config *schema.Identity
}

// NewIdentity creates a new mock identity.
func NewIdentity(name string, config *schema.Identity) *Identity {
	return &Identity{
		name:   name,
		config: config,
	}
}

// Kind returns the identity kind.
func (i *Identity) Kind() string {
	return i.config.Kind
}

// GetProviderName returns the provider name for this identity.
func (i *Identity) GetProviderName() (string, error) {
	if i.config.Via != nil && i.config.Via.Provider != "" {
		return i.config.Via.Provider, nil
	}
	return "mock", nil
}

// Authenticate performs mock authentication.
func (i *Identity) Authenticate(ctx context.Context, baseCreds types.ICredentials) (types.ICredentials, error) {
	// For mock identities, we just return new mock credentials.
	// In a real implementation, this would use baseCreds to assume a role or similar.
	return &Credentials{
		AccessKeyID:     fmt.Sprintf("MOCK_KEY_%s", i.name),
		SecretAccessKey: fmt.Sprintf("MOCK_SECRET_%s", i.name),
		SessionToken:    fmt.Sprintf("MOCK_TOKEN_%s", i.name),
		Region:          "us-east-1",
		Expiration:      time.Now().Add(1 * time.Hour),
	}, nil
}

// Validate validates the identity configuration.
func (i *Identity) Validate() error {
	return nil
}

// Environment returns mock environment variables.
func (i *Identity) Environment() (map[string]string, error) {
	return map[string]string{
		"MOCK_IDENTITY": i.name,
	}, nil
}

// PostAuthenticate is a no-op for mock identities.
func (i *Identity) PostAuthenticate(ctx context.Context, stackInfo *schema.ConfigAndStacksInfo, providerName, identityName string, creds types.ICredentials) error {
	return nil
}
