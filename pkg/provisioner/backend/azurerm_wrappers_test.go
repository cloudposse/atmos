package backend

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	azfake "github.com/Azure/azure-sdk-for-go/sdk/azcore/fake"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	armresourcesfake "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources/fake"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage/v4"
	armstoragefake "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage/v4/fake"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

const fakeSubID = "00000000-0000-0000-0000-000000000000"

// newFakeAzureBackendClient builds a real *azureBackendClient whose ARM SDK clients are wired to
// in-memory Azure SDK fake servers (no network). This exercises the concrete passthrough
// wrappers — the 404→exists mapping, poller completion, and error propagation — that the
// interface-mocked tests can't reach.
func newFakeAzureBackendClient(t *testing.T, rgSrv *armresourcesfake.ResourceGroupsServer, stSrv *armstoragefake.ServerFactory) *azureBackendClient {
	t.Helper()
	cred := &azfake.TokenCredential{}

	rgClient, err := armresources.NewResourceGroupsClient(fakeSubID, cred, &arm.ClientOptions{
		ClientOptions: azcore.ClientOptions{Transport: armresourcesfake.NewResourceGroupsServerTransport(rgSrv)},
	})
	require.NoError(t, err)

	stFactory, err := armstorage.NewClientFactory(fakeSubID, cred, &arm.ClientOptions{
		ClientOptions: azcore.ClientOptions{Transport: armstoragefake.NewServerFactoryTransport(stSrv)},
	})
	require.NoError(t, err)

	return &azureBackendClient{
		resourceGroups: rgClient,
		accounts:       stFactory.NewAccountsClient(),
		blobServices:   stFactory.NewBlobServicesClient(),
		blobContainers: stFactory.NewBlobContainersClient(),
	}
}

// mkResp returns a fake Responder carrying a successful body.
func mkResp[T any](status int, body T) azfake.Responder[T] {
	var r azfake.Responder[T]
	r.SetResponse(status, body, nil)
	return r
}

// mkErr returns a fake ErrorResponder for the given HTTP status + Azure error code.
func mkErr(status int, code string) azfake.ErrorResponder {
	var e azfake.ErrorResponder
	e.SetResponseError(status, code)
	return e
}

func TestAzureBackendClient_ResourceGroupExists(t *testing.T) {
	t.Run("exists returns location", func(t *testing.T) {
		rg := &armresourcesfake.ResourceGroupsServer{
			Get: func(_ context.Context, _ string, _ *armresources.ResourceGroupsClientGetOptions) (azfake.Responder[armresources.ResourceGroupsClientGetResponse], azfake.ErrorResponder) {
				return mkResp(http.StatusOK, armresources.ResourceGroupsClientGetResponse{
					ResourceGroup: armresources.ResourceGroup{Location: to.Ptr("eastus")},
				}), azfake.ErrorResponder{}
			},
		}
		client := newFakeAzureBackendClient(t, rg, &armstoragefake.ServerFactory{})
		exists, loc, err := client.resourceGroupExists(context.Background(), "rg")
		require.NoError(t, err)
		assert.True(t, exists)
		assert.Equal(t, "eastus", loc)
	})

	t.Run("not found returns false", func(t *testing.T) {
		rg := &armresourcesfake.ResourceGroupsServer{
			Get: func(_ context.Context, _ string, _ *armresources.ResourceGroupsClientGetOptions) (azfake.Responder[armresources.ResourceGroupsClientGetResponse], azfake.ErrorResponder) {
				return azfake.Responder[armresources.ResourceGroupsClientGetResponse]{}, mkErr(http.StatusNotFound, "ResourceGroupNotFound")
			},
		}
		client := newFakeAzureBackendClient(t, rg, &armstoragefake.ServerFactory{})
		exists, _, err := client.resourceGroupExists(context.Background(), "rg")
		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("other error propagates", func(t *testing.T) {
		rg := &armresourcesfake.ResourceGroupsServer{
			Get: func(_ context.Context, _ string, _ *armresources.ResourceGroupsClientGetOptions) (azfake.Responder[armresources.ResourceGroupsClientGetResponse], azfake.ErrorResponder) {
				return azfake.Responder[armresources.ResourceGroupsClientGetResponse]{}, mkErr(http.StatusForbidden, "AuthorizationFailed")
			},
		}
		client := newFakeAzureBackendClient(t, rg, &armstoragefake.ServerFactory{})
		_, _, err := client.resourceGroupExists(context.Background(), "rg")
		require.Error(t, err)
	})
}

func TestAzureBackendClient_CreateResourceGroup(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		var gotLocation string
		rg := &armresourcesfake.ResourceGroupsServer{
			CreateOrUpdate: func(_ context.Context, _ string, params armresources.ResourceGroup, _ *armresources.ResourceGroupsClientCreateOrUpdateOptions) (azfake.Responder[armresources.ResourceGroupsClientCreateOrUpdateResponse], azfake.ErrorResponder) {
				gotLocation = *params.Location
				return mkResp(http.StatusOK, armresources.ResourceGroupsClientCreateOrUpdateResponse{}), azfake.ErrorResponder{}
			},
		}
		client := newFakeAzureBackendClient(t, rg, &armstoragefake.ServerFactory{})
		err := client.createResourceGroup(context.Background(), "rg", "centralus", azureBackendTags("st"))
		require.NoError(t, err)
		assert.Equal(t, "centralus", gotLocation)
	})

	t.Run("error propagates", func(t *testing.T) {
		rg := &armresourcesfake.ResourceGroupsServer{
			CreateOrUpdate: func(_ context.Context, _ string, _ armresources.ResourceGroup, _ *armresources.ResourceGroupsClientCreateOrUpdateOptions) (azfake.Responder[armresources.ResourceGroupsClientCreateOrUpdateResponse], azfake.ErrorResponder) {
				return azfake.Responder[armresources.ResourceGroupsClientCreateOrUpdateResponse]{}, mkErr(http.StatusForbidden, "AuthorizationFailed")
			},
		}
		client := newFakeAzureBackendClient(t, rg, &armstoragefake.ServerFactory{})
		err := client.createResourceGroup(context.Background(), "rg", "centralus", nil)
		require.Error(t, err)
	})
}

