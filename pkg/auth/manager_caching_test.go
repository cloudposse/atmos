package auth

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestGetCachedCredentials_KeyringMiss_IdentityStorageFallback tests the critical
// fallback mechanism when credentials are not in keyring but exist in identity storage.
// This test covers the code path that was previously verified by debug log expectations:
// - "Credentials not in keyring, trying to load from identity storage"
// - "Successfully loaded credentials from identity storage".
func TestGetCachedCredentials_KeyringMiss_IdentityStorageFallback(t *testing.T) {
	tests := []struct {
		name              string
		setupIdentity     func() types.Identity
		expectSuccess     bool
		expectCredentials bool
		expectError       error
	}{
		{
			name: "keyring miss with valid identity storage credentials",
			setupIdentity: func() types.Identity {
				return &mockIdentityWithStorage{
					creds: &mockCreds{
						expired: false,
					},
				}
			},
			expectSuccess:     true,
			expectCredentials: true,
		},
		{
			name: "keyring miss with expired identity storage credentials",
			setupIdentity: func() types.Identity {
				return &mockIdentityWithStorage{
					creds: &mockCreds{
						expired: true,
					},
				}
			},
			expectSuccess: false,
			expectError:   errUtils.ErrExpiredCredentials,
		},
		{
			name: "keyring miss with no identity storage credentials",
			setupIdentity: func() types.Identity {
				return &mockIdentityWithStorage{
					creds: nil, // No credentials in storage
				}
			},
			expectSuccess: false,
			expectError:   errUtils.ErrNoCredentialsFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a credential store that always returns "not found" to simulate keyring miss.
			store := &keyringMissStore{}
			identity := tt.setupIdentity()

			m := &manager{
				config: &schema.AuthConfig{
					Identities: map[string]schema.Identity{
						"test-identity": {Kind: "test"},
					},
				},
				identities: map[string]types.Identity{
					"test-identity": identity,
				},
				credentialStore: store,
			}

			// Call GetCachedCredentials - should fall back to identity storage.
			info, err := m.GetCachedCredentials(context.Background(), "test-identity")

			if tt.expectSuccess {
				require.NoError(t, err, "GetCachedCredentials should succeed via identity storage fallback")
				assert.NotNil(t, info, "WhoamiInfo should not be nil")
				if tt.expectCredentials {
					assert.NotNil(t, info.Credentials, "Credentials should be loaded from identity storage")
				}
			} else {
				require.Error(t, err, "GetCachedCredentials should fail")
				if tt.expectError != nil {
					assert.ErrorIs(t, err, tt.expectError, "Error should match expected error type")
				}
			}
		})
	}
}

// TestGetCachedCredentials_KeyringHit tests the path where credentials are found in keyring.
// This verifies that keyring is checked first before falling back to identity storage.
func TestGetCachedCredentials_KeyringHit(t *testing.T) {
	tests := []struct {
		name          string
		credsExpired  bool
		expectSuccess bool
	}{
		{
			name:          "keyring hit with valid credentials",
			credsExpired:  false,
			expectSuccess: true,
		},
		{
			name:          "keyring hit with expired credentials",
			credsExpired:  true,
			expectSuccess: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &testStore{
				data: map[string]any{
					"test-identity": &mockCreds{expired: tt.credsExpired},
				},
				expired: map[string]bool{
					"test-identity": tt.credsExpired,
				},
			}

			m := &manager{
				config: &schema.AuthConfig{
					Identities: map[string]schema.Identity{
						"test-identity": {Kind: "test"},
					},
				},
				identities: map[string]types.Identity{
					"test-identity": stubIdentity{provider: "test-provider"},
				},
				credentialStore: store,
			}

			info, err := m.GetCachedCredentials(context.Background(), "test-identity")

			if tt.expectSuccess {
				require.NoError(t, err, "GetCachedCredentials should succeed from keyring")
				assert.NotNil(t, info, "WhoamiInfo should not be nil")
				assert.NotNil(t, info.Credentials, "Credentials should be from keyring")
			} else {
				require.Error(t, err, "GetCachedCredentials should fail with expired credentials")
				assert.ErrorIs(t, err, errUtils.ErrExpiredCredentials)
			}
		})
	}
}

