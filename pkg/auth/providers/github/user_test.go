package github

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewUserProvider(t *testing.T) {
	tests := []struct {
		name        string
		provName    string
		config      *schema.Provider
		expectError bool
		errorType   error
	}{
		{
			name:     "valid configuration",
			provName: "github-user",
			config: &schema.Provider{
				Kind: KindUser,
				Spec: map[string]interface{}{
					"client_id": "test-client-id",
				},
			},
			expectError: false,
		},
		{
			name:     "valid with scopes",
			provName: "github-user",
			config: &schema.Provider{
				Kind: KindUser,
				Spec: map[string]interface{}{
					"client_id": "test-client-id",
					"scopes":    []interface{}{"repo", "read:org"},
				},
			},
			expectError: false,
		},
		{
			name:        "nil config",
			provName:    "github-user",
			config:      nil,
			expectError: true,
			errorType:   errUtils.ErrInvalidProviderConfig,
		},
		{
			name:        "empty provider name",
			provName:    "",
			config:      &schema.Provider{Kind: KindUser},
			expectError: true,
			errorType:   errUtils.ErrInvalidProviderConfig,
		},
		{
			name:     "uses default client_id when not specified",
			provName: "github-user",
			config: &schema.Provider{
				Kind: KindUser,
				Spec: map[string]interface{}{},
			},
			expectError: false,
		},
		{
			name:     "uses default client_id when spec is nil",
			provName: "github-user",
			config: &schema.Provider{
				Kind: KindUser,
			},
			expectError: false,
		},
		{
			name:     "uses custom client_id when specified",
			provName: "github-user",
			config: &schema.Provider{
				Kind: KindUser,
				Spec: map[string]interface{}{
					"client_id": "custom-client-id",
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := NewUserProvider(tt.provName, tt.config)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorType != nil {
					assert.ErrorIs(t, err, tt.errorType)
				}
				assert.Nil(t, provider)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, provider)
				assert.Equal(t, tt.provName, provider.Name())
				assert.Equal(t, KindUser, provider.Kind())

				// Verify client_id defaults or custom value.
				userProv := provider.(*userProvider)
				if tt.config.Spec != nil {
					if customClientID, ok := tt.config.Spec["client_id"].(string); ok && customClientID != "" {
						assert.Equal(t, customClientID, userProv.clientID)
					} else {
						assert.Equal(t, DefaultClientID, userProv.clientID)
					}
				} else {
					assert.Equal(t, DefaultClientID, userProv.clientID)
				}
			}
		})
	}
}

// TestUserProvider_Authenticate_WithCachedToken removed - provider no longer caches tokens.
// Auth manager handles credential storage via credential store.

func TestUserProvider_Authenticate_DeviceFlow(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMockDeviceFlowClient(ctrl)

	config := &schema.Provider{
		Kind: KindUser,
		Spec: map[string]interface{}{
			"client_id": "test-client-id",
		},
	}

	provider, err := NewUserProvider("test-provider", config)
	require.NoError(t, err)

	userProv := provider.(*userProvider)
	userProv.deviceClient = mockClient

	deviceCode := "test-device-code"
	userCode := "ABCD-1234"
	verificationURI := "https://github.com/login/device"
	interval := 5
	newToken := "ghs_new_token_67890"

	deviceResp := &DeviceFlowResponse{
		DeviceCode:      deviceCode,
		UserCode:        userCode,
		VerificationURI: verificationURI,
		Interval:        interval,
		ExpiresIn:       900,
	}

	// Setup expectations: start device flow and poll for token.
	mockClient.EXPECT().
		StartDeviceFlow(gomock.Any()).
		Return(deviceResp, nil)

	mockClient.EXPECT().
		PollForToken(gomock.Any(), deviceCode, interval).
		Return(newToken, nil)

	// Authenticate should perform device flow.
	creds, err := provider.Authenticate(context.Background())

	require.NoError(t, err)
	require.NotNil(t, creds)

	githubCreds, ok := creds.(*types.GitHubUserCredentials)
	require.True(t, ok)
	assert.Equal(t, newToken, githubCreds.Token)
}

