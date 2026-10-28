package authbridge

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
)

// errTestAuthFailed is the sentinel returned by mocked managers to simulate authentication failure.
var errTestAuthFailed = errors.New("authentication failed")

func TestNewResolver(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockManager := types.NewMockAuthManager(ctrl)
	stackInfo := &schema.ConfigAndStacksInfo{
		AuthContext: &schema.AuthContext{},
	}

	resolver := NewResolver(mockManager, stackInfo)
	assert.NotNil(t, resolver)
	assert.Equal(t, mockManager, resolver.authManager)
	assert.Equal(t, stackInfo, resolver.stackInfo)
}

func TestResolveAWSAuthContext_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockManager := types.NewMockAuthManager(ctrl)

	expectedCredsFile := filepath.Join("tmp", "aws-creds")
	expectedConfigFile := filepath.Join("tmp", "aws-config")

	mockManager.EXPECT().
		AuthenticateIsolated(gomock.Any(), "prod-admin").
		Return(&schema.AuthContext{
			AWS: &schema.AWSAuthContext{
				CredentialsFile: expectedCredsFile,
				ConfigFile:      expectedConfigFile,
				Profile:         "prod",
				Region:          "us-east-1",
				EndpointURL:     "http://localhost:4566",
			},
		}, nil)

	resolver := NewResolver(mockManager, &schema.ConfigAndStacksInfo{})

	authConfig, err := resolver.ResolveAWSAuthContext(context.Background(), "prod-admin")
	require.NoError(t, err)
	require.NotNil(t, authConfig)
	assert.Equal(t, expectedCredsFile, authConfig.CredentialsFile)
	assert.Equal(t, expectedConfigFile, authConfig.ConfigFile)
	assert.Equal(t, "prod", authConfig.Profile)
	assert.Equal(t, "us-east-1", authConfig.Region)
	assert.Equal(t, "http://localhost:4566", authConfig.EndpointURL)
}

func TestResolveAWSAuthContext_RealmScopedPaths(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockManager := types.NewMockAuthManager(ctrl)

	// Simulate realm-scoped credential paths (as populated by auth system with realm).
	realmCredsFile := filepath.Join(".config", "atmos", "my-realm", "aws", "aws-sso", "credentials")
	realmConfigFile := filepath.Join(".config", "atmos", "my-realm", "aws", "aws-sso", "config")

	mockManager.EXPECT().
		AuthenticateIsolated(gomock.Any(), "prod-admin").
		Return(&schema.AuthContext{
			AWS: &schema.AWSAuthContext{
				CredentialsFile: realmCredsFile,
				ConfigFile:      realmConfigFile,
				Profile:         "prod-admin",
				Region:          "eu-west-1",
			},
		}, nil)

	resolver := NewResolver(mockManager, &schema.ConfigAndStacksInfo{})

	authConfig, err := resolver.ResolveAWSAuthContext(context.Background(), "prod-admin")
	require.NoError(t, err)
	require.NotNil(t, authConfig)
	// Verify realm-scoped paths flow through unchanged.
	assert.Equal(t, realmCredsFile, authConfig.CredentialsFile)
	assert.Equal(t, realmConfigFile, authConfig.ConfigFile)
	assert.Equal(t, "prod-admin", authConfig.Profile)
	assert.Equal(t, "eu-west-1", authConfig.Region)
}

func TestResolveAWSAuthContext_AuthFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockManager := types.NewMockAuthManager(ctrl)
	mockManager.EXPECT().
		AuthenticateIsolated(gomock.Any(), "bad-identity").
		Return(nil, errTestAuthFailed)

	resolver := NewResolver(mockManager, &schema.ConfigAndStacksInfo{})

	authConfig, err := resolver.ResolveAWSAuthContext(context.Background(), "bad-identity")
	require.Error(t, err)
	assert.Nil(t, authConfig)
	assert.ErrorIs(t, err, errTestAuthFailed)
	assert.Contains(t, err.Error(), `failed to authenticate identity "bad-identity" for store`)
}

func TestResolveAWSAuthContext_NoAWSContext(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockManager := types.NewMockAuthManager(ctrl)
	// AWS is nil: e.g., an Azure identity was authenticated.
	mockManager.EXPECT().
		AuthenticateIsolated(gomock.Any(), "azure-identity").
		Return(&schema.AuthContext{}, nil)

	resolver := NewResolver(mockManager, &schema.ConfigAndStacksInfo{})

	authConfig, err := resolver.ResolveAWSAuthContext(context.Background(), "azure-identity")
	require.Error(t, err)
	assert.Nil(t, authConfig)
	assert.ErrorIs(t, err, store.ErrAuthContextNotAvailable)
	assert.Contains(t, err.Error(), "AWS auth context not available")
}

