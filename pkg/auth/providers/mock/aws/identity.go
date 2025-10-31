package aws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// MockRegion is the default AWS region for mock credentials.
	MockRegion = "us-east-1"

	// MockFilePermissions are the file permissions for credential files (owner read/write only).
	MockFilePermissions = 0o600
)

// ErrNoStoredCredentials indicates storage is supported but currently empty.
// This error is returned when LoadCredentials is called before authentication.
var ErrNoStoredCredentials = errors.New("mock identity has no stored credentials")

// Identity is a mock authentication identity for testing purposes only.
// It simulates provider-agnostic credential storage behavior by persisting
// credentials to disk (like AWS writing to ~/.aws/credentials, or GitHub storing
// a token in a file). This allows credentials to persist across process invocations.
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

// getCredentialsFilePath returns the path where mock credentials are stored.
// This simulates how real providers persist credentials to disk.
func (i *Identity) getCredentialsFilePath() string {
	// Use a temp directory that's cleaned up by the OS.
	// In production, real providers would use XDG directories like ~/.config/atmos/aws/{provider}/.
	tmpDir := os.TempDir()
	return filepath.Join(tmpDir, "atmos-mock-"+i.name+".json")
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
		Region:          MockRegion,
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
	env["AWS_REGION"] = MockRegion
	env["AWS_DEFAULT_REGION"] = MockRegion

	return env, nil
}

// PrepareEnvironment prepares environment variables for external processes.
// For mock AWS identities, we set AWS-specific environment variables for testing.
func (i *Identity) PrepareEnvironment(_ context.Context, environ map[string]string) (map[string]string, error) {
	defer perf.Track(nil, "mockaws.Identity.PrepareEnvironment")()

	// Set ATMOS_IDENTITY to the identity name for integration testing.
	environ["ATMOS_IDENTITY"] = i.name

	// Set AWS-specific environment variables for auth exec/shell commands.
	environ["AWS_SHARED_CREDENTIALS_FILE"] = "/tmp/mock-credentials"
	environ["AWS_CONFIG_FILE"] = "/tmp/mock-config"
	environ["AWS_PROFILE"] = i.name
	environ["AWS_REGION"] = MockRegion
	environ["AWS_DEFAULT_REGION"] = MockRegion

	return environ, nil
}

// PostAuthenticate simulates writing credentials to persistent storage.
// For mock identities, this writes credentials to a temporary file to persist them.
// This mimics real provider behavior where authentication results in credentials being written
// to disk (AWS ~/.aws/credentials), environment variables (GitHub token), or other storage.
func (i *Identity) PostAuthenticate(ctx context.Context, params *types.PostAuthenticateParams) error {
	defer perf.Track(nil, "mock.Identity.PostAuthenticate")()

	// Write credentials to disk to simulate persistent storage.
	// Use a fixed timestamp for deterministic testing.
	fixedExpiration := time.Date(MockExpirationYear, MockExpirationMonth, MockExpirationDay, MockExpirationHour, MockExpirationMinute, MockExpirationSecond, 0, time.UTC)

	creds := &Credentials{
		AccessKeyID:     "mock-access-key",
		SecretAccessKey: "mock-secret-key",
		SessionToken:    "mock-session-token",
		Region:          MockRegion,
		Expiration:      fixedExpiration,
	}

	// Serialize credentials to JSON.
	data, err := json.Marshal(creds)
	if err != nil {
		return fmt.Errorf("failed to marshal mock credentials: %w", err)
	}

	// Write to temp file (simulates writing to XDG directory).
	credPath := i.getCredentialsFilePath()
	if err := os.WriteFile(credPath, data, MockFilePermissions); err != nil {
		return fmt.Errorf("failed to write mock credentials to %s: %w", credPath, err)
	}

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

	// Check if credentials file exists.
	credPath := i.getCredentialsFilePath()
	data, err := os.ReadFile(credPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Return typed error to indicate credentials must be obtained via authentication.
			return nil, fmt.Errorf("%w: %q — use 'atmos auth login' to authenticate", ErrNoStoredCredentials, i.name)
		}
		return nil, fmt.Errorf("failed to read mock credentials from %s: %w", credPath, err)
	}

	// Deserialize credentials from JSON.
	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("failed to unmarshal mock credentials: %w", err)
	}

	return &creds, nil
}

// Logout simulates removing credentials from persistent storage.
// This deletes the credentials file, requiring re-authentication.
func (i *Identity) Logout(ctx context.Context) error {
	defer perf.Track(nil, "mock.Identity.Logout")()

	// Delete the credentials file to simulate removal from disk/environment/etc.
	credPath := i.getCredentialsFilePath()
	err := os.Remove(credPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete mock credentials file %s: %w", credPath, err)
	}

	return nil
}
