package auth

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestSessionTokenDoesNotOverwriteLongLivedCredentialsInKeyring verifies that session tokens
// (temporary credentials) do NOT overwrite long-lived credentials in the keyring.
//
// Intended behavior:
// - Session tokens should NOT be cached in keyring
// - Long-lived credentials should remain in keyring unchanged
// - Session tokens should only exist in provider-specific storage (AWS files, etc.)
//
// This ensures users don't need to reconfigure credentials after authentication,
// as long-lived credentials remain available for generating new session tokens.
func TestSessionTokenDoesNotOverwriteLongLivedCredentialsInKeyring(t *testing.T) {
	ctx := context.Background()

	// Step 1: Store long-lived AWS credentials in keyring.
	// This simulates what `atmos auth user configure` does.
	longLivedCreds := &types.AWSCredentials{
		AccessKeyID:     "AKIA_LONG_LIVED_KEY",
		SecretAccessKey: "long_lived_secret_key",
		MfaArn:          "arn:aws:iam::123456789012:mfa/test-user",
		SessionDuration: "12h",
		Region:          "us-east-1",
		// NO SessionToken - this is a long-lived credential
	}

	// Create test credential store with long-lived credentials.
	// Pre-populate keyring with long-lived credentials for both provider and identity.
	// This simulates what `atmos auth user configure` does.
	store := &testStore{
		data: map[string]any{
			"test-provider": longLivedCreds, // Provider credentials (long-lived)
			"test-identity": longLivedCreds, // Identity credentials (long-lived) - should NOT be overwritten
		},
		expired: map[string]bool{
			"test-provider": false,
			"test-identity": false,
		},
	}

	// Step 2: Create a mock identity that returns session tokens.
	// This simulates what AWS user identity does after calling STS GetSessionToken.
	sessionCreds := &types.AWSCredentials{
		AccessKeyID:     "ASIA_SESSION_KEY",
		SecretAccessKey: "session_secret_key",
		SessionToken:    "session_token_12345", // Session tokens have this field
		Region:          "us-east-1",
		Expiration:      "2099-12-31T23:59:59Z",
	}

	mockIdentity := &mockIdentityReturningSessionTokens{
		sessionCreds: sessionCreds,
	}

	// Step 3: Create auth manager with minimal setup.
	authConfig := &schema.AuthConfig{
		Providers: map[string]schema.Provider{
			"test-provider": {
				Kind: "test",
			},
		},
		Identities: map[string]schema.Identity{
			"test-identity": {
				Kind: "test",
				Via: &schema.IdentityVia{
					Provider: "test-provider",
				},
			},
		},
	}

	m := &manager{
		config:          authConfig,
		credentialStore: store,
		providers:       map[string]types.Provider{},
		identities: map[string]types.Identity{
			"test-identity": mockIdentity,
		},
		chain: []string{"test-provider", "test-identity"},
	}

	// Step 4: Authenticate through the identity chain.
	// This returns session tokens but should NOT cache them in keyring.
	returnedCreds, err := m.authenticateIdentityChain(ctx, 1, longLivedCreds)
	require.NoError(t, err, "authenticateIdentityChain should succeed")

	// Verify that session tokens were returned (this is correct and expected).
	awsCreds, ok := returnedCreds.(*types.AWSCredentials)
	require.True(t, ok, "Returned credentials should be AWS credentials")
	assert.NotEmpty(t, awsCreds.SessionToken, "Should return session tokens")
	assert.Equal(t, "ASIA_SESSION_KEY", awsCreds.AccessKeyID, "Should return session access key")

	// Step 5: Verify keyring STILL contains long-lived credentials (INTENDED BEHAVIOR).
	// Session tokens should NOT have been cached in keyring.
	retrievedCreds, err := store.Retrieve("test-identity")
	require.NoError(t, err, "Should retrieve credentials from keyring")

	retrievedAWSCreds, ok := retrievedCreds.(*types.AWSCredentials)
	require.True(t, ok, "Retrieved credentials should be AWS credentials")

	// INTENDED BEHAVIOR: Keyring should NOT contain session tokens.
	assert.Empty(t, retrievedAWSCreds.SessionToken,
		"Keyring should NOT contain session tokens - they should only be in provider storage (AWS files)")

	// INTENDED BEHAVIOR: Keyring should STILL contain long-lived credentials.
	assert.Equal(t, "AKIA_LONG_LIVED_KEY", retrievedAWSCreds.AccessKeyID,
		"Keyring should preserve long-lived access key")

	assert.Equal(t, "long_lived_secret_key", retrievedAWSCreds.SecretAccessKey,
		"Keyring should preserve long-lived secret key")

	// INTENDED BEHAVIOR: Keyring should preserve MFA ARN and session duration.
	assert.Equal(t, "arn:aws:iam::123456789012:mfa/test-user", retrievedAWSCreds.MfaArn,
		"Keyring should preserve MFA ARN for future session token generation")

	assert.Equal(t, "12h", retrievedAWSCreds.SessionDuration,
		"Keyring should preserve session duration for future session token generation")
}

