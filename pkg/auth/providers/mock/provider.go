package mock

import (
	"context"
	"time"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// MockExpirationYear is the fixed year used for deterministic testing timestamps.
	// Using a far-future year ensures tests don't become flaky due to expiration checks.
	MockExpirationYear = 2099
	// MockExpirationMonth is the fixed month used for deterministic testing timestamps.
	MockExpirationMonth = 12
	// MockExpirationDay is the fixed day used for deterministic testing timestamps.
	MockExpirationDay = 31
	// MockExpirationHour is the fixed hour used for deterministic testing timestamps.
	MockExpirationHour = 23
	// MockExpirationMinute is the fixed minute used for deterministic testing timestamps.
	MockExpirationMinute = 59
	// MockExpirationSecond is the fixed second used for deterministic testing timestamps.
	MockExpirationSecond = 59
)

// Provider is a mock authentication provider for testing purposes only.
// It simulates authentication without requiring real cloud credentials.
type Provider struct {
	name   string
	config *schema.Provider
}

// NewProvider creates a new mock provider.
func NewProvider(name string, config *schema.Provider) *Provider {
	defer perf.Track(nil, "mock.NewProvider")()

	if config == nil {
		return nil
	}

	return &Provider{
		name:   name,
		config: config,
	}
}

// Kind returns the provider kind.
func (p *Provider) Kind() string {
	defer perf.Track(nil, "mock.Provider.Kind")()

	return p.config.Kind
}

// Name returns the provider name.
func (p *Provider) Name() string {
	defer perf.Track(nil, "mock.Provider.Name")()

	return p.name
}

// PreAuthenticate is a no-op for the mock provider.
func (p *Provider) PreAuthenticate(manager types.AuthManager) error {
	defer perf.Track(nil, "mock.Provider.PreAuthenticate")()

	return nil
}

// Authenticate returns mock credentials.
func (p *Provider) Authenticate(ctx context.Context) (types.ICredentials, error) {
	defer perf.Track(nil, "mock.Provider.Authenticate")()

	// Use a fixed timestamp for deterministic testing.
	// Use 2099-12-31 23:59:59 UTC - a far-future date to avoid expiration-related test failures.
	expiration := time.Date(MockExpirationYear, MockExpirationMonth, MockExpirationDay, MockExpirationHour, MockExpirationMinute, MockExpirationSecond, 0, time.UTC)

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
	defer perf.Track(nil, "mock.Provider.Validate")()

	return nil
}

// Environment returns mock environment variables.
func (p *Provider) Environment() (map[string]string, error) {
	defer perf.Track(nil, "mock.Provider.Environment")()

	return map[string]string{
		"MOCK_PROVIDER": p.name,
	}, nil
}

// PrepareEnvironment prepares environment variables for external processes.
// For mock providers, we don't modify the environment since mock credentials
// are only for testing and don't interact with real cloud SDKs.
func (p *Provider) PrepareEnvironment(_ context.Context, environ map[string]string) (map[string]string, error) {
	defer perf.Track(nil, "mock.Provider.PrepareEnvironment")()

	// Mock providers don't need to modify environment for external processes.
	// Just return the environment unchanged.
	return environ, nil
}

// Logout is a no-op for the mock provider.
func (p *Provider) Logout(ctx context.Context) error {
	defer perf.Track(nil, "mock.Provider.Logout")()

	return nil
}

// GetFilesDisplayPath returns the mock display path.
func (p *Provider) GetFilesDisplayPath() string {
	defer perf.Track(nil, "mock.Provider.GetFilesDisplayPath")()

	return "~/.mock/credentials"
}