func TestAzureBackendClient_StorageAccountExists(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		code       string
		wantExists bool
		wantErr    bool
	}{
		{name: "exists", status: http.StatusOK, wantExists: true},
		{name: "not found", status: http.StatusNotFound, code: "ResourceNotFound", wantExists: false},
		{name: "forbidden", status: http.StatusForbidden, code: "AuthorizationFailed", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			acct := armstoragefake.AccountsServer{
				GetProperties: func(_ context.Context, _, _ string, _ *armstorage.AccountsClientGetPropertiesOptions) (azfake.Responder[armstorage.AccountsClientGetPropertiesResponse], azfake.ErrorResponder) {
					if tt.code != "" {
						return azfake.Responder[armstorage.AccountsClientGetPropertiesResponse]{}, mkErr(tt.status, tt.code)
					}
					return mkResp(tt.status, armstorage.AccountsClientGetPropertiesResponse{Account: armstorage.Account{}}), azfake.ErrorResponder{}
				},
			}
			client := newFakeAzureBackendClient(t, &armresourcesfake.ResourceGroupsServer{}, &armstoragefake.ServerFactory{AccountsServer: acct})
			exists, err := client.storageAccountExists(context.Background(), "rg", "st")
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantExists, exists)
		})
	}
}

func TestAzureBackendClient_CreateStorageAccount(t *testing.T) {
	t.Run("success passes params through", func(t *testing.T) {
		var gotSKU armstorage.SKUName
		var gotTLS armstorage.MinimumTLSVersion
		var gotSharedKey *bool
		acct := armstoragefake.AccountsServer{
			BeginCreate: func(_ context.Context, _, _ string, params armstorage.AccountCreateParameters, _ *armstorage.AccountsClientBeginCreateOptions) (azfake.PollerResponder[armstorage.AccountsClientCreateResponse], azfake.ErrorResponder) {
				gotSKU = *params.SKU.Name
				gotTLS = *params.Properties.MinimumTLSVersion
				gotSharedKey = params.Properties.AllowSharedKeyAccess
				var resp azfake.PollerResponder[armstorage.AccountsClientCreateResponse]
				resp.SetTerminalResponse(http.StatusOK, armstorage.AccountsClientCreateResponse{Account: armstorage.Account{}}, nil)
				return resp, azfake.ErrorResponder{}
			},
		}
		client := newFakeAzureBackendClient(t, &armresourcesfake.ResourceGroupsServer{}, &armstoragefake.ServerFactory{AccountsServer: acct})
		err := client.createStorageAccount(context.Background(), azureStorageAccountParams{
			resourceGroup: "rg", account: "st", location: "eastus", disableSharedKey: true, tags: azureBackendTags("st"),
		})
		require.NoError(t, err)
		assert.Equal(t, armstorage.SKUNameStandardLRS, gotSKU)
		assert.Equal(t, armstorage.MinimumTLSVersionTLS12, gotTLS)
		require.NotNil(t, gotSharedKey)
		assert.False(t, *gotSharedKey, "disableSharedKey=true → AllowSharedKeyAccess=false")
	})

	t.Run("shared key left unset when not disabled", func(t *testing.T) {
		var gotSharedKey *bool
		called := false
		acct := armstoragefake.AccountsServer{
			BeginCreate: func(_ context.Context, _, _ string, params armstorage.AccountCreateParameters, _ *armstorage.AccountsClientBeginCreateOptions) (azfake.PollerResponder[armstorage.AccountsClientCreateResponse], azfake.ErrorResponder) {
				called = true
				gotSharedKey = params.Properties.AllowSharedKeyAccess
				var resp azfake.PollerResponder[armstorage.AccountsClientCreateResponse]
				resp.SetTerminalResponse(http.StatusOK, armstorage.AccountsClientCreateResponse{Account: armstorage.Account{}}, nil)
				return resp, azfake.ErrorResponder{}
			},
		}
		client := newFakeAzureBackendClient(t, &armresourcesfake.ResourceGroupsServer{}, &armstoragefake.ServerFactory{AccountsServer: acct})
		err := client.createStorageAccount(context.Background(), azureStorageAccountParams{
			resourceGroup: "rg", account: "st", location: "eastus", disableSharedKey: false,
		})
		require.NoError(t, err)
		assert.True(t, called)
		assert.Nil(t, gotSharedKey, "AllowSharedKeyAccess stays at the Azure default")
	})

	t.Run("error propagates", func(t *testing.T) {
		acct := armstoragefake.AccountsServer{
			BeginCreate: func(_ context.Context, _, _ string, _ armstorage.AccountCreateParameters, _ *armstorage.AccountsClientBeginCreateOptions) (azfake.PollerResponder[armstorage.AccountsClientCreateResponse], azfake.ErrorResponder) {
				return azfake.PollerResponder[armstorage.AccountsClientCreateResponse]{}, mkErr(http.StatusForbidden, "AuthorizationFailed")
			},
		}
		client := newFakeAzureBackendClient(t, &armresourcesfake.ResourceGroupsServer{}, &armstoragefake.ServerFactory{AccountsServer: acct})
		err := client.createStorageAccount(context.Background(), azureStorageAccountParams{resourceGroup: "rg", account: "st", location: "eastus"})
		require.Error(t, err)
	})
}

