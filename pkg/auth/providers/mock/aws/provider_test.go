package aws

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewProvider(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		config   *schema.Provider
		wantNil  bool
	}{
		{
			name:     "nil config returns nil",
			provider: "test",
			config:   nil,
			wantNil:  true,
		},
		{
			name:     "valid config returns provider",
			provider: "test-provider",
			config: &schema.Provider{
				Kind: "aws",
			},
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NewProvider(tt.provider, tt.config)
			if tt.wantNil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Equal(t, tt.provider, result.name)
				assert.Equal(t, tt.config, result.config)
			}
		})
	}
}

func TestProvider_Kind(t *testing.T) {
	provider := NewProvider("test", &schema.Provider{
		Kind: "aws",
	})

	kind := provider.Kind()
	assert.Equal(t, "aws", kind)
}

func TestProvider_Name(t *testing.T) {
	provider := NewProvider("test-name", &schema.Provider{
		Kind: "aws",
	})

	name := provider.Name()
	assert.Equal(t, "test-name", name)
}

func TestProvider_PreAuthenticate(t *testing.T) {
	provider := NewProvider("test", &schema.Provider{
		Kind: "aws",
	})

	err := provider.PreAuthenticate(nil)
	assert.NoError(t, err)
}

func TestProvider_Authenticate(t *testing.T) {
	provider := NewProvider("test", &schema.Provider{
		Kind: "aws",
	})

	ctx := context.Background()
	creds, err := provider.Authenticate(ctx)

	require.NoError(t, err)
	require.NotNil(t, creds)

	// Verify credentials are of the expected type.
	mockCreds, ok := creds.(*Credentials)
	require.True(t, ok, "credentials should be *Credentials type")

	// Verify mock credentials.
	assert.Equal(t, "MOCK_ACCESS_KEY_ID", mockCreds.AccessKeyID)
	assert.Equal(t, "MOCK_SECRET_ACCESS_KEY", mockCreds.SecretAccessKey)
	assert.Equal(t, "MOCK_SESSION_TOKEN", mockCreds.SessionToken)
	assert.Equal(t, "us-east-1", mockCreds.Region)

	// Verify expiration is set to the fixed far-future timestamp.
	assert.Equal(t, MockExpirationYear, mockCreds.Expiration.Year())
	assert.Equal(t, time.Month(MockExpirationMonth), mockCreds.Expiration.Month())
	assert.Equal(t, MockExpirationDay, mockCreds.Expiration.Day())
	assert.False(t, mockCreds.IsExpired(), "mock credentials should not be expired")
}

func TestProvider_Validate(t *testing.T) {
	provider := NewProvider("test", &schema.Provider{
		Kind: "aws",
	})

	err := provider.Validate()
	assert.NoError(t, err)
}

func TestProvider_Environment(t *testing.T) {
	provider := NewProvider("test-provider", &schema.Provider{
		Kind: "aws",
	})

	env, err := provider.Environment()

	require.NoError(t, err)
	require.NotNil(t, env)
	assert.Equal(t, "test-provider", env["MOCK_PROVIDER"])
}

func TestProvider_PrepareEnvironment(t *testing.T) {
	provider := NewProvider("test", &schema.Provider{
		Kind: "aws",
	})

	ctx := context.Background()
	inputEnv := map[string]string{
		"EXISTING_VAR": "value",
	}

	outputEnv, err := provider.PrepareEnvironment(ctx, inputEnv)

	require.NoError(t, err)
	require.NotNil(t, outputEnv)
	// Mock provider should return environment unchanged.
	assert.Equal(t, inputEnv, outputEnv)
	assert.Equal(t, "value", outputEnv["EXISTING_VAR"])
}

func TestProvider_Logout(t *testing.T) {
	provider := NewProvider("test", &schema.Provider{
		Kind: "aws",
	})

	ctx := context.Background()
	err := provider.Logout(ctx)
	assert.NoError(t, err)
}

func TestProvider_GetFilesDisplayPath(t *testing.T) {
	provider := NewProvider("test", &schema.Provider{
		Kind: "aws",
	})

	path := provider.GetFilesDisplayPath()
	assert.Equal(t, "~/.mock/credentials", path)
}

func TestProvider_Paths(t *testing.T) {
	provider := NewProvider("test", &schema.Provider{
		Kind: "aws",
	})

	paths, err := provider.Paths()

	require.NoError(t, err)
	assert.Empty(t, paths, "mock provider should return empty paths list")
}
