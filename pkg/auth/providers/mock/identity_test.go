package mock

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewIdentity(t *testing.T) {
	t.Run("creates identity with valid config", func(t *testing.T) {
		config := &schema.Identity{
			Kind: "mock",
		}

		identity := NewIdentity("test-identity", config)

		require.NotNil(t, identity)
		assert.Equal(t, "mock", identity.Kind())
	})
}

func TestIdentity_Kind(t *testing.T) {
	config := &schema.Identity{
		Kind: "mock",
	}

	identity := NewIdentity("test-identity", config)

	assert.Equal(t, "mock", identity.Kind())
}

func TestIdentity_GetProviderName(t *testing.T) {
	t.Run("returns provider name from via config", func(t *testing.T) {
		config := &schema.Identity{
			Kind: "mock",
			Via: &schema.IdentityVia{
				Provider: "test-provider",
			},
		}

		identity := NewIdentity("test-identity", config)

		providerName, err := identity.GetProviderName()

		require.NoError(t, err)
		assert.Equal(t, "test-provider", providerName)
	})

	t.Run("returns 'mock' when no via provider specified", func(t *testing.T) {
		config := &schema.Identity{
			Kind: "mock",
		}

		identity := NewIdentity("test-identity", config)

		providerName, err := identity.GetProviderName()

		require.NoError(t, err)
		assert.Equal(t, "mock", providerName)
	})

	t.Run("returns 'mock' when via is nil", func(t *testing.T) {
		config := &schema.Identity{
			Kind: "mock",
			Via:  nil,
		}

		identity := NewIdentity("test-identity", config)

		providerName, err := identity.GetProviderName()

		require.NoError(t, err)
		assert.Equal(t, "mock", providerName)
	})
}

func TestIdentity_Authenticate(t *testing.T) {
	t.Run("returns mock credentials with identity-specific values", func(t *testing.T) {
		config := &schema.Identity{
			Kind: "mock",
		}

		identity := NewIdentity("test-identity", config)
		ctx := context.Background()

		// Base credentials (would normally come from provider).
		baseCreds := &Credentials{
			AccessKeyID:     "BASE_KEY",
			SecretAccessKey: "BASE_SECRET",
			SessionToken:    "BASE_TOKEN",
			Region:          "us-west-2",
			Expiration:      time.Now().Add(1 * time.Hour),
		}

		creds, err := identity.Authenticate(ctx, baseCreds)

		require.NoError(t, err)
		require.NotNil(t, creds)

		// Verify credentials are not expired.
		assert.False(t, creds.IsExpired(), "Mock credentials should not be expired")

		// Cast to mock.Credentials to verify structure.
		mockCreds, ok := creds.(*Credentials)
		require.True(t, ok, "Credentials should be *mock.Credentials")

		// Verify identity-specific values.
		assert.Equal(t, "MOCK_KEY_test-identity", mockCreds.AccessKeyID)
		assert.Equal(t, "MOCK_SECRET_test-identity", mockCreds.SecretAccessKey)
		assert.Equal(t, "MOCK_TOKEN_test-identity", mockCreds.SessionToken)
		assert.Equal(t, "us-east-1", mockCreds.Region)

		// Verify expiration is deterministic.
		expectedExpiration := time.Date(
			MockExpirationYear,
			MockExpirationMonth,
			MockExpirationDay,
			MockExpirationHour,
			MockExpirationMinute,
			MockExpirationSecond,
			0,
			time.UTC,
		)
		assert.Equal(t, expectedExpiration, mockCreds.Expiration)
	})

	t.Run("ignores base credentials (mock implementation)", func(t *testing.T) {
		config := &schema.Identity{
			Kind: "mock",
		}

		identity := NewIdentity("my-identity", config)
		ctx := context.Background()

		// Pass nil base credentials.
		creds, err := identity.Authenticate(ctx, nil)

		require.NoError(t, err)
		require.NotNil(t, creds, "Mock identity should return credentials even with nil base")

		mockCreds, ok := creds.(*Credentials)
		require.True(t, ok)

		// Verify credentials are identity-specific.
		assert.Equal(t, "MOCK_KEY_my-identity", mockCreds.AccessKeyID)
	})
}

func TestIdentity_Validate(t *testing.T) {
	config := &schema.Identity{
		Kind: "mock",
	}

	identity := NewIdentity("test-identity", config)

	// Validate always succeeds for mock identity.
	err := identity.Validate()

	assert.NoError(t, err)
}

func TestIdentity_Environment(t *testing.T) {
	t.Run("returns mock environment variables", func(t *testing.T) {
		config := &schema.Identity{
			Kind: "mock",
		}

		identity := NewIdentity("test-identity", config)

		env, err := identity.Environment()

		require.NoError(t, err)
		require.NotNil(t, env)

		// Verify mock identity environment variable.
		assert.Equal(t, "test-identity", env["MOCK_IDENTITY"])

		// Verify AWS-like environment variables for testing.
		assert.Equal(t, "/tmp/mock-credentials", env["AWS_SHARED_CREDENTIALS_FILE"])
		assert.Equal(t, "/tmp/mock-config", env["AWS_CONFIG_FILE"])
		assert.Equal(t, "test-identity", env["AWS_PROFILE"])
	})
}

