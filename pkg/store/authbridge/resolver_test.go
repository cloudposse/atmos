package authbridge

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

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

	// managerStackInfo simulates the auth manager's internal stackInfo, which is a
	// separate allocation from the stackInfo passed to NewResolver (see pkg/auth.createAuthManagerInstance).
	managerStackInfo := &schema.ConfigAndStacksInfo{
		AuthContext: &schema.AuthContext{
			AWS: &schema.AWSAuthContext{
				CredentialsFile: expectedCredsFile,
				ConfigFile:      expectedConfigFile,
				Profile:         "prod",
				Region:          "us-east-1",
			},
		},
	}

	mockManager.EXPECT().
		Authenticate(gomock.Any(), "prod-admin").
		Return(&types.WhoamiInfo{}, nil)
	mockManager.EXPECT().
		GetStackInfo().
		Return(managerStackInfo)

	resolver := NewResolver(mockManager, &schema.ConfigAndStacksInfo{})

	authConfig, err := resolver.ResolveAWSAuthContext(context.Background(), "prod-admin")
	assert.NoError(t, err)
	assert.NotNil(t, authConfig)
	assert.Equal(t, expectedCredsFile, authConfig.CredentialsFile)
	assert.Equal(t, expectedConfigFile, authConfig.ConfigFile)
	assert.Equal(t, "prod", authConfig.Profile)
	assert.Equal(t, "us-east-1", authConfig.Region)
}

func TestResolveAWSAuthContext_RealmScopedPaths(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockManager := types.NewMockAuthManager(ctrl)

	// Simulate realm-scoped credential paths (as populated by auth system with realm).
	realmCredsFile := filepath.Join(".config", "atmos", "my-realm", "aws", "aws-sso", "credentials")
	realmConfigFile := filepath.Join(".config", "atmos", "my-realm", "aws", "aws-sso", "config")

	managerStackInfo := &schema.ConfigAndStacksInfo{
		AuthContext: &schema.AuthContext{
			AWS: &schema.AWSAuthContext{
				CredentialsFile: realmCredsFile,
				ConfigFile:      realmConfigFile,
				Profile:         "prod-admin",
				Region:          "eu-west-1",
			},
		},
	}

	mockManager.EXPECT().
		Authenticate(gomock.Any(), "prod-admin").
		Return(&types.WhoamiInfo{}, nil)
	mockManager.EXPECT().
		GetStackInfo().
		Return(managerStackInfo)

	resolver := NewResolver(mockManager, &schema.ConfigAndStacksInfo{})

	authConfig, err := resolver.ResolveAWSAuthContext(context.Background(), "prod-admin")
	assert.NoError(t, err)
	assert.NotNil(t, authConfig)
	// Verify realm-scoped paths flow through unchanged.
	assert.Equal(t, realmCredsFile, authConfig.CredentialsFile)
	assert.Equal(t, realmConfigFile, authConfig.ConfigFile)
	assert.Equal(t, "prod-admin", authConfig.Profile)
	assert.Equal(t, "eu-west-1", authConfig.Region)
}

// TestResolveAWSAuthContext_PointerMismatch is a regression test for the bug where
// NewResolver was given the terraform execution's stackInfo while the auth manager
// internally used a separate *schema.ConfigAndStacksInfo allocated in
// pkg/auth.createAuthManagerInstance. The resolver previously checked r.stackInfo.AuthContext.AWS
// (always nil) instead of the manager's own stackInfo (populated by PostAuthenticate).
func TestResolveAWSAuthContext_PointerMismatch(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockManager := types.NewMockAuthManager(ctrl)

	// resolverStackInfo simulates the caller's ConfigAndStacksInfo — passed to NewResolver
	// but never written to by the auth manager (different pointer).
	resolverStackInfo := &schema.ConfigAndStacksInfo{}

	// managerStackInfo simulates the auth manager's own internal allocation, which is
	// what PostAuthenticate populates after a successful Authenticate call.
	expectedCredsFile := filepath.Join("tmp", "aws-creds")
	expectedConfigFile := filepath.Join("tmp", "aws-config")

	managerStackInfo := &schema.ConfigAndStacksInfo{
		AuthContext: &schema.AuthContext{
			AWS: &schema.AWSAuthContext{
				CredentialsFile: expectedCredsFile,
				ConfigFile:      expectedConfigFile,
				Profile:         "dev-admin",
				Region:          "us-west-2",
			},
		},
	}

	mockManager.EXPECT().
		Authenticate(gomock.Any(), "dev-admin").
		Return(&types.WhoamiInfo{}, nil)
	// GetStackInfo returns the manager's own stackInfo, NOT resolverStackInfo.
	mockManager.EXPECT().
		GetStackInfo().
		Return(managerStackInfo)

	resolver := NewResolver(mockManager, resolverStackInfo)

	authConfig, err := resolver.ResolveAWSAuthContext(context.Background(), "dev-admin")
	assert.NoError(t, err)
	assert.NotNil(t, authConfig)
	assert.Equal(t, expectedCredsFile, authConfig.CredentialsFile)
	assert.Equal(t, "dev-admin", authConfig.Profile)

	// Confirm the resolver's own stackInfo was never populated (proving the fix reads from the manager).
	assert.Nil(t, resolverStackInfo.AuthContext)
}