// mockIdentityReturningSessionTokens is a mock identity that returns session tokens.
// This simulates what AWS user identity does after calling STS GetSessionToken.
type mockIdentityReturningSessionTokens struct {
	sessionCreds *types.AWSCredentials
}

func (m *mockIdentityReturningSessionTokens) Kind() string {
	return "test"
}

func (m *mockIdentityReturningSessionTokens) Authenticate(ctx context.Context, baseCreds types.ICredentials) (types.ICredentials, error) {
	// Return session tokens (credentials WITH SessionToken field).
	// This simulates what generateSessionToken() does in AWS user identity.
	return m.sessionCreds, nil
}

func (m *mockIdentityReturningSessionTokens) Validate() error {
	return nil
}

func (m *mockIdentityReturningSessionTokens) Environment() (map[string]string, error) {
	return map[string]string{}, nil
}

func (m *mockIdentityReturningSessionTokens) Paths() ([]types.Path, error) {
	return []types.Path{}, nil
}

func (m *mockIdentityReturningSessionTokens) PrepareEnvironment(ctx context.Context, environ map[string]string) (map[string]string, error) {
	return environ, nil
}

func (m *mockIdentityReturningSessionTokens) PostAuthenticate(ctx context.Context, params *types.PostAuthenticateParams) error {
	return nil
}

func (m *mockIdentityReturningSessionTokens) CredentialsExist() (bool, error) {
	return true, nil
}

func (m *mockIdentityReturningSessionTokens) LoadCredentials(ctx context.Context) (types.ICredentials, error) {
	// Return session credentials from "storage" (simulates loading from AWS files).
	return m.sessionCreds, nil
}

func (m *mockIdentityReturningSessionTokens) Logout(ctx context.Context) error {
	return nil
}

func (m *mockIdentityReturningSessionTokens) GetProviderName() (string, error) {
	return "test-provider", nil
}