func TestIdentity_PostAuthenticate(t *testing.T) {
	config := &schema.Identity{
		Kind: "mock",
	}

	identity := NewIdentity("test-identity", config)
	ctx := context.Background()

	mockCreds := &Credentials{
		AccessKeyID:     "MOCK_KEY",
		SecretAccessKey: "MOCK_SECRET",
		SessionToken:    "MOCK_TOKEN",
		Region:          "us-east-1",
		Expiration:      time.Now().Add(1 * time.Hour),
	}

	// PostAuthenticate is a no-op for mock identity.
	params := &types.PostAuthenticateParams{
		AuthContext:  nil,
		ProviderName: "test-provider",
		IdentityName: "test-identity",
		Credentials:  mockCreds,
	}
	err := identity.PostAuthenticate(ctx, params)

	assert.NoError(t, err)
}

func TestIdentity_Logout(t *testing.T) {
	config := &schema.Identity{
		Kind: "mock",
	}

	identity := NewIdentity("test-identity", config)
	ctx := context.Background()

	// First, authenticate to set up stored credentials.
	err := identity.PostAuthenticate(ctx, &types.PostAuthenticateParams{})
	require.NoError(t, err)

	// Verify LoadCredentials succeeds after authentication.
	creds, err := identity.LoadCredentials(ctx)
	require.NoError(t, err)
	require.NotNil(t, creds)

	// Now logout, which should clear the stored credentials state.
	err = identity.Logout(ctx)
	require.NoError(t, err)

	// Verify LoadCredentials now fails after logout.
	creds, err = identity.LoadCredentials(ctx)
	require.Error(t, err)
	require.Nil(t, creds)
	require.ErrorIs(t, err, ErrNoStoredCredentials)
}

// TestIdentity_ImplementsInterface verifies that Identity implements types.Identity.
func TestIdentity_ImplementsInterface(t *testing.T) {
	config := &schema.Identity{
		Kind: "mock",
	}

	identity := NewIdentity("test-identity", config)

	// This will fail to compile if Identity doesn't implement types.Identity.
	var _ types.Identity = identity
}

// TestIdentity_MultipleInstances verifies that multiple identity instances
// return different credentials.
func TestIdentity_MultipleInstances(t *testing.T) {
	config1 := &schema.Identity{Kind: "mock"}
	config2 := &schema.Identity{Kind: "mock"}

	identity1 := NewIdentity("identity-1", config1)
	identity2 := NewIdentity("identity-2", config2)

	ctx := context.Background()

	creds1, err1 := identity1.Authenticate(ctx, nil)
	require.NoError(t, err1)

	creds2, err2 := identity2.Authenticate(ctx, nil)
	require.NoError(t, err2)

	mockCreds1, ok1 := creds1.(*Credentials)
	require.True(t, ok1)

	mockCreds2, ok2 := creds2.(*Credentials)
	require.True(t, ok2)

	// Verify credentials are different based on identity name.
	assert.NotEqual(t, mockCreds1.AccessKeyID, mockCreds2.AccessKeyID)
	assert.Contains(t, mockCreds1.AccessKeyID, "identity-1")
	assert.Contains(t, mockCreds2.AccessKeyID, "identity-2")
}

// TestIdentity_Concurrency verifies that multiple concurrent authentications work correctly.
func TestIdentity_Concurrency(t *testing.T) {
	config := &schema.Identity{
		Kind: "mock",
	}

	identity := NewIdentity("test-identity", config)
	ctx := context.Background()

	// Run 10 concurrent authentications.
	const numGoroutines = 10
	results := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			creds, err := identity.Authenticate(ctx, nil)
			if err != nil {
				results <- err
				return
			}
			if creds == nil {
				results <- assert.AnError
				return
			}
			results <- nil
		}()
	}

	// Collect results.
	for i := 0; i < numGoroutines; i++ {
		err := <-results
		assert.NoError(t, err, "Concurrent authentication %d should succeed", i)
	}
}

func TestIdentity_LoadCredentials(t *testing.T) {
	tests := []struct {
		name             string
		setupAuth        bool  // Whether to call PostAuthenticate before LoadCredentials
		expectedError    error // Expected sentinel error (nil if no error expected)
		expectedCredsNil bool
	}{
		{
			name:             "returns error when no credentials stored (before authentication)",
			setupAuth:        false,
			expectedError:    ErrNoStoredCredentials,
			expectedCredsNil: true,
		},
		{
			name:             "returns credentials after authentication (stored state)",
			setupAuth:        true,
			expectedError:    nil,
			expectedCredsNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &schema.Identity{
				Kind: "mock",
			}

			identity := NewIdentity("test-load-creds", config)
			ctx := context.Background()

			// Cleanup credentials file after test.
			t.Cleanup(func() {
				_ = identity.Logout(ctx)
			})

			// Simulate authentication if requested.
			if tt.setupAuth {
				err := identity.PostAuthenticate(ctx, &types.PostAuthenticateParams{})
				require.NoError(t, err, "PostAuthenticate should succeed")
			}

			// Attempt to load credentials.
			creds, err := identity.LoadCredentials(ctx)

			// Verify expectations.
			if tt.expectedError != nil {
				require.Error(t, err)
				require.ErrorIs(t, err, tt.expectedError)
			} else {
				require.NoError(t, err)
			}

			if tt.expectedCredsNil {
				require.Nil(t, creds)
			} else {
				require.NotNil(t, creds)
				// Verify it's mock credentials with correct values.
				mockCreds, ok := creds.(*Credentials)
				require.True(t, ok, "credentials should be mock Credentials type")
				assert.Equal(t, "mock-access-key", mockCreds.AccessKeyID)
				assert.Equal(t, "mock-secret-key", mockCreds.SecretAccessKey)
				assert.Equal(t, "mock-session-token", mockCreds.SessionToken)
				assert.Equal(t, "us-east-1", mockCreds.Region)
				assert.False(t, mockCreds.Expiration.IsZero())
				assert.True(t, mockCreds.Expiration.After(time.Now()))
			}
		})
	}
}
