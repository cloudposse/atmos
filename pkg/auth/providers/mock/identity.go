package mock

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Identity is a mock authentication identity for testing purposes only.
type Identity struct {
	name   string
	config *schema.Identity
}

// NewIdentity creates a new mock identity.
func NewIdentity(name string, config *schema.Identity) *Identity {
	defer perf.Track(nil, "mock.NewIdentity")()

	return &Identity{
		name:   name,
		config: config,
	}
}

// Kind returns the identity kind.
func (i *Identity) Kind() string {
	defer perf.Track(nil, "mock.Identity.Kind")()

	return i.config.Kind
}

// GetProviderName returns the provider name for this identity.
func (i *Identity) GetProviderName() (string, error) {
	defer perf.Track(nil, "mock.Identity.GetProviderName")()

	if i.config.Via != nil && i.config.Via.Provider != "" {
		return i.config.Via.Provider, nil
	}
	return "mock", nil
}

// Authenticate performs mock authentication.
func (i *Identity) Authenticate(ctx context.Context, baseCreds types.ICredentials) (types.ICredentials, error) {
	defer perf.Track(nil, "mock.Identity.Authenticate")()

	// For mock identities, we just return new mock credentials.
	// In a real implementation, this would use baseCreds to assume a role or similar.
	// Use a fixed timestamp far in the future for deterministic testing and snapshot stability.
	// This ensures tests don't become flaky due to expiration checks.
	fixedExpiration := time.Date(MockExpirationYear, MockExpirationMonth, MockExpirationDay, MockExpirationHour, MockExpirationMinute, MockExpirationSecond, 0, time.UTC)
	return &Credentials{
		AccessKeyID:     fmt.Sprintf("MOCK_KEY_%s", i.name),
		SecretAccessKey: fmt.Sprintf("MOCK_SECRET_%s", i.name),
		SessionToken:    fmt.Sprintf("MOCK_TOKEN_%s", i.name),
		Region:          "us-east-1",
		Expiration:      fixedExpiration,
	}, nil
}

// Validate validates the identity configuration.
func (i *Identity) Validate() error {
	defer perf.Track(nil, "mock.Identity.Validate")()

	return nil
}

// Environment returns mock environment variables.
// For mock AWS-like identities, we return file paths similar to real AWS identities.
func (i *Identity) Environment() (map[string]string, error) {
	defer perf.Track(nil, "mock.Identity.Environment")()

	env := map[string]string{
		"MOCK_IDENTITY": i.name,
	}

	// For testing purposes, provide mock AWS config paths that would be used
	// by the AWS SDK. These point to non-existent paths but allow testing
	// of the environment variable flow without exposing real credentials.
	env["AWS_SHARED_CREDENTIALS_FILE"] = "/tmp/mock-credentials"
	env["AWS_CONFIG_FILE"] = "/tmp/mock-config"
	env["AWS_PROFILE"] = i.name

	return env, nil
}

// PostAuthenticate is a no-op for mock identities.
func (i *Identity) PostAuthenticate(ctx context.Context, params *types.PostAuthenticateParams) error {
	defer perf.Track(nil, "mock.Identity.PostAuthenticate")()

	return nil
}

// Logout is a no-op for mock identities.
func (i *Identity) Logout(ctx context.Context) error {
	defer perf.Track(nil, "mock.Identity.Logout")()

	return nil
}