func TestResolveAWSAuthContext_AuthFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockManager := types.NewMockAuthManager(ctrl)

	mockManager.EXPECT().
		Authenticate(gomock.Any(), "bad-identity").
		Return(nil, errors.New("authentication failed"))

	resolver := NewResolver(mockManager, &schema.ConfigAndStacksInfo{})

	authConfig, err := resolver.ResolveAWSAuthContext(context.Background(), "bad-identity")
	assert.Error(t, err)
	assert.Nil(t, authConfig)
	assert.Contains(t, err.Error(), "failed to authenticate identity")
}

func TestResolveAWSAuthContext_NoAWSContext(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockManager := types.NewMockAuthManager(ctrl)

	// Manager's stackInfo has no AWS context (e.g., an Azure identity was authenticated).
	managerStackInfo := &schema.ConfigAndStacksInfo{
		AuthContext: &schema.AuthContext{
			// AWS is nil — not populated by auth.
		},
	}

	mockManager.EXPECT().
		Authenticate(gomock.Any(), "azure-identity").
		Return(&types.WhoamiInfo{}, nil)
	mockManager.EXPECT().
		GetStackInfo().
		Return(managerStackInfo)

	resolver := NewResolver(mockManager, &schema.ConfigAndStacksInfo{})

	authConfig, err := resolver.ResolveAWSAuthContext(context.Background(), "azure-identity")
	assert.Error(t, err)
	assert.Nil(t, authConfig)
	assert.Contains(t, err.Error(), "AWS auth context not available")
}

func TestResolveAzureAuthContext_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockManager := types.NewMockAuthManager(ctrl)

	// Simulate realm-scoped credential path (as populated by auth system with realm).
	realmCredsFile := filepath.Join(".azure", "atmos", "my-realm", "azure-oidc", "credentials.json")

	managerStackInfo := &schema.ConfigAndStacksInfo{
		AuthContext: &schema.AuthContext{
			Azure: &schema.AzureAuthContext{
				CredentialsFile: realmCredsFile,
				SubscriptionID:  "sub-789",
				TenantID:        "tenant-123",
				UseOIDC:         true,
				ClientID:        "client-456",
				TokenFilePath:   filepath.Join("tmp", "oidc-token"),
			},
		},
	}

	mockManager.EXPECT().
		Authenticate(gomock.Any(), "azure-prod").
		Return(&types.WhoamiInfo{}, nil)
	mockManager.EXPECT().
		GetStackInfo().
		Return(managerStackInfo)

	resolver := NewResolver(mockManager, &schema.ConfigAndStacksInfo{})

	authConfig, err := resolver.ResolveAzureAuthContext(context.Background(), "azure-prod")
	assert.NoError(t, err)
	assert.NotNil(t, authConfig)
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
		Authenticate(gomock.Any(), "bad-identity").
		Return(nil, errors.New("authentication failed"))

	resolver := NewResolver(mockManager, &schema.ConfigAndStacksInfo{})

	authConfig, err := resolver.ResolveAzureAuthContext(context.Background(), "bad-identity")
	assert.Error(t, err)
	assert.Nil(t, authConfig)
}

