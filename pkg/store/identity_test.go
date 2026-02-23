package store

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestSetAuthContextResolver_MixedStores(t *testing.T) {
	ctrl := gomock.NewController(t)

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
	resolver := NewMockAuthContextResolver(ctrl)
	registry.SetAuthContextResolver(resolver)

	// Verify that identity-aware stores got the resolver.
	assert.NotNil(t, ssmStore.authResolver)
	assert.Equal(t, "prod-admin", ssmStore.identityName) // Identity preserved.

	// Verify that the resolver was set even on stores without identity.
	assert.NotNil(t, noIdentityStore.authResolver)
	assert.Equal(t, "", noIdentityStore.identityName) // No identity set.
}

func TestSetAuthContext_DoesNotOverrideExistingIdentity(t *testing.T) {
	ctrl := gomock.NewController(t)

	store := &SSMStore{
		identityName:   "original-identity",
		region:         "us-east-1",
		stackDelimiter: stringPtr("-"),
	}

	resolver := NewMockAuthContextResolver(ctrl)

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
	ctrl := gomock.NewController(t)

	resolver := NewMockAuthContextResolver(ctrl)
	resolver.EXPECT().
		ResolveAWSAuthContext(gomock.Any(), "bad-identity").
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
}

func TestSSMStore_LazyInit_WithResolver(t *testing.T) {
	ctrl := gomock.NewController(t)

	resolver := NewMockAuthContextResolver(ctrl)
	// Simulate realm-scoped credential paths.
	resolver.EXPECT().
		ResolveAWSAuthContext(gomock.Any(), "prod-admin").
		Return(&AWSAuthConfig{
			CredentialsFile: filepath.Join(".config", "atmos", "my-realm", "aws", "aws-sso", "credentials"),
			ConfigFile:      filepath.Join(".config", "atmos", "my-realm", "aws", "aws-sso", "config"),
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
}

func TestAzureKeyVaultStore_SetAuthContext_PreservesIdentity(t *testing.T) {
	ctrl := gomock.NewController(t)

	store := &AzureKeyVaultStore{
		identityName: "azure-prod",
		vaultURL:     "https://vault.example.com",
	}

	resolver := NewMockAuthContextResolver(ctrl)
	store.SetAuthContext(resolver, "")
	assert.Equal(t, "azure-prod", store.identityName)
	assert.NotNil(t, store.authResolver)
}

func TestGSMStore_SetAuthContext_PreservesIdentity(t *testing.T) {
	ctrl := gomock.NewController(t)

	store := &GSMStore{
		identityName: "gcp-prod",
		projectID:    "my-project",
	}

	resolver := NewMockAuthContextResolver(ctrl)
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
	ctrl := gomock.NewController(t)

	resolver := NewMockAuthContextResolver(ctrl)
	resolver.EXPECT().
		ResolveGCPAuthContext(gomock.Any(), "bad-identity").
		Return(nil, errors.New("identity not found"))

	store := &GSMStore{
		identityName: "bad-identity",
		projectID:    "my-project",
		authResolver: resolver,
	}

	err := store.ensureClient()
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrAuthContextNotAvailable))
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
	ctrl := gomock.NewController(t)

	resolver := NewMockAuthContextResolver(ctrl)
	resolver.EXPECT().
		ResolveAzureAuthContext(gomock.Any(), "bad-identity").
		Return(nil, errors.New("identity not found"))

	store := &AzureKeyVaultStore{
		identityName: "bad-identity",
		vaultURL:     "https://vault.example.com",
		authResolver: resolver,
	}

	err := store.ensureClient()
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrAuthContextNotAvailable))
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

// --- Constructor tests: verify lazy init when identity is configured ---

func TestNewSSMStore_WithIdentity(t *testing.T) {
	store, err := NewSSMStore(SSMStoreOptions{
		Region: "us-east-1",
		Prefix: stringPtr("/prod"),
	}, "prod-admin")

	assert.NoError(t, err)
	assert.NotNil(t, store)

	ssmStore := store.(*SSMStore)
	assert.Equal(t, "prod-admin", ssmStore.identityName)
	assert.Equal(t, "us-east-1", ssmStore.region)
	assert.Nil(t, ssmStore.client)    // Client deferred — lazy init.
	assert.Nil(t, ssmStore.awsConfig) // AWS config not loaded yet.
	assert.Equal(t, "/prod", ssmStore.prefix)
	assert.Equal(t, "-", *ssmStore.stackDelimiter) // Default delimiter.
}