// TestGetCachedCredentials_IdentityNotFound tests error handling for non-existent identity.
func TestGetCachedCredentials_IdentityNotFound(t *testing.T) {
	m := &manager{
		config: &schema.AuthConfig{
			Identities: map[string]schema.Identity{},
		},
		identities:      map[string]types.Identity{},
		credentialStore: &testStore{data: map[string]any{}, expired: map[string]bool{}},
	}

	_, err := m.GetCachedCredentials(context.Background(), "non-existent")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrIdentityNotFound)
}

// keyringMissStore is a mock credential store that always returns ErrCredentialsNotFound.
// This simulates a keyring miss, forcing the manager to fall back to identity storage.
type keyringMissStore struct{}

func (s *keyringMissStore) Type() string {
	return "keyring-miss-mock"
}

func (s *keyringMissStore) Store(alias string, creds types.ICredentials) error {
	return nil
}

func (s *keyringMissStore) Retrieve(alias string) (types.ICredentials, error) {
	return nil, credentials.ErrCredentialsNotFound
}

func (s *keyringMissStore) Delete(alias string) error {
	return nil
}

func (s *keyringMissStore) List() ([]string, error) {
	return []string{}, nil
}

func (s *keyringMissStore) IsExpired(alias string) (bool, error) {
	return false, credentials.ErrCredentialsNotFound
}

func (s *keyringMissStore) GetAny(key string, dest interface{}) error {
	return credentials.ErrCredentialsNotFound
}

func (s *keyringMissStore) SetAny(key string, value interface{}) error {
	return nil
}

// mockIdentityWithStorage is a mock identity that simulates having credentials in identity storage.
type mockIdentityWithStorage struct {
	creds types.ICredentials
}

func (m *mockIdentityWithStorage) Kind() string {
	return "mock"
}

func (m *mockIdentityWithStorage) GetProviderName() (string, error) {
	return "mock-provider", nil
}

func (m *mockIdentityWithStorage) Authenticate(ctx context.Context, previousCredentials types.ICredentials) (types.ICredentials, error) {
	return nil, fmt.Errorf("authenticate should not be called in fallback path")
}

func (m *mockIdentityWithStorage) LoadCredentials(ctx context.Context) (types.ICredentials, error) {
	if m.creds == nil {
		return nil, fmt.Errorf("no credentials in identity storage")
	}
	return m.creds, nil
}

func (m *mockIdentityWithStorage) Environment() (map[string]string, error) {
	return map[string]string{"TEST_VAR": "test-value"}, nil
}

func (m *mockIdentityWithStorage) Paths() ([]types.Path, error) {
	return []types.Path{}, nil
}

func (m *mockIdentityWithStorage) CredentialsExist() (bool, error) {
	return m.creds != nil, nil
}

func (m *mockIdentityWithStorage) Validate() error {
	return nil
}

func (m *mockIdentityWithStorage) PostAuthenticate(ctx context.Context, params *types.PostAuthenticateParams) error {
	return nil
}

func (m *mockIdentityWithStorage) Logout(ctx context.Context) error {
	return nil
}

func (m *mockIdentityWithStorage) PrepareEnvironment(_ context.Context, environ map[string]string) (map[string]string, error) {
	return environ, nil
}

// mockCreds is a simple test credential implementation.
type mockCreds struct {
	expired bool
}

func (c *mockCreds) IsExpired() bool {
	return c.expired
}

func (c *mockCreds) GetExpiration() (*time.Time, error) {
	if c.expired {
		past := time.Now().Add(-1 * time.Hour)
		return &past, nil
	}
	future := time.Now().Add(1 * time.Hour)
	return &future, nil
}

func (c *mockCreds) BuildWhoamiInfo(info *types.WhoamiInfo) {
	info.Region = "us-east-1"
}

func (c *mockCreds) Validate(ctx context.Context) (*types.ValidationInfo, error) {
	exp, _ := c.GetExpiration()
	return &types.ValidationInfo{
		Expiration: exp,
	}, nil
}