func TestResolveAzureAuthContext_NoAzureContext(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockManager := types.NewMockAuthManager(ctrl)

	managerStackInfo := &schema.ConfigAndStacksInfo{
		AuthContext: &schema.AuthContext{},
	}

	mockManager.EXPECT().
		Authenticate(gomock.Any(), "aws-identity").
		Return(&types.WhoamiInfo{}, nil)
	mockManager.EXPECT().
		GetStackInfo().
		Return(managerStackInfo)

	resolver := NewResolver(mockManager, &schema.ConfigAndStacksInfo{})

	authConfig, err := resolver.ResolveAzureAuthContext(context.Background(), "aws-identity")
	assert.Error(t, err)
	assert.Nil(t, authConfig)
	assert.Contains(t, err.Error(), "Azure auth context not available")
}

func TestResolveGCPAuthContext_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockManager := types.NewMockAuthManager(ctrl)

	// Simulate realm-scoped credential path (as populated by auth system with realm).
	realmCredsFile := filepath.Join(".config", "atmos", "my-realm", "gcp", "gcp-adc", "adc", "gcp-prod", "application_default_credentials.json")

	managerStackInfo := &schema.ConfigAndStacksInfo{
		AuthContext: &schema.AuthContext{
			GCP: &schema.GCPAuthContext{
				CredentialsFile: realmCredsFile,
				ProjectID:       "my-gcp-project",
			},
		},
	}

	mockManager.EXPECT().
		Authenticate(gomock.Any(), "gcp-prod").
		Return(&types.WhoamiInfo{}, nil)
	mockManager.EXPECT().
		GetStackInfo().
		Return(managerStackInfo)

	resolver := NewResolver(mockManager, &schema.ConfigAndStacksInfo{})

	authConfig, err := resolver.ResolveGCPAuthContext(context.Background(), "gcp-prod")
	assert.NoError(t, err)
	assert.NotNil(t, authConfig)
	// Verify realm-scoped path and project ID flow through.
	assert.Equal(t, realmCredsFile, authConfig.CredentialsFile)
	assert.Equal(t, "my-gcp-project", authConfig.ProjectID)
}

func TestResolveGCPAuthContext_AuthFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockManager := types.NewMockAuthManager(ctrl)

	mockManager.EXPECT().
		Authenticate(gomock.Any(), "bad-identity").
		Return(nil, errors.New("authentication failed"))

	resolver := NewResolver(mockManager, &schema.ConfigAndStacksInfo{})

	authConfig, err := resolver.ResolveGCPAuthContext(context.Background(), "bad-identity")
	assert.Error(t, err)
	assert.Nil(t, authConfig)
}

func TestResolveGCPAuthContext_NoGCPContext(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockManager := types.NewMockAuthManager(ctrl)

	managerStackInfo := &schema.ConfigAndStacksInfo{
		AuthContext: &schema.AuthContext{},
	}

	mockManager.EXPECT().
		Authenticate(gomock.Any(), "aws-identity").
		Return(&types.WhoamiInfo{}, nil)
	mockManager.EXPECT().
		GetStackInfo().
		Return(managerStackInfo)

	resolver := NewResolver(mockManager, &schema.ConfigAndStacksInfo{})

	authConfig, err := resolver.ResolveGCPAuthContext(context.Background(), "aws-identity")
	assert.Error(t, err)
	assert.Nil(t, authConfig)
	assert.Contains(t, err.Error(), "GCP auth context not available")
}

func TestResolver_NilManagerStackInfo(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockManager := types.NewMockAuthManager(ctrl)

	// All three resolve methods call Authenticate then GetStackInfo, so expect three calls each.
	mockManager.EXPECT().
		Authenticate(gomock.Any(), "test-identity").
		Return(&types.WhoamiInfo{}, nil).
		Times(3)
	mockManager.EXPECT().
		GetStackInfo().
		Return(nil).
		Times(3)

	resolver := NewResolver(mockManager, nil)

	// All resolve methods should return error when the manager's stackInfo is nil.
	_, err := resolver.ResolveAWSAuthContext(context.Background(), "test-identity")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "AWS auth context not available")

	_, err = resolver.ResolveAzureAuthContext(context.Background(), "test-identity")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Azure auth context not available")

	_, err = resolver.ResolveGCPAuthContext(context.Background(), "test-identity")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "GCP auth context not available")
}