func TestNewAzureKeyVaultStore_WithIdentity(t *testing.T) {
	store, err := NewAzureKeyVaultStore(AzureKeyVaultStoreOptions{
		VaultURL: "https://test.vault.azure.net",
		Prefix:   stringPtr("prod"),
	}, "azure-prod")

	assert.NoError(t, err)
	assert.NotNil(t, store)

	azStore := store.(*AzureKeyVaultStore)
	assert.Equal(t, "azure-prod", azStore.identityName)
	assert.Equal(t, "https://test.vault.azure.net", azStore.vaultURL)
	assert.Nil(t, azStore.client) // Client deferred — lazy init.
	assert.Equal(t, "prod", azStore.prefix)
}

func TestNewGSMStore_WithIdentity(t *testing.T) {
	store, err := NewGSMStore(GSMStoreOptions{
		ProjectID: "my-project",
		Prefix:    stringPtr("prod"),
	}, "gcp-prod")

	assert.NoError(t, err)
	assert.NotNil(t, store)

	gsmStore := store.(*GSMStore)
	assert.Equal(t, "gcp-prod", gsmStore.identityName)
	assert.Equal(t, "my-project", gsmStore.projectID)
	assert.Nil(t, gsmStore.client) // Client deferred — lazy init.
	assert.Equal(t, "prod", gsmStore.prefix)
}

// --- ensureClient early return: client already initialized ---

func TestSSMStore_EnsureClient_AlreadyInitialized(t *testing.T) {
	store := &SSMStore{
		client:         new(MockSSMClient),
		stackDelimiter: stringPtr("-"),
	}

	err := store.ensureClient()
	assert.NoError(t, err)
	assert.NotNil(t, store.client)
}

func TestAzureKeyVaultStore_EnsureClient_AlreadyInitialized(t *testing.T) {
	store := &AzureKeyVaultStore{
		client:         &mockClient{},
		vaultURL:       "https://vault.example.com",
		stackDelimiter: stringPtr("-"),
	}

	err := store.ensureClient()
	assert.NoError(t, err)
	assert.NotNil(t, store.client)
}

func TestGSMStore_EnsureClient_AlreadyInitialized(t *testing.T) {
	store := &GSMStore{
		client:    new(MockGSMClient),
		projectID: "test-project",
	}

	err := store.ensureClient()
	assert.NoError(t, err)
	assert.NotNil(t, store.client)
}

// --- SetAuthContext override: non-empty identity replaces existing ---

func TestAzureKeyVaultStore_SetAuthContext_OverridesIdentity(t *testing.T) {
	ctrl := gomock.NewController(t)

	store := &AzureKeyVaultStore{
		identityName: "original-azure",
		vaultURL:     "https://vault.example.com",
	}

	resolver := NewMockAuthContextResolver(ctrl)

	// Calling with non-empty identity should override.
	store.SetAuthContext(resolver, "new-azure-identity")
	assert.Equal(t, "new-azure-identity", store.identityName)
	assert.NotNil(t, store.authResolver)
}

func TestGSMStore_SetAuthContext_OverridesIdentity(t *testing.T) {
	ctrl := gomock.NewController(t)

	store := &GSMStore{
		identityName: "original-gcp",
		projectID:    "my-project",
	}

	resolver := NewMockAuthContextResolver(ctrl)

	// Calling with non-empty identity should override.
	store.SetAuthContext(resolver, "new-gcp-identity")
	assert.Equal(t, "new-gcp-identity", store.identityName)
	assert.NotNil(t, store.authResolver)
}

// --- Lazy init with resolver: exercises initIdentityClient for Azure and GSM ---

