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

func TestNewProvider(t *testing.T) {
	t.Run("creates provider with valid config", func(t *testing.T) {
		config := &schema.Provider{
			Kind: "mock",
		}

		provider := NewProvider("test-provider", config)

		require.NotNil(t, provider)
		assert.Equal(t, "test-provider", provider.Name())
		assert.Equal(t, "mock", provider.Kind())
	})

	t.Run("returns nil with nil config", func(t *testing.T) {
		provider := NewProvider("test-provider", nil)

		assert.Nil(t, provider)
	})
}

func TestProvider_Kind(t *testing.T) {
	config := &schema.Provider{
		Kind: "mock",
	}

	provider := NewProvider("test-provider", config)

	assert.Equal(t, "mock", provider.Kind())
}

func TestProvider_Name(t *testing.T) {
	config := &schema.Provider{
		Kind: "mock",
	}

	provider := NewProvider("test-provider", config)

	assert.Equal(t, "test-provider", provider.Name())
}

func TestProvider_PreAuthenticate(t *testing.T) {
	config := &schema.Provider{
		Kind: "mock",
	}

	provider := NewProvider("test-provider", config)

	// PreAuthenticate is a no-op for mock provider.
	err := provider.PreAuthenticate(nil)

	assert.NoError(t, err)
}

func TestProvider_Authenticate(t *testing.T) {
	t.Run("returns mock credentials with fixed expiration", func(t *testing.T) {
		config := &schema.Provider{
			Kind: "mock",
		}

		provider := NewProvider("test-provider", config)
		ctx := context.Background()

		creds, err := provider.Authenticate(ctx)

		require.NoError(t, err)
		require.NotNil(t, creds)

		// Verify credentials are not expired.
		assert.False(t, creds.IsExpired(), "Mock credentials should not be expired")

		// Verify expiration is in the far future (2099).
		expiration, err := creds.GetExpiration()
		require.NoError(t, err)
		require.NotNil(t, expiration)

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

		assert.Equal(t, expectedExpiration, *expiration,
			"Expiration should match deterministic mock timestamp")

		// Verify expiration is in the future.
		assert.True(t, expiration.After(time.Now()),
			"Mock credentials should expire in the future")
	})

	t.Run("credentials have expected structure", func(t *testing.T) {
		config := &schema.Provider{
			Kind: "mock",
		}

		provider := NewProvider("test-provider", config)
		ctx := context.Background()

		creds, err := provider.Authenticate(ctx)

		require.NoError(t, err)

		// Cast to Credentials to verify structure.
		mockCreds, ok := creds.(*Credentials)
		require.True(t, ok, "Credentials should be *mock.Credentials")

		assert.Equal(t, "MOCK_ACCESS_KEY_ID", mockCreds.AccessKeyID)
		assert.Equal(t, "MOCK_SECRET_ACCESS_KEY", mockCreds.SecretAccessKey)
		assert.Equal(t, "MOCK_SESSION_TOKEN", mockCreds.SessionToken)
		assert.Equal(t, "us-east-1", mockCreds.Region)
	})
}

func TestProvider_Validate(t *testing.T) {
	config := &schema.Provider{
		Kind: "mock",
	}

	provider := NewProvider("test-provider", config)

	// Validate always succeeds for mock provider.
	err := provider.Validate()

	assert.NoError(t, err)
}

func TestProvider_Environment(t *testing.T) {
	config := &schema.Provider{
		Kind: "mock",
	}

	provider := NewProvider("test-provider", config)

	env, err := provider.Environment()

	require.NoError(t, err)
	require.NotNil(t, env)

	// Verify mock environment variable is set.
	assert.Equal(t, "test-provider", env["MOCK_PROVIDER"])
}

func TestProvider_Logout(t *testing.T) {
	config := &schema.Provider{
		Kind: "mock",
	}

	provider := NewProvider("test-provider", config)
	ctx := context.Background()

	// Logout is a no-op for mock provider.
	err := provider.Logout(ctx)

	assert.NoError(t, err)
}

func TestProvider_GetFilesDisplayPath(t *testing.T) {
	config := &schema.Provider{
		Kind: "mock",
	}

	provider := NewProvider("test-provider", config)

	path := provider.GetFilesDisplayPath()

	assert.Equal(t, "~/.mock/credentials", path)
}

// TestProvider_ImplementsInterface verifies that Provider implements types.Provider.
func TestProvider_ImplementsInterface(t *testing.T) {
	config := &schema.Provider{
		Kind: "mock",
	}

	provider := NewProvider("test-provider", config)

	// This will fail to compile if Provider doesn't implement types.Provider.
	var _ types.Provider = provider
}

// TestProvider_Concurrency verifies that multiple concurrent authentications work correctly.
func TestProvider_Concurrency(t *testing.T) {
	config := &schema.Provider{
		Kind: "mock",
	}

	provider := NewProvider("test-provider", config)
	ctx := context.Background()

	// Run 10 concurrent authentications.
	const numGoroutines = 10
	results := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			creds, err := provider.Authenticate(ctx)
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
