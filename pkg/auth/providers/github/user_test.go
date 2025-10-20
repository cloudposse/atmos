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
			name:     "missing client_id",
			provName: "github-user",
			config: &schema.Provider{
				Kind: KindUser,
				Spec: map[string]interface{}{},
			},
			expectError: true,
			errorType:   errUtils.ErrInvalidProviderConfig,
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
			}
		})
	}
}

func TestUserProvider_Authenticate_WithCachedToken(t *testing.T) {
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

	// Inject mock client.
	userProv := provider.(*userProvider)
	userProv.deviceClient = mockClient

	// Setup expectations: cached token exists and is valid.
	cachedToken := "ghs_cached_token_12345"
	mockClient.EXPECT().
		GetCachedToken(gomock.Any()).
		Return(cachedToken, nil)

	// Authenticate should use cached token.
	creds, err := provider.Authenticate(context.Background())

	require.NoError(t, err)
	require.NotNil(t, creds)

	// Verify credentials.
	githubCreds, ok := creds.(*types.GitHubUserCredentials)
	require.True(t, ok)
	assert.Equal(t, cachedToken, githubCreds.Token)
	assert.Equal(t, "test-provider", githubCreds.Provider)
}

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
	newToken := "ghs_new_token_67890"

	// Setup expectations: no cached token, start device flow.
	mockClient.EXPECT().
		GetCachedToken(gomock.Any()).
		Return("", errors.New("token not found"))

	mockClient.EXPECT().
		StartDeviceFlow(gomock.Any()).
		Return(&DeviceFlowResponse{
			DeviceCode:      deviceCode,
			UserCode:        userCode,
			VerificationURI: verificationURI,
			Interval:        5,
			ExpiresIn:       900,
		}, nil)

	mockClient.EXPECT().
		PollForToken(gomock.Any(), deviceCode).
		Return(newToken, nil)

	mockClient.EXPECT().
		StoreToken(gomock.Any(), newToken).
		Return(nil)

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
	mockClient.EXPECT().
		GetCachedToken(gomock.Any()).
		Return("", errors.New("token not found"))

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

func TestUserProvider_Logout(t *testing.T) {
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

	// Setup expectations: delete token.
	mockClient.EXPECT().
		DeleteToken(gomock.Any()).
		Return(nil)

	// Logout should delete token.
	err = userProv.Logout(context.Background())

	require.NoError(t, err)
}

func TestUserProvider_Logout_Failure(t *testing.T) {
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

	deleteErr := errors.New("failed to delete token")
	mockClient.EXPECT().
		DeleteToken(gomock.Any()).
		Return(deleteErr)

	err = userProv.Logout(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete token")
}

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
	// Create provider with client_id, then clear it to test validation.
	config := &schema.Provider{
		Kind: KindUser,
		Spec: map[string]interface{}{
			"client_id": "test-client-id",
		},
	}

	provider, err := NewUserProvider("test-provider", config)
	require.NoError(t, err)

	// Manually clear client_id to test validation.
	userProv := provider.(*userProvider)
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
