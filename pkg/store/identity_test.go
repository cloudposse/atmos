package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// mockAuthContextResolver is a mock implementation of AuthContextResolver for testing.
type mockAuthContextResolver struct {
	mock.Mock
}

func (m *mockAuthContextResolver) ResolveAWSAuthContext(ctx context.Context, identityName string) (*AWSAuthConfig, error) {
	args := m.Called(ctx, identityName)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*AWSAuthConfig), args.Error(1)
}

func (m *mockAuthContextResolver) ResolveAzureAuthContext(ctx context.Context, identityName string) (*AzureAuthConfig, error) {
	args := m.Called(ctx, identityName)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*AzureAuthConfig), args.Error(1)
}

func (m *mockAuthContextResolver) ResolveGCPAuthContext(ctx context.Context, identityName string) (*GCPAuthConfig, error) {
	args := m.Called(ctx, identityName)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*GCPAuthConfig), args.Error(1)
}

func TestSetAuthContextResolver_MixedStores(t *testing.T) {
	// Create a registry with a mix of identity-aware and non-identity-aware stores.
	registry := make(StoreRegistry)

	// Create an SSM store with identity (identity-aware).
	ssmStore := &SSMStore{
		identityName:   "prod-admin",
		region:         "us-east-1",
		stackDelimiter: stringPtr("-"),
	}
	registry["prod-ssm"] = ssmStore

	// Create an SSM store without identity (also identity-aware but no identity set).
	noIdentityStore := &SSMStore{
		region:         "us-west-2",
		stackDelimiter: stringPtr("-"),
	}
	registry["default-ssm"] = noIdentityStore

	// Set the resolver.
	resolver := &mockAuthContextResolver{}
	registry.SetAuthContextResolver(resolver)

	// Verify that identity-aware stores got the resolver.
	assert.NotNil(t, ssmStore.authResolver)
	assert.Equal(t, "prod-admin", ssmStore.identityName) // Identity preserved.

	// Verify that the resolver was set even on stores without identity.
	assert.NotNil(t, noIdentityStore.authResolver)
	assert.Equal(t, "", noIdentityStore.identityName) // No identity set.
}

func TestSetAuthContext_DoesNotOverrideExistingIdentity(t *testing.T) {
	store := &SSMStore{
		identityName:   "original-identity",
		region:         "us-east-1",
		stackDelimiter: stringPtr("-"),
	}

	resolver := &mockAuthContextResolver{}

	// Calling SetAuthContext with empty identity should NOT override the existing one.
	store.SetAuthContext(resolver, "")
	assert.Equal(t, "original-identity", store.identityName)
	assert.NotNil(t, store.authResolver)

	// Calling with a non-empty identity should override.
	store.SetAuthContext(resolver, "new-identity")
	assert.Equal(t, "new-identity", store.identityName)
}

func TestSSMStore_LazyInit_WithIdentity(t *testing.T) {
	// Create an SSM store with identity — client should NOT be initialized immediately.
	store := &SSMStore{
		identityName:   "prod-admin",
		region:         "us-east-1",
		stackDelimiter: stringPtr("-"),
	}

	// Client should be nil since we have an identity (lazy init).
	assert.Nil(t, store.client)

	// Without a resolver, ensureClient should fail.
	err := store.ensureClient()
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrIdentityNotConfigured))
}

func TestSSMStore_LazyInit_ResolverError(t *testing.T) {
	resolver := &mockAuthContextResolver{}
	resolver.On("ResolveAWSAuthContext", mock.Anything, "bad-identity").
		Return(nil, errors.New("identity not found"))

	store := &SSMStore{
		identityName:   "bad-identity",
		region:         "us-east-1",
		stackDelimiter: stringPtr("-"),
		authResolver:   resolver,
	}

	err := store.ensureClient()
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrAuthContextNotAvailable))
	resolver.AssertExpectations(t)
}

