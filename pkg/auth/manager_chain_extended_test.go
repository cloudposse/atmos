package auth

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/auth/validation"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestManager_IsSessionToken(t *testing.T) {
	tests := []struct {
		name     string
		creds    types.ICredentials
		expected bool
	}{
		{
			name: "AWS credentials with session token",
			creds: &types.AWSCredentials{
				AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
				SecretAccessKey: "secret",
				SessionToken:    "session-token",
			},
			expected: true,
		},
		{
			name: "AWS credentials without session token",
			creds: &types.AWSCredentials{
				AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
				SecretAccessKey: "secret",
				SessionToken:    "",
			},
			expected: false,
		},
		{
			name:     "nil credentials",
			creds:    nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isSessionToken(tt.creds)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestManager_DetermineStartingIndex(t *testing.T) {
	credStore := credentials.NewCredentialStore()
	validator := validation.NewValidator()

	m, err := NewAuthManager(&schema.AuthConfig{
		Providers:  map[string]schema.Provider{},
		Identities: map[string]schema.Identity{},
	}, credStore, validator, nil)
	require.NoError(t, err)

	mgr := m.(*manager)

	tests := []struct {
		name       string
		startIndex int
		expected   int
	}{
		{
			name:       "no cached credentials returns 0",
			startIndex: -1,
			expected:   0,
		},
		{
			name:       "cached at index 0 returns 0",
			startIndex: 0,
			expected:   0,
		},
		{
			name:       "cached at index 2 returns 2",
			startIndex: 2,
			expected:   2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mgr.determineStartingIndex(tt.startIndex)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestManager_GetChainStepName(t *testing.T) {
	credStore := credentials.NewCredentialStore()
	validator := validation.NewValidator()

	m, err := NewAuthManager(&schema.AuthConfig{
		Providers:  map[string]schema.Provider{},
		Identities: map[string]schema.Identity{},
	}, credStore, validator, nil)
	require.NoError(t, err)

	mgr := m.(*manager)
	mgr.chain = []string{"provider1", "identity1", "identity2"}

	tests := []struct {
		name     string
		index    int
		expected string
	}{
		{
			name:     "valid index 0",
			index:    0,
			expected: "provider1",
		},
		{
			name:     "valid index 1",
			index:    1,
			expected: "identity1",
		},
		{
			name:     "valid index 2",
			index:    2,
			expected: "identity2",
		},
		{
			name:     "index out of bounds",
			index:    10,
			expected: "unknown",
		},
		// Note: negative indices cause a panic in the current implementation.
		// This is a potential bug that should be addressed separately.
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mgr.getChainStepName(tt.index)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestManager_IsCredentialValid(t *testing.T) {
	credStore := credentials.NewCredentialStore()
	validator := validation.NewValidator()

	m, err := NewAuthManager(&schema.AuthConfig{
		Providers:  map[string]schema.Provider{},
		Identities: map[string]schema.Identity{},
	}, credStore, validator, nil)
	require.NoError(t, err)

	mgr := m.(*manager)

	tests := []struct {
		name          string
		expiration    *time.Time
		expectValid   bool
		expectExpTime bool
	}{
		{
			name:          "no expiration - valid",
			expiration:    nil,
			expectValid:   true,
			expectExpTime: false,
		},
		{
			name: "future expiration - valid",
			expiration: func() *time.Time {
				t := time.Now().Add(time.Hour)
				return &t
			}(),
			expectValid:   true,
			expectExpTime: true,
		},
		{
			name: "past expiration - invalid",
			expiration: func() *time.Time {
				t := time.Now().Add(-time.Hour)
				return &t
			}(),
			expectValid:   false,
			expectExpTime: true,
		},
		{
			name: "expiring soon (within buffer) - invalid",
			expiration: func() *time.Time {
				t := time.Now().Add(time.Second * 30) // Within 5 min buffer.
				return &t
			}(),
			expectValid:   false,
			expectExpTime: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock credentials with the given expiration.
			creds := &mockCredentialsWithExpiration{expiration: tt.expiration}

			valid, expTime := mgr.isCredentialValid("test-identity", creds)
			assert.Equal(t, tt.expectValid, valid)

			if tt.expectExpTime {
				assert.NotNil(t, expTime)
			} else {
				assert.Nil(t, expTime)
			}
		})
	}
}

func TestManager_FetchCachedCredentials_NilCredentialStore(t *testing.T) {
	m := &manager{
		config:          &schema.AuthConfig{},
		credentialStore: nil, // No credential store.
		chain:           []string{"provider", "identity"},
	}

	creds, index := m.fetchCachedCredentials(0)
	assert.Nil(t, creds)
	assert.Equal(t, 0, index) // Should return 0 when no credential store.
}

func TestManager_BuildAuthenticationChain_AWSUserStandalone(t *testing.T) {
	credStore := credentials.NewCredentialStore()
	validator := validation.NewValidator()

	m, err := NewAuthManager(&schema.AuthConfig{
		Providers: map[string]schema.Provider{},
		Identities: map[string]schema.Identity{
			"aws-user": {
				Kind: "aws/user",
				// No Via - standalone.
			},
		},
	}, credStore, validator, nil)
	require.NoError(t, err)

	mgr := m.(*manager)
	chain, err := mgr.buildAuthenticationChain("aws-user")
	require.NoError(t, err)
	assert.Equal(t, []string{"aws-user"}, chain)
}

func TestManager_BuildAuthenticationChain_WithProvider(t *testing.T) {
	credStore := credentials.NewCredentialStore()
	validator := validation.NewValidator()

	m, err := NewAuthManager(&schema.AuthConfig{
		Providers: map[string]schema.Provider{
			"my-provider": {
				Kind: "mock",
			},
		},
		Identities: map[string]schema.Identity{
			"my-identity": {
				Kind: "aws/permission-set",
				Via: &schema.IdentityVia{
					Provider: "my-provider",
				},
			},
		},
	}, credStore, validator, nil)
	require.NoError(t, err)

	mgr := m.(*manager)
	chain, err := mgr.buildAuthenticationChain("my-identity")
	require.NoError(t, err)
	// Chain should be [provider, identity] in authentication order.
	assert.Equal(t, []string{"my-provider", "my-identity"}, chain)
}

func TestManager_BuildAuthenticationChain_MultipleIdentities(t *testing.T) {
	credStore := credentials.NewCredentialStore()
	validator := validation.NewValidator()

	m, err := NewAuthManager(&schema.AuthConfig{
		Providers: map[string]schema.Provider{
			"my-provider": {
				Kind: "mock",
			},
		},
		Identities: map[string]schema.Identity{
			"base-identity": {
				Kind: "aws/permission-set",
				Via: &schema.IdentityVia{
					Provider: "my-provider",
				},
			},
			"derived-identity": {
				Kind: "aws/assume-role",
				Via: &schema.IdentityVia{
					Identity: "base-identity",
				},
			},
		},
	}, credStore, validator, nil)
	require.NoError(t, err)

	mgr := m.(*manager)
	chain, err := mgr.buildAuthenticationChain("derived-identity")
	require.NoError(t, err)
	// Chain should be [provider, base, derived] in authentication order.
	assert.Equal(t, []string{"my-provider", "base-identity", "derived-identity"}, chain)
}

// mockCredentialsWithExpiration is a mock for testing expiration validation.
type mockCredentialsWithExpiration struct {
	expiration *time.Time
}

func (m *mockCredentialsWithExpiration) IsExpired() bool {
	if m.expiration == nil {
		return false
	}
	return m.expiration.Before(time.Now())
}

func (m *mockCredentialsWithExpiration) GetExpiration() (*time.Time, error) {
	return m.expiration, nil
}

func (m *mockCredentialsWithExpiration) BuildWhoamiInfo(_ *types.WhoamiInfo) {
	// No-op for mock.
}

func (m *mockCredentialsWithExpiration) Validate(_ context.Context) (*types.ValidationInfo, error) {
	return nil, nil
}