func TestResolveAzureAuthContext_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockManager := types.NewMockAuthManager(ctrl)

	// Simulate realm-scoped credential path (as populated by auth system with realm).
	realmCredsFile := filepath.Join(".azure", "atmos", "my-realm", "azure-oidc", "credentials.json")

	mockManager.EXPECT().
		AuthenticateIsolated(gomock.Any(), "azure-prod").
		Return(&schema.AuthContext{
			Azure: &schema.AzureAuthContext{
				CredentialsFile: realmCredsFile,
				SubscriptionID:  "sub-789",
				TenantID:        "tenant-123",
				UseOIDC:         true,
				ClientID:        "client-456",
				TokenFilePath:   filepath.Join("tmp", "oidc-token"),
			},
		}, nil)

	resolver := NewResolver(mockManager, &schema.ConfigAndStacksInfo{})

	authConfig, err := resolver.ResolveAzureAuthContext(context.Background(), "azure-prod")
	require.NoError(t, err)
	require.NotNil(t, authConfig)
	// Verify all fields including realm-scoped paths flow through.
	assert.Equal(t, realmCredsFile, authConfig.CredentialsFile)
	assert.Equal(t, "sub-789", authConfig.SubscriptionID)
	assert.Equal(t, "tenant-123", authConfig.TenantID)
	assert.True(t, authConfig.UseOIDC)
	assert.Equal(t, "client-456", authConfig.ClientID)
	assert.Equal(t, filepath.Join("tmp", "oidc-token"), authConfig.TokenFilePath)
}

func TestResolveAzureAuthContext_AuthFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockManager := types.NewMockAuthManager(ctrl)
	mockManager.EXPECT().
		AuthenticateIsolated(gomock.Any(), "bad-identity").
		Return(nil, errTestAuthFailed)

	resolver := NewResolver(mockManager, &schema.ConfigAndStacksInfo{})

	authConfig, err := resolver.ResolveAzureAuthContext(context.Background(), "bad-identity")
	require.Error(t, err)
	assert.Nil(t, authConfig)
	assert.ErrorIs(t, err, errTestAuthFailed)
	assert.Contains(t, err.Error(), `failed to authenticate identity "bad-identity" for store`)
}

func TestResolveAzureAuthContext_NoAzureContext(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockManager := types.NewMockAuthManager(ctrl)
	mockManager.EXPECT().
		AuthenticateIsolated(gomock.Any(), "aws-identity").
		Return(&schema.AuthContext{}, nil)

	resolver := NewResolver(mockManager, &schema.ConfigAndStacksInfo{})

	authConfig, err := resolver.ResolveAzureAuthContext(context.Background(), "aws-identity")
	require.Error(t, err)
	assert.Nil(t, authConfig)
	assert.ErrorIs(t, err, store.ErrAuthContextNotAvailable)
	assert.Contains(t, err.Error(), "Azure auth context not available")
}

func TestResolveGCPAuthContext_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockManager := types.NewMockAuthManager(ctrl)

	// Simulate realm-scoped credential path (as populated by auth system with realm).
	realmCredsFile := filepath.Join(".config", "atmos", "my-realm", "gcp", "gcp-adc", "adc", "gcp-prod", "application_default_credentials.json")
	tokenExpiry := time.Now().Add(time.Hour).UTC()

	mockManager.EXPECT().
		AuthenticateIsolated(gomock.Any(), "gcp-prod").
		Return(&schema.AuthContext{
			GCP: &schema.GCPAuthContext{
				CredentialsFile: realmCredsFile,
				ProjectID:       "my-gcp-project",
				AccessToken:     "ya29.identity-token",
				TokenExpiry:     tokenExpiry,
			},
		}, nil)

	resolver := NewResolver(mockManager, &schema.ConfigAndStacksInfo{})

	authConfig, err := resolver.ResolveGCPAuthContext(context.Background(), "gcp-prod")
	require.NoError(t, err)
	require.NotNil(t, authConfig)
	// Verify realm-scoped path and project ID flow through.
	assert.Equal(t, realmCredsFile, authConfig.CredentialsFile)
	assert.Equal(t, "my-gcp-project", authConfig.ProjectID)
	assert.Equal(t, "ya29.identity-token", authConfig.AccessToken)
	assert.Equal(t, tokenExpiry, authConfig.TokenExpiry)
}