func TestUserProvider_Authenticate_DeviceFlowFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := NewMockDeviceFlowClient(ctrl)

	config := &schema.Provider{
		Kind: KindUser,
		Spec: map[string]interface{}{
			"client_id": "test-client-id",
		},
	}

	provider, err := NewUserProvider("test-provider", config)
	require.NoError(t, err)

	userProv := provider.(*userProvider)
	userProv.deviceClient = mockClient

	// Setup expectations: device flow fails.
	deviceFlowErr := errors.New("device code request failed")
	mockClient.EXPECT().
		StartDeviceFlow(gomock.Any()).
		Return(nil, deviceFlowErr)

	// Authenticate should return error.
	creds, err := provider.Authenticate(context.Background())

	require.Error(t, err)
	assert.Nil(t, creds)
	assert.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
}

// TestUserProvider_Logout and TestUserProvider_Logout_Failure removed.
// Provider no longer manages logout - auth manager handles credential deletion via credential store.

func TestUserProvider_Validate(t *testing.T) {
	config := &schema.Provider{
		Kind: KindUser,
		Spec: map[string]interface{}{
			"client_id": "test-client-id",
		},
	}

	provider, err := NewUserProvider("test-provider", config)
	require.NoError(t, err)

	err = provider.Validate()
	assert.NoError(t, err)
}

func TestUserProvider_Environment(t *testing.T) {
	config := &schema.Provider{
		Kind: KindUser,
		Spec: map[string]interface{}{
			"client_id": "test-client-id",
		},
	}

	provider, err := NewUserProvider("test-provider", config)
	require.NoError(t, err)

	env, err := provider.Environment()
	assert.NoError(t, err)
	assert.Empty(t, env)
}

func TestGitHubUserCredentials_IsExpired(t *testing.T) {
	tests := []struct {
		name       string
		expiration time.Time
		expected   bool
	}{
		{
			name:       "not expired",
			expiration: time.Now().Add(1 * time.Hour),
			expected:   false,
		},
		{
			name:       "expired",
			expiration: time.Now().Add(-1 * time.Hour),
			expected:   true,
		},
		{
			name:       "about to expire (within 5 min)",
			expiration: time.Now().Add(3 * time.Minute),
			expected:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			creds := &types.GitHubUserCredentials{
				Token:      "test-token",
				Expiration: tt.expiration,
			}

			assert.Equal(t, tt.expected, creds.IsExpired())
		})
	}
}

func TestGitHubUserCredentials_BuildWhoamiInfo(t *testing.T) {
	creds := &types.GitHubUserCredentials{
		Token:    "test-token",
		Provider: "github-user",
	}

	info := &types.WhoamiInfo{
		Environment: make(map[string]string),
	}

	creds.BuildWhoamiInfo(info)

	assert.Equal(t, "test-token", info.Environment["GITHUB_TOKEN"])
	assert.Equal(t, "test-token", info.Environment["GH_TOKEN"])
}

func TestUserProvider_PreAuthenticate(t *testing.T) {
	config := &schema.Provider{
		Kind: KindUser,
		Spec: map[string]interface{}{
			"client_id": "test-client-id",
		},
	}

	provider, err := NewUserProvider("test-provider", config)
	require.NoError(t, err)

	err = provider.PreAuthenticate(nil)
	assert.NoError(t, err)
}

func TestUserProvider_Validate_MissingClientID(t *testing.T) {
	// Create provider with default client_id, then manually clear it to test validation.
	config := &schema.Provider{
		Kind: KindUser,
		Spec: map[string]interface{}{},
	}

	provider, err := NewUserProvider("test-provider", config)
	require.NoError(t, err)

	// Verify default client_id was set.
	userProv := provider.(*userProvider)
	assert.Equal(t, DefaultClientID, userProv.clientID)

	// Manually clear client_id to test validation.
	userProv.clientID = ""

	err = provider.Validate()
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidProviderConfig)
}

func TestGitHubUserCredentials_GetExpiration(t *testing.T) {
	expiration := time.Now().Add(1 * time.Hour)
	creds := &types.GitHubUserCredentials{
		Token:      "test-token",
		Expiration: expiration,
	}

	exp, err := creds.GetExpiration()
	require.NoError(t, err)
	assert.Equal(t, expiration, *exp)
}