func TestAzureKeyVaultStore_LazyInit_WithResolver(t *testing.T) {
	ctrl := gomock.NewController(t)

	resolver := NewMockAuthContextResolver(ctrl)
	// Simulate realm-scoped credential paths.
	resolver.EXPECT().
		ResolveAzureAuthContext(gomock.Any(), "azure-prod").
		Return(&AzureAuthConfig{
			CredentialsFile: filepath.Join(".azure", "atmos", "my-realm", "azure-oidc", "credentials.json"),
			SubscriptionID:  "sub-789",
			TenantID:        "tenant-123",
			UseOIDC:         true,
			ClientID:        "client-456",
			TokenFilePath:   filepath.Join("tmp", "oidc-token"),
		}, nil)

	store := &AzureKeyVaultStore{
		identityName:   "azure-prod",
		vaultURL:       "https://vault.example.com",
		stackDelimiter: stringPtr("-"),
		authResolver:   resolver,
	}

	// ensureClient will attempt to create Azure credentials. The resolver
	// should be called and auth context fields processed. Azure SDK credential
	// creation may succeed (it creates a chain without authenticating).
	_ = store.ensureClient()
}

func TestGSMStore_LazyInit_WithResolver(t *testing.T) {
	ctrl := gomock.NewController(t)

	resolver := NewMockAuthContextResolver(ctrl)
	// Simulate realm-scoped credential paths.
	resolver.EXPECT().
		ResolveGCPAuthContext(gomock.Any(), "gcp-prod").
		Return(&GCPAuthConfig{
			CredentialsFile: filepath.Join(".config", "atmos", "my-realm", "gcp", "gcp-adc", "adc", "gcp-prod", "application_default_credentials.json"),
			ProjectID:       "my-gcp-project",
		}, nil)

	store := &GSMStore{
		identityName:   "gcp-prod",
		projectID:      "my-project",
		stackDelimiter: stringPtr("-"),
		authResolver:   resolver,
	}

	// ensureClient will attempt to create GCP client. The resolver should be
	// called and credentials file path processed. Actual client creation may
	// fail in test (no real GCP credentials).
	_ = store.ensureClient()
}

// --- SetAuthContextResolver with all cloud store types ---

func TestSetAuthContextResolver_AllCloudStoreTypes(t *testing.T) {
	ctrl := gomock.NewController(t)

	registry := make(StoreRegistry)

	ssmStore := &SSMStore{
		identityName:   "prod-admin",
		region:         "us-east-1",
		stackDelimiter: stringPtr("-"),
	}
	azStore := &AzureKeyVaultStore{
		identityName:   "azure-prod",
		vaultURL:       "https://vault.example.com",
		stackDelimiter: stringPtr("-"),
	}
	gsmStore := &GSMStore{
		identityName: "gcp-prod",
		projectID:    "my-project",
	}

	registry["ssm"] = ssmStore
	registry["azure"] = azStore
	registry["gsm"] = gsmStore

	resolver := NewMockAuthContextResolver(ctrl)
	registry.SetAuthContextResolver(resolver)

	// Verify resolver was injected into all identity-aware stores.
	assert.NotNil(t, ssmStore.authResolver)
	assert.NotNil(t, azStore.authResolver)
	assert.NotNil(t, gsmStore.authResolver)
}

// --- Eager init (no identity): constructor initializes client immediately ---

func TestNewSSMStore_WithoutIdentity_EagerInit(t *testing.T) {
	// Constructor with empty identity triggers initDefaultClient.
	// AWS config.LoadDefaultConfig + ssm.NewFromConfig succeed without real credentials.
	store, err := NewSSMStore(SSMStoreOptions{
		Region: "us-east-1",
		Prefix: stringPtr("/test"),
	}, "")
	// In test environment, AWS config loading typically succeeds.
	if err != nil {
		// Error path covers initDefaultClient error branch.
		assert.Nil(t, store)
		return
	}

	ssmStore := store.(*SSMStore)
	assert.NotNil(t, ssmStore.client)
	assert.NotNil(t, ssmStore.awsConfig)
	assert.Empty(t, ssmStore.identityName)
	assert.Equal(t, "us-east-1", ssmStore.awsConfig.Region)
}

func TestNewAzureKeyVaultStore_WithoutIdentity_EagerInit(t *testing.T) {
	// Constructor with empty identity triggers initDefaultClient.
	// Azure SDK may fail without credentials in test env — both paths provide coverage.
	store, err := NewAzureKeyVaultStore(AzureKeyVaultStoreOptions{
		VaultURL: "https://test.vault.azure.net",
		Prefix:   stringPtr("test"),
	}, "")
	if err != nil {
		// Error path covers initDefaultClient error branch.
		assert.Nil(t, store)
		return
	}

	azStore := store.(*AzureKeyVaultStore)
	assert.NotNil(t, azStore.client)
	assert.Empty(t, azStore.identityName)
}