func TestSSMStore_LazyInit_WithResolver(t *testing.T) {
	resolver := &mockAuthContextResolver{}
	resolver.On("ResolveAWSAuthContext", mock.Anything, "prod-admin").
		Return(&AWSAuthConfig{
			CredentialsFile: filepath.Join("tmp", "test-creds"),
			ConfigFile:      filepath.Join("tmp", "test-config"),
			Profile:         "prod",
			Region:          "us-east-1",
		}, nil)

	store := &SSMStore{
		identityName:   "prod-admin",
		region:         "us-east-1",
		stackDelimiter: stringPtr("-"),
		authResolver:   resolver,
	}

	// Note: ensureClient will try to load AWS config with the credentials file.
	// In test, this will fail because the files don't exist, but the resolver
	// should be called correctly.
	_ = store.ensureClient()
	resolver.AssertExpectations(t)
}

func TestAzureKeyVaultStore_SetAuthContext_PreservesIdentity(t *testing.T) {
	store := &AzureKeyVaultStore{
		identityName: "azure-prod",
		vaultURL:     "https://vault.example.com",
	}

	resolver := &mockAuthContextResolver{}
	store.SetAuthContext(resolver, "")
	assert.Equal(t, "azure-prod", store.identityName)
	assert.NotNil(t, store.authResolver)
}

func TestGSMStore_SetAuthContext_PreservesIdentity(t *testing.T) {
	store := &GSMStore{
		identityName: "gcp-prod",
		projectID:    "my-project",
	}

	resolver := &mockAuthContextResolver{}
	store.SetAuthContext(resolver, "")
	assert.Equal(t, "gcp-prod", store.identityName)
	assert.NotNil(t, store.authResolver)
}

func TestGSMStore_LazyInit_WithIdentity_NoResolver(t *testing.T) {
	store := &GSMStore{
		identityName: "gcp-prod",
		projectID:    "my-project",
	}

	assert.Nil(t, store.client)

	err := store.ensureClient()
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrIdentityNotConfigured))
}

func TestGSMStore_LazyInit_ResolverError(t *testing.T) {
	resolver := &mockAuthContextResolver{}
	resolver.On("ResolveGCPAuthContext", mock.Anything, "bad-identity").
		Return(nil, errors.New("identity not found"))

	store := &GSMStore{
		identityName: "bad-identity",
		projectID:    "my-project",
		authResolver: resolver,
	}

	err := store.ensureClient()
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrAuthContextNotAvailable))
	resolver.AssertExpectations(t)
}

func TestAzureKeyVaultStore_LazyInit_NoResolver(t *testing.T) {
	store := &AzureKeyVaultStore{
		identityName: "azure-prod",
		vaultURL:     "https://vault.example.com",
	}

	assert.Nil(t, store.client)

	err := store.ensureClient()
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrIdentityNotConfigured))
}

func TestAzureKeyVaultStore_LazyInit_ResolverError(t *testing.T) {
	resolver := &mockAuthContextResolver{}
	resolver.On("ResolveAzureAuthContext", mock.Anything, "bad-identity").
		Return(nil, errors.New("identity not found"))

	store := &AzureKeyVaultStore{
		identityName: "bad-identity",
		vaultURL:     "https://vault.example.com",
		authResolver: resolver,
	}

	err := store.ensureClient()
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrAuthContextNotAvailable))
	resolver.AssertExpectations(t)
}

func TestStoreConfig_IdentityField(t *testing.T) {
	config := StoreConfig{
		Type:     "aws-ssm-parameter-store",
		Identity: "prod-admin",
		Options:  map[string]interface{}{"region": "us-east-1"},
	}

	assert.Equal(t, "prod-admin", config.Identity)
	assert.Equal(t, "aws-ssm-parameter-store", config.Type)
}

func TestStoreConfig_IdentityEmpty(t *testing.T) {
	config := StoreConfig{
		Type:    "aws-ssm-parameter-store",
		Options: map[string]interface{}{"region": "us-east-1"},
	}

	assert.Empty(t, config.Identity)
}

func TestIdentityAwareStore_InterfaceCompliance(t *testing.T) {
	// Verify that all cloud stores implement IdentityAwareStore.
	var _ IdentityAwareStore = (*SSMStore)(nil)
	var _ IdentityAwareStore = (*AzureKeyVaultStore)(nil)
	var _ IdentityAwareStore = (*GSMStore)(nil)
}