func TestAzureBackendClient_ApplyBlobDataProtection(t *testing.T) {
	t.Run("success enables versioning + soft delete", func(t *testing.T) {
		var versioning bool
		var days int32
		svc := armstoragefake.BlobServicesServer{
			SetServiceProperties: func(_ context.Context, _, _ string, params armstorage.BlobServiceProperties, _ *armstorage.BlobServicesClientSetServicePropertiesOptions) (azfake.Responder[armstorage.BlobServicesClientSetServicePropertiesResponse], azfake.ErrorResponder) {
				versioning = *params.BlobServiceProperties.IsVersioningEnabled
				days = *params.BlobServiceProperties.DeleteRetentionPolicy.Days
				return mkResp(http.StatusOK, armstorage.BlobServicesClientSetServicePropertiesResponse{}), azfake.ErrorResponder{}
			},
		}
		client := newFakeAzureBackendClient(t, &armresourcesfake.ResourceGroupsServer{}, &armstoragefake.ServerFactory{BlobServicesServer: svc})
		err := client.applyBlobDataProtection(context.Background(), "rg", "st")
		require.NoError(t, err)
		assert.True(t, versioning)
		assert.Equal(t, blobSoftDeleteRetentionDays, days)
	})

	t.Run("error propagates", func(t *testing.T) {
		svc := armstoragefake.BlobServicesServer{
			SetServiceProperties: func(_ context.Context, _, _ string, _ armstorage.BlobServiceProperties, _ *armstorage.BlobServicesClientSetServicePropertiesOptions) (azfake.Responder[armstorage.BlobServicesClientSetServicePropertiesResponse], azfake.ErrorResponder) {
				return azfake.Responder[armstorage.BlobServicesClientSetServicePropertiesResponse]{}, mkErr(http.StatusForbidden, "AuthorizationFailed")
			},
		}
		client := newFakeAzureBackendClient(t, &armresourcesfake.ResourceGroupsServer{}, &armstoragefake.ServerFactory{BlobServicesServer: svc})
		err := client.applyBlobDataProtection(context.Background(), "rg", "st")
		require.Error(t, err)
	})
}

func TestAzureBackendClient_ContainerExists(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		code       string
		wantExists bool
		wantErr    bool
	}{
		{name: "exists", status: http.StatusOK, wantExists: true},
		{name: "not found", status: http.StatusNotFound, code: "ContainerNotFound", wantExists: false},
		{name: "forbidden", status: http.StatusForbidden, code: "AuthorizationFailed", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cont := armstoragefake.BlobContainersServer{
				Get: func(_ context.Context, _, _, _ string, _ *armstorage.BlobContainersClientGetOptions) (azfake.Responder[armstorage.BlobContainersClientGetResponse], azfake.ErrorResponder) {
					if tt.code != "" {
						return azfake.Responder[armstorage.BlobContainersClientGetResponse]{}, mkErr(tt.status, tt.code)
					}
					return mkResp(tt.status, armstorage.BlobContainersClientGetResponse{BlobContainer: armstorage.BlobContainer{}}), azfake.ErrorResponder{}
				},
			}
			client := newFakeAzureBackendClient(t, &armresourcesfake.ResourceGroupsServer{}, &armstoragefake.ServerFactory{BlobContainersServer: cont})
			exists, err := client.containerExists(context.Background(), "rg", "st", "tfstate")
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantExists, exists)
		})
	}
}