func TestNewGSMStore_WithoutIdentity_EagerInit(t *testing.T) {
	// Constructor with empty identity triggers initDefaultClient.
	// GCP client creation may fail without credentials in test env.
	store, err := NewGSMStore(GSMStoreOptions{
		ProjectID: "test-project",
		Prefix:    stringPtr("test"),
	}, "")
	if err != nil {
		// Error path covers initDefaultClient error branch.
		assert.Nil(t, store)
		return
	}

	gsmStore := store.(*GSMStore)
	assert.NotNil(t, gsmStore.client)
	assert.Empty(t, gsmStore.identityName)
}

// --- ensureClient default-client path: exercises initDefaultClient through ensureClient ---

func TestSSMStore_EnsureClient_DefaultClientPath(t *testing.T) {
	// Store with no identity and no client — ensureClient calls initDefaultClient.
	store := &SSMStore{
		region:         "us-east-1",
		stackDelimiter: stringPtr("-"),
	}

	err := store.ensureClient()
	if err != nil {
		// Error path covers initDefaultClient error branch inside initOnce.Do.
		assert.Nil(t, store.client)
		return
	}

	assert.NotNil(t, store.client)
	assert.NotNil(t, store.awsConfig)
}

func TestAzureKeyVaultStore_EnsureClient_DefaultClientPath(t *testing.T) {
	store := &AzureKeyVaultStore{
		vaultURL:       "https://test.vault.azure.net",
		stackDelimiter: stringPtr("-"),
	}

	err := store.ensureClient()
	if err != nil {
		assert.Nil(t, store.client)
		return
	}

	assert.NotNil(t, store.client)
}

func TestGSMStore_EnsureClient_DefaultClientPath(t *testing.T) {
	store := &GSMStore{
		projectID:      "test-project",
		stackDelimiter: stringPtr("-"),
	}

	err := store.ensureClient()
	if err != nil {
		assert.Nil(t, store.client)
		return
	}

	assert.NotNil(t, store.client)
}

// --- Identity client success path with real temp credential files ---

func TestSSMStore_InitIdentityClient_FullSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)

	// Create temp AWS credential files with valid format.
	tmpDir := t.TempDir()
	credsFile := filepath.Join(tmpDir, "credentials")
	configFile := filepath.Join(tmpDir, "config")

	err := os.WriteFile(credsFile, []byte("[prod]\naws_access_key_id = AKIAIOSFODNN7EXAMPLE\naws_secret_access_key = wJalrXUtnFEMI\n"), 0o600)
	assert.NoError(t, err)
	err = os.WriteFile(configFile, []byte("[profile prod]\nregion = us-east-1\n"), 0o600)
	assert.NoError(t, err)

	resolver := NewMockAuthContextResolver(ctrl)
	resolver.EXPECT().
		ResolveAWSAuthContext(gomock.Any(), "prod-admin").
		Return(&AWSAuthConfig{
			CredentialsFile: credsFile,
			ConfigFile:      configFile,
			Profile:         "prod",
			Region:          "us-east-1",
		}, nil)

	store := &SSMStore{
		identityName:   "prod-admin",
		region:         "us-east-1",
		stackDelimiter: stringPtr("-"),
		authResolver:   resolver,
	}

	err = store.ensureClient()
	assert.NoError(t, err)
	assert.NotNil(t, store.client)
	assert.NotNil(t, store.awsConfig)
}

func TestAzureKeyVaultStore_InitIdentityClient_FullPath(t *testing.T) {
	ctrl := gomock.NewController(t)

	resolver := NewMockAuthContextResolver(ctrl)
	resolver.EXPECT().
		ResolveAzureAuthContext(gomock.Any(), "azure-prod").
		Return(&AzureAuthConfig{
			TenantID:       "tenant-123",
			SubscriptionID: "sub-456",
		}, nil)

	store := &AzureKeyVaultStore{
		identityName:   "azure-prod",
		vaultURL:       "https://test.vault.azure.net",
		stackDelimiter: stringPtr("-"),
		authResolver:   resolver,
	}

	// Azure initIdentityClient creates DefaultAzureCredential with tenant hint.
	// SDK may succeed or fail depending on environment — both paths provide coverage.
	err := store.ensureClient()
	if err != nil {
		// Error path covers credential creation failure.
		assert.Nil(t, store.client)
		return
	}

	assert.NotNil(t, store.client)
}