func TestResolveGCPAuthContext_AuthFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockManager := types.NewMockAuthManager(ctrl)
	mockManager.EXPECT().
		AuthenticateIsolated(gomock.Any(), "bad-identity").
		Return(nil, errTestAuthFailed)

	resolver := NewResolver(mockManager, &schema.ConfigAndStacksInfo{})

	authConfig, err := resolver.ResolveGCPAuthContext(context.Background(), "bad-identity")
	require.Error(t, err)
	assert.Nil(t, authConfig)
	assert.ErrorIs(t, err, errTestAuthFailed)
	assert.Contains(t, err.Error(), `failed to authenticate identity "bad-identity" for store`)
}

func TestResolveGCPAuthContext_NoGCPContext(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockManager := types.NewMockAuthManager(ctrl)
	mockManager.EXPECT().
		AuthenticateIsolated(gomock.Any(), "aws-identity").
		Return(&schema.AuthContext{}, nil)

	resolver := NewResolver(mockManager, &schema.ConfigAndStacksInfo{})

	authConfig, err := resolver.ResolveGCPAuthContext(context.Background(), "aws-identity")
	require.Error(t, err)
	assert.Nil(t, authConfig)
	assert.ErrorIs(t, err, store.ErrAuthContextNotAvailable)
	assert.Contains(t, err.Error(), "GCP auth context not available")
}

// TestResolver_NeverTouchesSharedManagerState verifies the resolver only calls AuthenticateIsolated.
// The mock fails the test on any unexpected Authenticate or GetStackInfo call, because authenticating
// on the shared manager overwrites its chain and auth context (issue #3544).
func TestResolver_NeverTouchesSharedManagerState(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockManager := types.NewMockAuthManager(ctrl)
	mockManager.EXPECT().
		AuthenticateIsolated(gomock.Any(), "store-identity").
		Return(&schema.AuthContext{
			AWS:         &schema.AWSAuthContext{Profile: "aws"},
			Azure:       &schema.AzureAuthContext{TenantID: "tenant"},
			GCP:         &schema.GCPAuthContext{ProjectID: "project"},
			Environment: map[string]string{"VAULT_TOKEN": "identity-token"},
		}, nil).
		Times(4)
	// Deliberately no expectations for Authenticate or GetStackInfo.

	resolver := NewResolver(mockManager, &schema.ConfigAndStacksInfo{})

	awsConfig, err := resolver.ResolveAWSAuthContext(context.Background(), "store-identity")
	require.NoError(t, err)
	assert.Equal(t, "aws", awsConfig.Profile)

	azureConfig, err := resolver.ResolveAzureAuthContext(context.Background(), "store-identity")
	require.NoError(t, err)
	assert.Equal(t, "tenant", azureConfig.TenantID)

	gcpConfig, err := resolver.ResolveGCPAuthContext(context.Background(), "store-identity")
	require.NoError(t, err)
	assert.Equal(t, "project", gcpConfig.ProjectID)

	environment, err := resolver.ResolveEnvironmentAuthContext(context.Background(), "store-identity")
	require.NoError(t, err)
	assert.Equal(t, "identity-token", environment.Environment["VAULT_TOKEN"])
}

// TestResolver_NilReturnedContext verifies a nil context from isolated authentication maps to
// store.ErrAuthContextNotAvailable for every cloud.
func TestResolver_NilReturnedContext(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockManager := types.NewMockAuthManager(ctrl)
	mockManager.EXPECT().
		AuthenticateIsolated(gomock.Any(), "test-identity").
		Return(nil, nil).
		Times(4)

	resolver := NewResolver(mockManager, nil)

	_, err := resolver.ResolveAWSAuthContext(context.Background(), "test-identity")
	require.ErrorIs(t, err, store.ErrAuthContextNotAvailable)
	assert.Contains(t, err.Error(), "AWS auth context not available")

	_, err = resolver.ResolveAzureAuthContext(context.Background(), "test-identity")
	require.ErrorIs(t, err, store.ErrAuthContextNotAvailable)
	assert.Contains(t, err.Error(), "Azure auth context not available")

	_, err = resolver.ResolveGCPAuthContext(context.Background(), "test-identity")
	require.ErrorIs(t, err, store.ErrAuthContextNotAvailable)
	assert.Contains(t, err.Error(), "GCP auth context not available")

	_, err = resolver.ResolveEnvironmentAuthContext(context.Background(), "test-identity")
	require.ErrorIs(t, err, store.ErrAuthContextNotAvailable)
	assert.Contains(t, err.Error(), "environment auth context not available")
}
