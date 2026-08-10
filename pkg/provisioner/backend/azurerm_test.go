package backend

import (
	"context"
	"errors"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// mockAzureBackendClient is a manual mock of the azureBackendAPI interface. A manual mock
// (func fields with safe defaults) is used — matching mockS3Client in s3_test.go — because
// the interface intentionally hides the Azure SDK's pollers behind simple methods.
type mockAzureBackendClient struct {
	resourceGroupExistsFunc  func(ctx context.Context, resourceGroup string) (bool, string, error)
	createResourceGroupFunc  func(ctx context.Context, resourceGroup, location string, tags map[string]*string) error
	storageAccountExistsFunc func(ctx context.Context, resourceGroup, account string) (bool, error)
	createStorageAccountFunc func(ctx context.Context, params azureStorageAccountParams) error
	applyDataProtectionFunc  func(ctx context.Context, resourceGroup, account string) error
	containerExistsFunc      func(ctx context.Context, resourceGroup, account, container string) (bool, error)
	createContainerFunc      func(ctx context.Context, resourceGroup, account, container string) error
	deleteStorageAccountFunc func(ctx context.Context, resourceGroup, account string) error

	// createdAccountParams captures the params passed to createStorageAccount for assertions.
	createdAccountParams *azureStorageAccountParams
}

func (m *mockAzureBackendClient) resourceGroupExists(ctx context.Context, resourceGroup string) (bool, string, error) {
	if m.resourceGroupExistsFunc != nil {
		return m.resourceGroupExistsFunc(ctx, resourceGroup)
	}
	return false, "", nil
}

func (m *mockAzureBackendClient) createResourceGroup(ctx context.Context, resourceGroup, location string, tags map[string]*string) error {
	if m.createResourceGroupFunc != nil {
		return m.createResourceGroupFunc(ctx, resourceGroup, location, tags)
	}
	return nil
}

func (m *mockAzureBackendClient) storageAccountExists(ctx context.Context, resourceGroup, account string) (bool, error) {
	if m.storageAccountExistsFunc != nil {
		return m.storageAccountExistsFunc(ctx, resourceGroup, account)
	}
	return false, nil
}

func (m *mockAzureBackendClient) createStorageAccount(ctx context.Context, params azureStorageAccountParams) error {
	m.createdAccountParams = &params
	if m.createStorageAccountFunc != nil {
		return m.createStorageAccountFunc(ctx, params)
	}
	return nil
}

func (m *mockAzureBackendClient) applyBlobDataProtection(ctx context.Context, resourceGroup, account string) error {
	if m.applyDataProtectionFunc != nil {
		return m.applyDataProtectionFunc(ctx, resourceGroup, account)
	}
	return nil
}

func (m *mockAzureBackendClient) containerExists(ctx context.Context, resourceGroup, account, container string) (bool, error) {
	if m.containerExistsFunc != nil {
		return m.containerExistsFunc(ctx, resourceGroup, account, container)
	}
	return false, nil
}

func (m *mockAzureBackendClient) createContainer(ctx context.Context, resourceGroup, account, container string) error {
	if m.createContainerFunc != nil {
		return m.createContainerFunc(ctx, resourceGroup, account, container)
	}
	return nil
}

func (m *mockAzureBackendClient) deleteStorageAccount(ctx context.Context, resourceGroup, account string) error {
	if m.deleteStorageAccountFunc != nil {
		return m.deleteStorageAccountFunc(ctx, resourceGroup, account)
	}
	return nil
}

// useMockAzureClient installs a mock client via the factory and restores the default on cleanup.
func useMockAzureClient(t *testing.T, mock azureBackendAPI) {
	t.Helper()
	SetAzureBackendClientFactory(func(string, *schema.AuthContext) (azureBackendAPI, error) {
		return mock, nil
	})
	t.Cleanup(ResetAzureBackendClientFactory)
}

// validAzurermBackendConfig returns a minimal valid azurerm backend config for tests.
func validAzurermBackendConfig() map[string]any {
	return map[string]any{
		"storage_account_name": "stcwtfstate",
		"container_name":       "tfstate",
		"resource_group_name":  "rg-tfstate",
		"subscription_id":      "00000000-0000-0000-0000-000000000000",
	}
}

func TestExtractAzurermConfig(t *testing.T) {
	tests := []struct {
		name          string
		backendConfig map[string]any
		authContext   *schema.AuthContext
		want          *azurermConfig
		wantErr       error
	}{
		{
			name:          "valid config with subscription in backend",
			backendConfig: validAzurermBackendConfig(),
			want: &azurermConfig{
				resourceGroupName:  "rg-tfstate",
				storageAccountName: "stcwtfstate",
				containerName:      "tfstate",
				subscriptionID:     "00000000-0000-0000-0000-000000000000",
			},
		},
		{
			name: "subscription and location sourced from auth context",
			backendConfig: map[string]any{
				"storage_account_name": "stcwtfstate",
				"container_name":       "tfstate",
				"resource_group_name":  "rg-tfstate",
			},
			authContext: &schema.AuthContext{Azure: &schema.AzureAuthContext{
				SubscriptionID: "11111111-1111-1111-1111-111111111111",
				Location:       "centralus",
			}},
			want: &azurermConfig{
				resourceGroupName:  "rg-tfstate",
				storageAccountName: "stcwtfstate",
				containerName:      "tfstate",
				subscriptionID:     "11111111-1111-1111-1111-111111111111",
				location:           "centralus",
			},
		},
		{
			name: "backend subscription_id takes precedence over auth context",
			backendConfig: map[string]any{
				"storage_account_name": "stcwtfstate",
				"container_name":       "tfstate",
				"resource_group_name":  "rg-tfstate",
				"subscription_id":      "backend-sub",
			},
			authContext: &schema.AuthContext{Azure: &schema.AzureAuthContext{SubscriptionID: "identity-sub"}},
			want: &azurermConfig{
				resourceGroupName:  "rg-tfstate",
				storageAccountName: "stcwtfstate",
				containerName:      "tfstate",
				subscriptionID:     "backend-sub",
			},
		},
		{
			name: "use_azuread_auth bool true hardens the account",
			backendConfig: map[string]any{
				"storage_account_name": "stcwtfstate",
				"container_name":       "tfstate",
				"resource_group_name":  "rg-tfstate",
				"subscription_id":      "sub",
				"use_azuread_auth":     true,
			},
			want: &azurermConfig{
				resourceGroupName:  "rg-tfstate",
				storageAccountName: "stcwtfstate",
				containerName:      "tfstate",
				subscriptionID:     "sub",
				useAzureADAuth:     true,
			},
		},
		{
			name: "use_azuread_auth string true is honored",
			backendConfig: map[string]any{
				"storage_account_name": "stcwtfstate",
				"container_name":       "tfstate",
				"resource_group_name":  "rg-tfstate",
				"subscription_id":      "sub",
				"use_azuread_auth":     "TRUE",
			},
			want: &azurermConfig{
				resourceGroupName:  "rg-tfstate",
				storageAccountName: "stcwtfstate",
				containerName:      "tfstate",
				subscriptionID:     "sub",
				useAzureADAuth:     true,
			},
		},
		{
			name: "missing storage_account_name",
			backendConfig: map[string]any{
				"container_name":      "tfstate",
				"resource_group_name": "rg-tfstate",
				"subscription_id":     "sub",
			},
			wantErr: errUtils.ErrStorageAccountRequired,
		},
		{
			name: "missing container_name",
			backendConfig: map[string]any{
				"storage_account_name": "stcwtfstate",
				"resource_group_name":  "rg-tfstate",
				"subscription_id":      "sub",
			},
			wantErr: errUtils.ErrAzureContainerRequired,
		},
		{
			name: "missing resource_group_name",
			backendConfig: map[string]any{
				"storage_account_name": "stcwtfstate",
				"container_name":       "tfstate",
				"subscription_id":      "sub",
			},
			wantErr: errUtils.ErrResourceGroupRequired,
		},
		{
			name: "missing subscription everywhere",
			backendConfig: map[string]any{
				"storage_account_name": "stcwtfstate",
				"container_name":       "tfstate",
				"resource_group_name":  "rg-tfstate",
			},
			wantErr: errUtils.ErrAzureSubscriptionRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractAzurermConfig(tt.backendConfig, tt.authContext)
			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestExtractUseAzureADAuth(t *testing.T) {
	tests := []struct {
		name string
		val  any
		want bool
	}{
		{name: "bool true", val: true, want: true},
		{name: "bool false", val: false, want: false},
		{name: "string true", val: "true", want: true},
		{name: "string mixed case", val: "True", want: true},
		{name: "string false", val: "false", want: false},
		{name: "unrelated string", val: "yes", want: false},
		{name: "nil/missing", val: nil, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := map[string]any{}
			if tt.val != nil {
				cfg["use_azuread_auth"] = tt.val
			}
			assert.Equal(t, tt.want, extractUseAzureADAuth(cfg))
		})
	}
}

func TestAzurermBackendName(t *testing.T) {
	assert.Equal(t, "stcwtfstate", AzurermBackendName(map[string]any{"storage_account_name": "stcwtfstate"}))
	assert.Equal(t, "", AzurermBackendName(map[string]any{}))
	assert.Equal(t, "", AzurermBackendName(map[string]any{"storage_account_name": ""}))
}

func TestIsAzureNotFound(t *testing.T) {
	assert.True(t, isAzureNotFound(&azcore.ResponseError{StatusCode: 404}))
	assert.False(t, isAzureNotFound(&azcore.ResponseError{StatusCode: 403}))
	assert.False(t, isAzureNotFound(errors.New("some other error")))
	assert.False(t, isAzureNotFound(nil))
}

func TestCreateAzurermBackend_FullCreate(t *testing.T) {
	var createdRG, createdContainer bool
	dataProtectionApplied := false
	mock := &mockAzureBackendClient{
		resourceGroupExistsFunc: func(context.Context, string) (bool, string, error) { return false, "", nil },
		createResourceGroupFunc: func(context.Context, string, string, map[string]*string) error {
			createdRG = true
			return nil
		},
		storageAccountExistsFunc: func(context.Context, string, string) (bool, error) { return false, nil },
		applyDataProtectionFunc: func(context.Context, string, string) error {
			dataProtectionApplied = true
			return nil
		},
		containerExistsFunc: func(context.Context, string, string, string) (bool, error) { return false, nil },
		createContainerFunc: func(context.Context, string, string, string) error {
			createdContainer = true
			return nil
		},
	}
	useMockAzureClient(t, mock)

	authContext := &schema.AuthContext{Azure: &schema.AzureAuthContext{
		SubscriptionID: "sub",
		Location:       "centralus",
	}}
	backendConfig := map[string]any{
		"storage_account_name": "stcwtfstate",
		"container_name":       "tfstate",
		"resource_group_name":  "rg-tfstate",
		"use_azuread_auth":     true,
	}

	result, err := CreateAzurermBackend(context.Background(), nil, backendConfig, authContext)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, result.Warnings, "no warnings expected when nothing pre-existed")
	assert.True(t, createdRG, "resource group should be created")
	assert.True(t, dataProtectionApplied, "blob data protection should be applied")
	assert.True(t, createdContainer, "container should be created")

	require.NotNil(t, mock.createdAccountParams)
	assert.Equal(t, "centralus", mock.createdAccountParams.location, "account created in identity location")
	assert.True(t, mock.createdAccountParams.disableSharedKey, "shared key disabled when use_azuread_auth is true")

	// The hardcoded default tags (Name=<account>, ManagedBy=Atmos) must be applied.
	require.NotNil(t, mock.createdAccountParams.tags["Name"])
	require.NotNil(t, mock.createdAccountParams.tags["ManagedBy"])
	assert.Equal(t, "stcwtfstate", *mock.createdAccountParams.tags["Name"])
	assert.Equal(t, "Atmos", *mock.createdAccountParams.tags["ManagedBy"])
}

func TestCreateAzurermBackend_ResourceGroupExistsUsesItsLocation(t *testing.T) {
	mock := &mockAzureBackendClient{
		resourceGroupExistsFunc:  func(context.Context, string) (bool, string, error) { return true, "eastus", nil },
		storageAccountExistsFunc: func(context.Context, string, string) (bool, error) { return false, nil },
	}
	useMockAzureClient(t, mock)

	// No location on the identity — must be inherited from the existing resource group.
	authContext := &schema.AuthContext{Azure: &schema.AzureAuthContext{SubscriptionID: "sub"}}

	_, err := CreateAzurermBackend(context.Background(), nil, validAzurermBackendConfig(), authContext)
	require.NoError(t, err)
	require.NotNil(t, mock.createdAccountParams)
	assert.Equal(t, "eastus", mock.createdAccountParams.location)
	assert.False(t, mock.createdAccountParams.disableSharedKey, "shared key left enabled without use_azuread_auth")
}

func TestCreateAzurermBackend_ExistingAccountWarns(t *testing.T) {
	mock := &mockAzureBackendClient{
		resourceGroupExistsFunc:  func(context.Context, string) (bool, string, error) { return true, "eastus", nil },
		storageAccountExistsFunc: func(context.Context, string, string) (bool, error) { return true, nil },
		containerExistsFunc:      func(context.Context, string, string, string) (bool, error) { return true, nil },
	}
	useMockAzureClient(t, mock)

	result, err := CreateAzurermBackend(context.Background(), nil, validAzurermBackendConfig(), nil)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Warnings, 1)
	assert.Contains(t, result.Warnings[0], "existing storage account")
}

func TestCreateAzurermBackend_LocationRequiredWhenCreatingRG(t *testing.T) {
	mock := &mockAzureBackendClient{
		resourceGroupExistsFunc: func(context.Context, string) (bool, string, error) { return false, "", nil },
	}
	useMockAzureClient(t, mock)

	// Resource group missing AND no location available anywhere.
	_, err := CreateAzurermBackend(context.Background(), nil, validAzurermBackendConfig(), nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAzureLocationRequired)
}

func TestCreateAzurermBackend_ErrorsPropagate(t *testing.T) {
	sentinel := errors.New("boom")
	tests := []struct {
		name    string
		mock    *mockAzureBackendClient
		wantErr error
	}{
		{
			name: "resource group check fails",
			mock: &mockAzureBackendClient{
				resourceGroupExistsFunc: func(context.Context, string) (bool, string, error) { return false, "", sentinel },
			},
			wantErr: errUtils.ErrCheckResourceGroupExist,
		},
		{
			name: "resource group create fails",
			mock: &mockAzureBackendClient{
				resourceGroupExistsFunc: func(context.Context, string) (bool, string, error) { return false, "", nil },
				createResourceGroupFunc: func(context.Context, string, string, map[string]*string) error { return sentinel },
			},
			wantErr: errUtils.ErrCreateResourceGroup,
		},
		{
			name: "storage account check fails",
			mock: &mockAzureBackendClient{
				resourceGroupExistsFunc:  func(context.Context, string) (bool, string, error) { return true, "eastus", nil },
				storageAccountExistsFunc: func(context.Context, string, string) (bool, error) { return false, sentinel },
			},
			wantErr: errUtils.ErrCheckStorageAccountExist,
		},
		{
			name: "storage account create fails",
			mock: &mockAzureBackendClient{
				resourceGroupExistsFunc:  func(context.Context, string) (bool, string, error) { return true, "eastus", nil },
				storageAccountExistsFunc: func(context.Context, string, string) (bool, error) { return false, nil },
				createStorageAccountFunc: func(context.Context, azureStorageAccountParams) error { return sentinel },
			},
			wantErr: errUtils.ErrCreateStorageAccount,
		},
		{
			name: "data protection fails",
			mock: &mockAzureBackendClient{
				resourceGroupExistsFunc:  func(context.Context, string) (bool, string, error) { return true, "eastus", nil },
				storageAccountExistsFunc: func(context.Context, string, string) (bool, error) { return true, nil },
				applyDataProtectionFunc:  func(context.Context, string, string) error { return sentinel },
			},
			wantErr: errUtils.ErrApplyBlobDataProtection,
		},
		{
			name: "container check fails",
			mock: &mockAzureBackendClient{
				resourceGroupExistsFunc:  func(context.Context, string) (bool, string, error) { return true, "eastus", nil },
				storageAccountExistsFunc: func(context.Context, string, string) (bool, error) { return true, nil },
				containerExistsFunc:      func(context.Context, string, string, string) (bool, error) { return false, sentinel },
			},
			wantErr: errUtils.ErrCheckContainerExist,
		},
		{
			name: "container create fails",
			mock: &mockAzureBackendClient{
				resourceGroupExistsFunc:  func(context.Context, string) (bool, string, error) { return true, "eastus", nil },
				storageAccountExistsFunc: func(context.Context, string, string) (bool, error) { return true, nil },
				containerExistsFunc:      func(context.Context, string, string, string) (bool, error) { return false, nil },
				createContainerFunc:      func(context.Context, string, string, string) error { return sentinel },
			},
			wantErr: errUtils.ErrCreateStorageContainer,
		},
	}

	// Provide a location so the "resource group create fails" case reaches the create call
	// (an empty location would short-circuit with ErrAzureLocationRequired first). Cases whose
	// resource group already exists ignore it.
	authContext := &schema.AuthContext{Azure: &schema.AzureAuthContext{SubscriptionID: "sub", Location: "centralus"}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			useMockAzureClient(t, tt.mock)
			_, err := CreateAzurermBackend(context.Background(), nil, validAzurermBackendConfig(), authContext)
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestAzurermBackendExists(t *testing.T) {
	tests := []struct {
		name          string
		accountExists bool
		accountErr    error
		containerFunc func(context.Context, string, string, string) (bool, error)
		want          bool
		wantErr       bool
		wantSentinel  error
	}{
		{
			name:          "account missing",
			accountExists: false,
			want:          false,
		},
		{
			name:          "account exists but container missing",
			accountExists: true,
			containerFunc: func(context.Context, string, string, string) (bool, error) { return false, nil },
			want:          false,
		},
		{
			name:          "account and container exist",
			accountExists: true,
			containerFunc: func(context.Context, string, string, string) (bool, error) { return true, nil },
			want:          true,
		},
		{
			name:         "account check error",
			accountErr:   errors.New("boom"),
			wantErr:      true,
			wantSentinel: errUtils.ErrCheckStorageAccountExist,
		},
		{
			name:          "container check error",
			accountExists: true,
			containerFunc: func(context.Context, string, string, string) (bool, error) { return false, errors.New("boom") },
			wantErr:       true,
			wantSentinel:  errUtils.ErrCheckContainerExist,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockAzureBackendClient{
				storageAccountExistsFunc: func(context.Context, string, string) (bool, error) {
					return tt.accountExists, tt.accountErr
				},
				containerExistsFunc: tt.containerFunc,
			}
			useMockAzureClient(t, mock)

			got, err := AzurermBackendExists(context.Background(), nil, validAzurermBackendConfig(), nil)
			if tt.wantErr {
				require.Error(t, err)
				if tt.wantSentinel != nil {
					assert.ErrorIs(t, err, tt.wantSentinel)
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDeleteAzurermBackend(t *testing.T) {
	t.Run("force required", func(t *testing.T) {
		mock := &mockAzureBackendClient{}
		useMockAzureClient(t, mock)
		err := DeleteAzurermBackend(context.Background(), nil, validAzurermBackendConfig(), nil, false)
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrForceRequired)
	})

	t.Run("account not found", func(t *testing.T) {
		mock := &mockAzureBackendClient{
			storageAccountExistsFunc: func(context.Context, string, string) (bool, error) { return false, nil },
		}
		useMockAzureClient(t, mock)
		err := DeleteAzurermBackend(context.Background(), nil, validAzurermBackendConfig(), nil, true)
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrBackendNotFound)
	})

	t.Run("successful delete", func(t *testing.T) {
		deleted := false
		mock := &mockAzureBackendClient{
			storageAccountExistsFunc: func(context.Context, string, string) (bool, error) { return true, nil },
			deleteStorageAccountFunc: func(context.Context, string, string) error {
				deleted = true
				return nil
			},
		}
		useMockAzureClient(t, mock)
		err := DeleteAzurermBackend(context.Background(), nil, validAzurermBackendConfig(), nil, true)
		require.NoError(t, err)
		assert.True(t, deleted)
	})

	t.Run("delete error propagates", func(t *testing.T) {
		mock := &mockAzureBackendClient{
			storageAccountExistsFunc: func(context.Context, string, string) (bool, error) { return true, nil },
			deleteStorageAccountFunc: func(context.Context, string, string) error { return errors.New("boom") },
		}
		useMockAzureClient(t, mock)
		err := DeleteAzurermBackend(context.Background(), nil, validAzurermBackendConfig(), nil, true)
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrDeleteStorageAccount)
	})

	t.Run("storage account check error", func(t *testing.T) {
		mock := &mockAzureBackendClient{
			storageAccountExistsFunc: func(context.Context, string, string) (bool, error) { return false, errors.New("boom") },
		}
		useMockAzureClient(t, mock)
		err := DeleteAzurermBackend(context.Background(), nil, validAzurermBackendConfig(), nil, true)
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrCheckStorageAccountExist)
	})
}

// TestAzurerm_InvalidConfigPropagates covers the extractAzurermConfig error branch shared by
// Create/Exists/Delete — a bad config must surface before any client is built.
func TestAzurerm_InvalidConfigPropagates(t *testing.T) {
	// Missing storage_account_name.
	bad := map[string]any{
		"container_name":      "tfstate",
		"resource_group_name": "rg-tfstate",
		"subscription_id":     "sub",
	}

	_, err := CreateAzurermBackend(context.Background(), nil, bad, nil)
	assert.ErrorIs(t, err, errUtils.ErrStorageAccountRequired)

	_, err = AzurermBackendExists(context.Background(), nil, bad, nil)
	assert.ErrorIs(t, err, errUtils.ErrStorageAccountRequired)

	err = DeleteAzurermBackend(context.Background(), nil, bad, nil, true)
	assert.ErrorIs(t, err, errUtils.ErrStorageAccountRequired)
}

func TestNewAzureBackendClient(t *testing.T) {
	// Client construction is offline — DefaultAzureCredential and the ARM client
	// constructors build structs without any network call, so this is safe to unit test.
	// The wrapper methods themselves (resourceGroupExists, createStorageAccount, ...) are
	// thin passthroughs to the Azure management plane and are exercised by integration tests
	// against real Azure, not here — mirroring how s3.go's real client construction is not
	// unit-tested (the S3 unit tests inject a fake client instead).
	client, err := newAzureBackendClient("00000000-0000-0000-0000-000000000000", nil)
	require.NoError(t, err)
	require.NotNil(t, client)

	impl, ok := client.(*azureBackendClient)
	require.True(t, ok)
	assert.NotNil(t, impl.resourceGroups)
	assert.NotNil(t, impl.accounts)
	assert.NotNil(t, impl.blobServices)
	assert.NotNil(t, impl.blobContainers)
}

func TestAzurermBackendRegisteredInRegistry(t *testing.T) {
	// The init() registrations must wire azurerm into the shared backend registry so the
	// auto-provision hook and `atmos terraform backend` commands can find it.
	assert.NotNil(t, GetBackendCreate(backendTypeAzurerm), "create func registered")
	assert.NotNil(t, GetBackendDelete(backendTypeAzurerm), "delete func registered")
	assert.NotNil(t, GetBackendExists(backendTypeAzurerm), "exists func registered")
	assert.NotNil(t, GetBackendName(backendTypeAzurerm), "name func registered")
}