// TestBuildWhoamiInfo_SkipsSessionTokenCaching verifies that buildWhoamiInfo
// does NOT cache session tokens in the keyring.
//
// This is a critical test because buildWhoamiInfo is called after authentication
// completes, and if it cached session tokens, it would overwrite the long-lived
// credentials that the user configured via `atmos auth user configure`.
func TestBuildWhoamiInfo_SkipsSessionTokenCaching(t *testing.T) {
	// Step 1: Set up keyring with long-lived credentials.
	longLivedCreds := &types.AWSCredentials{
		AccessKeyID:     "AKIA_LONG_LIVED_KEY",
		SecretAccessKey: "long_lived_secret_key",
		MfaArn:          "arn:aws:iam::123456789012:mfa/test-user",
		SessionDuration: "12h",
		Region:          "us-east-1",
		// NO SessionToken - this is a long-lived credential
	}

	store := &testStore{
		data: map[string]any{
			"test-identity": longLivedCreds, // Long-lived credentials should NOT be overwritten
		},
		expired: map[string]bool{},
	}

	// Step 2: Create a manager with the test store.
	mockIdentity := &mockIdentityReturningSessionTokens{
		sessionCreds: &types.AWSCredentials{
			AccessKeyID:     "ASIA_SESSION_KEY",
			SecretAccessKey: "session_secret_key",
			SessionToken:    "session_token_12345",
			Region:          "us-east-1",
			Expiration:      "2099-12-31T23:59:59Z",
		},
	}

	m := &manager{
		credentialStore: store,
		identities: map[string]types.Identity{
			"test-identity": mockIdentity,
		},
	}

	// Step 3: Call buildWhoamiInfo with session tokens.
	// This simulates what happens after authentication completes.
	sessionCreds := &types.AWSCredentials{
		AccessKeyID:     "ASIA_SESSION_KEY",
		SecretAccessKey: "session_secret_key",
		SessionToken:    "session_token_12345", // Session token present
		Region:          "us-east-1",
		Expiration:      "2099-12-31T23:59:59Z",
	}

	info := m.buildWhoamiInfo("test-identity", sessionCreds)

	// Verify buildWhoamiInfo returns correct info.
	assert.Equal(t, "test-identity", info.Identity)
	assert.Equal(t, "test-identity", info.CredentialsRef, "CredentialsRef should be set even for session tokens")
	assert.NotNil(t, info.Credentials, "Credentials should be set for validation")

	// Step 4: Verify keyring STILL contains long-lived credentials.
	// Session tokens should NOT have been cached.
	retrievedCreds, err := store.Retrieve("test-identity")
	require.NoError(t, err, "Should retrieve credentials from keyring")

	retrievedAWSCreds, ok := retrievedCreds.(*types.AWSCredentials)
	require.True(t, ok, "Retrieved credentials should be AWS credentials")

	// CRITICAL: Keyring should NOT contain session tokens.
	assert.Empty(t, retrievedAWSCreds.SessionToken,
		"buildWhoamiInfo should NOT cache session tokens in keyring")

	// CRITICAL: Keyring should STILL contain long-lived credentials.
	assert.Equal(t, "AKIA_LONG_LIVED_KEY", retrievedAWSCreds.AccessKeyID,
		"Long-lived access key should be preserved in keyring")

	assert.Equal(t, "long_lived_secret_key", retrievedAWSCreds.SecretAccessKey,
		"Long-lived secret key should be preserved in keyring")
}

// TestBuildWhoamiInfo_CachesNonSessionCredentials verifies that buildWhoamiInfo
// DOES cache non-session credentials (like long-lived credentials or OIDC tokens).
func TestBuildWhoamiInfo_CachesNonSessionCredentials(t *testing.T) {
	// Start with empty keyring.
	store := &testStore{
		data:    map[string]any{},
		expired: map[string]bool{},
	}

	mockIdentity := &mockIdentityReturningSessionTokens{}

	m := &manager{
		credentialStore: store,
		identities: map[string]types.Identity{
			"test-identity": mockIdentity,
		},
	}

	// Call buildWhoamiInfo with long-lived credentials (no SessionToken).
	longLivedCreds := &types.AWSCredentials{
		AccessKeyID:     "AKIA_LONG_LIVED_KEY",
		SecretAccessKey: "long_lived_secret_key",
		Region:          "us-east-1",
		// NO SessionToken - this should be cached
	}

	info := m.buildWhoamiInfo("test-identity", longLivedCreds)

	// Verify buildWhoamiInfo returns correct info.
	assert.Equal(t, "test-identity", info.Identity)
	assert.Equal(t, "test-identity", info.CredentialsRef)

	// Verify credentials WERE cached (since they're not session tokens).
	retrievedCreds, err := store.Retrieve("test-identity")
	require.NoError(t, err, "Credentials should be cached in keyring")

	retrievedAWSCreds, ok := retrievedCreds.(*types.AWSCredentials)
	require.True(t, ok, "Retrieved credentials should be AWS credentials")

	assert.Equal(t, "AKIA_LONG_LIVED_KEY", retrievedAWSCreds.AccessKeyID,
		"Long-lived credentials should be cached in keyring")
}