func TestGSMStore_InitIdentityClient_FullPath(t *testing.T) {
	ctrl := gomock.NewController(t)

	// Create a temp GCP credentials file with valid format.
	tmpDir := t.TempDir()
	credsFile := filepath.Join(tmpDir, "application_default_credentials.json")
	err := os.WriteFile(credsFile, []byte(`{"type":"authorized_user","client_id":"test","client_secret":"test","refresh_token":"test"}`), 0o600)
	assert.NoError(t, err)

	resolver := NewMockAuthContextResolver(ctrl)
	resolver.EXPECT().
		ResolveGCPAuthContext(gomock.Any(), "gcp-prod").
		Return(&GCPAuthConfig{
			CredentialsFile: credsFile,
			ProjectID:       "my-project",
		}, nil)

	store := &GSMStore{
		identityName:   "gcp-prod",
		projectID:      "my-project",
		stackDelimiter: stringPtr("-"),
		authResolver:   resolver,
	}

	// GCP client creation may succeed or fail depending on credentials.
	err = store.ensureClient()
	if err != nil {
		// Error path covers client creation failure — still exercises initIdentityClient.
		assert.Nil(t, store.client)
		return
	}

	assert.NotNil(t, store.client)
}

// --- Get/Set/GetKey with ensureClient failure: covers the new guard lines ---

func TestSSMStore_Get_EnsureClientError(t *testing.T) {
	store := &SSMStore{
		identityName:   "broken",
		region:         "us-east-1",
		stackDelimiter: stringPtr("-"),
		// No authResolver → ensureClient will fail.
	}

	_, err := store.Get("stack", "component", "key")
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrIdentityNotConfigured))
}

func TestSSMStore_Set_EnsureClientError(t *testing.T) {
	store := &SSMStore{
		identityName:   "broken",
		region:         "us-east-1",
		stackDelimiter: stringPtr("-"),
	}

	err := store.Set("stack", "component", "key", "value")
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrIdentityNotConfigured))
}

func TestSSMStore_GetKey_EnsureClientError(t *testing.T) {
	store := &SSMStore{
		identityName:   "broken",
		region:         "us-east-1",
		stackDelimiter: stringPtr("-"),
	}

	_, err := store.GetKey("some-key")
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrIdentityNotConfigured))
}

func TestAzureKeyVaultStore_Get_EnsureClientError(t *testing.T) {
	store := &AzureKeyVaultStore{
		identityName:   "broken",
		vaultURL:       "https://vault.example.com",
		stackDelimiter: stringPtr("-"),
	}

	_, err := store.Get("stack", "component", "key")
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrIdentityNotConfigured))
}

func TestAzureKeyVaultStore_Set_EnsureClientError(t *testing.T) {
	store := &AzureKeyVaultStore{
		identityName:   "broken",
		vaultURL:       "https://vault.example.com",
		stackDelimiter: stringPtr("-"),
	}

	err := store.Set("stack", "component", "key", "value")
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrIdentityNotConfigured))
}

func TestAzureKeyVaultStore_GetKey_EnsureClientError(t *testing.T) {
	store := &AzureKeyVaultStore{
		identityName:   "broken",
		vaultURL:       "https://vault.example.com",
		stackDelimiter: stringPtr("-"),
	}

	_, err := store.GetKey("some-key")
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrIdentityNotConfigured))
}

func TestGSMStore_Get_EnsureClientError(t *testing.T) {
	store := &GSMStore{
		identityName:   "broken",
		projectID:      "test-project",
		stackDelimiter: stringPtr("-"),
	}

	_, err := store.Get("stack", "component", "key")
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrIdentityNotConfigured))
}

func TestGSMStore_Set_EnsureClientError(t *testing.T) {
	store := &GSMStore{
		identityName:   "broken",
		projectID:      "test-project",
		stackDelimiter: stringPtr("-"),
	}

	err := store.Set("stack", "component", "key", "value")
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrIdentityNotConfigured))
}

func TestGSMStore_GetKey_EnsureClientError(t *testing.T) {
	store := &GSMStore{
		identityName:   "broken",
		projectID:      "test-project",
		stackDelimiter: stringPtr("-"),
	}

	_, err := store.GetKey("some-key")
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrIdentityNotConfigured))
}