func TestAzureBackendClient_CreateContainer(t *testing.T) {
	t.Run("success creates private container", func(t *testing.T) {
		var access armstorage.PublicAccess
		cont := armstoragefake.BlobContainersServer{
			Create: func(_ context.Context, _, _, _ string, params armstorage.BlobContainer, _ *armstorage.BlobContainersClientCreateOptions) (azfake.Responder[armstorage.BlobContainersClientCreateResponse], azfake.ErrorResponder) {
				access = *params.ContainerProperties.PublicAccess
				return mkResp(http.StatusOK, armstorage.BlobContainersClientCreateResponse{}), azfake.ErrorResponder{}
			},
		}
		client := newFakeAzureBackendClient(t, &armresourcesfake.ResourceGroupsServer{}, &armstoragefake.ServerFactory{BlobContainersServer: cont})
		err := client.createContainer(context.Background(), "rg", "st", "tfstate")
		require.NoError(t, err)
		assert.Equal(t, armstorage.PublicAccessNone, access)
	})

	t.Run("error propagates", func(t *testing.T) {
		cont := armstoragefake.BlobContainersServer{
			Create: func(_ context.Context, _, _, _ string, _ armstorage.BlobContainer, _ *armstorage.BlobContainersClientCreateOptions) (azfake.Responder[armstorage.BlobContainersClientCreateResponse], azfake.ErrorResponder) {
				return azfake.Responder[armstorage.BlobContainersClientCreateResponse]{}, mkErr(http.StatusForbidden, "AuthorizationFailed")
			},
		}
		client := newFakeAzureBackendClient(t, &armresourcesfake.ResourceGroupsServer{}, &armstoragefake.ServerFactory{BlobContainersServer: cont})
		err := client.createContainer(context.Background(), "rg", "st", "tfstate")
		require.Error(t, err)
	})
}

func TestAzureBackendClient_DeleteStorageAccount(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		acct := armstoragefake.AccountsServer{
			Delete: func(_ context.Context, _, _ string, _ *armstorage.AccountsClientDeleteOptions) (azfake.Responder[armstorage.AccountsClientDeleteResponse], azfake.ErrorResponder) {
				return mkResp(http.StatusOK, armstorage.AccountsClientDeleteResponse{}), azfake.ErrorResponder{}
			},
		}
		client := newFakeAzureBackendClient(t, &armresourcesfake.ResourceGroupsServer{}, &armstoragefake.ServerFactory{AccountsServer: acct})
		err := client.deleteStorageAccount(context.Background(), "rg", "st")
		require.NoError(t, err)
	})

	t.Run("error propagates", func(t *testing.T) {
		acct := armstoragefake.AccountsServer{
			Delete: func(_ context.Context, _, _ string, _ *armstorage.AccountsClientDeleteOptions) (azfake.Responder[armstorage.AccountsClientDeleteResponse], azfake.ErrorResponder) {
				return azfake.Responder[armstorage.AccountsClientDeleteResponse]{}, mkErr(http.StatusForbidden, "AuthorizationFailed")
			},
		}
		client := newFakeAzureBackendClient(t, &armresourcesfake.ResourceGroupsServer{}, &armstoragefake.ServerFactory{AccountsServer: acct})
		err := client.deleteStorageAccount(context.Background(), "rg", "st")
		require.Error(t, err)
	})
}

// TestAzurerm_ClientFactoryError covers the client-build failure branch shared by
// Create/Exists/Delete, exercised via the injectable factory returning an error.
func TestAzurerm_ClientFactoryError(t *testing.T) {
	SetAzureBackendClientFactory(func(string, *schema.AuthContext) (azureBackendAPI, error) {
		return nil, errors.New("cred boom")
	})
	t.Cleanup(ResetAzureBackendClientFactory)

	cfg := validAzurermBackendConfig()

	t.Run("create", func(t *testing.T) {
		_, err := CreateAzurermBackend(context.Background(), nil, cfg, nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrLoadAzureConfig)
	})
	t.Run("exists", func(t *testing.T) {
		_, err := AzurermBackendExists(context.Background(), nil, cfg, nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrLoadAzureConfig)
	})
	t.Run("delete", func(t *testing.T) {
		err := DeleteAzurermBackend(context.Background(), nil, cfg, nil, true)
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrLoadAzureConfig)
	})
}
