package mock

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ErrNoStoredCredentials indicates storage is supported but currently empty.
// This error is returned when LoadCredentials is called before authentication.
var ErrNoStoredCredentials = errors.New("mock identity has no stored credentials")

// Identity is a mock authentication identity for testing purposes only.
// It simulates provider-agnostic credential storage behavior by tracking whether
// credentials have been persisted (like AWS writing to ~/.aws/credentials, or
// GitHub storing a token in an environment variable/file).
type Identity struct {
	name                 string
	config               *schema.Identity
	hasStoredCredentials bool // Tracks if credentials have been written to "storage"
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
	env["AWS_REGION"] = "us-east-1"
	env["AWS_DEFAULT_REGION"] = "us-east-1"

	return env, nil
}

// PrepareEnvironment prepares environment variables for external processes.
// For mock identities, we don't modify the environment since mock credentials
// are only for testing and don't interact with real cloud SDKs.
func (i *Identity) PrepareEnvironment(_ context.Context, environ map[string]string) (map[string]string, error) {
	defer perf.Track(nil, "mock.Identity.PrepareEnvironment")()

	// Mock identities don't need to modify environment for external processes.
	// Just return the environment unchanged.
	return environ, nil
}

// PostAuthenticate simulates writing credentials to persistent storage.
// For mock identities, this tracks that credentials have been "stored" after authentication.
// This mimics real provider behavior where authentication results in credentials being written
// to disk (AWS ~/.aws/credentials), environment variables (GitHub token), or other storage.
func (i *Identity) PostAuthenticate(ctx context.Context, params *types.PostAuthenticateParams) error {
	defer perf.Track(nil, "mock.Identity.PostAuthenticate")()

	// Mark that credentials have been written to "storage".
	// This allows LoadCredentials to succeed on subsequent calls.
	i.hasStoredCredentials = true

	return nil
}

// CredentialsExist always returns true for mock identities (credentials are in-memory).
func (i *Identity) CredentialsExist() (bool, error) {
	defer perf.Track(nil, "mock.Identity.CredentialsExist")()

	// Mock identities don't use file-based storage.
	// Credentials are always available in-memory.
	return true, nil
}

// LoadCredentials simulates loading credentials from persistent storage.
// This method implements provider-agnostic credential loading behavior:
// - Returns ErrNoStoredCredentials if credentials haven't been stored yet (no authentication performed).
// - Returns credentials if they were previously stored via PostAuthenticate.
//
// This mimics real provider behavior across different storage mechanisms:
// - AWS: Loading from XDG directories (~/.config/atmos/aws/{provider}/) after SSO login.
// - GitHub: Loading token from environment variable or file.
// - Azure: Loading from XDG directories after authentication.
// - Google Cloud: Loading from XDG directories after auth.
func (i *Identity) LoadCredentials(ctx context.Context) (types.ICredentials, error) {
	defer perf.Track(nil, "mock.Identity.LoadCredentials")()

	// Check if credentials have been stored (via PostAuthenticate).
	if !i.hasStoredCredentials {
		// Return a typed error to indicate credentials must be obtained via authentication.
		return nil, fmt.Errorf("%w: %q — use 'atmos auth login' to authenticate", ErrNoStoredCredentials, i.name)
	}

	// Use a fixed timestamp far in the future for deterministic testing and snapshot stability.
	// This ensures tests don't become flaky due to expiration checks.
	fixedExpiration := time.Date(MockExpirationYear, MockExpirationMonth, MockExpirationDay, MockExpirationHour, MockExpirationMinute, MockExpirationSecond, 0, time.UTC)

	// Return stored credentials.
	// In a real provider, this would read from disk/environment/etc.
	return &Credentials{
		AccessKeyID:     "mock-access-key",
		SecretAccessKey: "mock-secret-key",
		SessionToken:    "mock-session-token",
		Region:          "us-east-1",
		Expiration:      fixedExpiration,
	}, nil
}

// Logout simulates removing credentials from persistent storage.
// This clears the stored credentials state, requiring re-authentication.
func (i *Identity) Logout(ctx context.Context) error {
	defer perf.Track(nil, "mock.Identity.Logout")()

	// Clear the stored credentials flag.
	// This simulates removing credentials from disk/environment/etc.
	i.hasStoredCredentials = false

	return nil
}
