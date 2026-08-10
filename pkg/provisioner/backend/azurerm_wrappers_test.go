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
	tests := []struct {
		name       string
		status     int
		code       string // non-empty → error response
		location   string
		wantExists bool
		wantLoc    string
		wantErr    bool
	}{
		{name: "exists returns location", status: http.StatusOK, location: "eastus", wantExists: true, wantLoc: "eastus"},
		{name: "not found returns false", status: http.StatusNotFound, code: "ResourceGroupNotFound", wantExists: false},
		{name: "other error propagates", status: http.StatusForbidden, code: "AuthorizationFailed", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rg := &armresourcesfake.ResourceGroupsServer{
				Get: func(_ context.Context, _ string, _ *armresources.ResourceGroupsClientGetOptions) (azfake.Responder[armresources.ResourceGroupsClientGetResponse], azfake.ErrorResponder) {
					if tt.code != "" {
						return azfake.Responder[armresources.ResourceGroupsClientGetResponse]{}, mkErr(tt.status, tt.code)
					}
					return mkResp(tt.status, armresources.ResourceGroupsClientGetResponse{
						ResourceGroup: armresources.ResourceGroup{Location: to.Ptr(tt.location)},
					}), azfake.ErrorResponder{}
				},
			}
			client := newFakeAzureBackendClient(t, rg, &armstoragefake.ServerFactory{})
			exists, loc, err := client.resourceGroupExists(context.Background(), "rg")
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantExists, exists)
			assert.Equal(t, tt.wantLoc, loc)
		})
	}
}

func TestAzureBackendClient_CreateResourceGroup(t *testing.T) {
	tests := []struct {
		name    string
		fail    bool
		wantErr bool
	}{
		{name: "success"},
		{name: "error propagates", fail: true, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got armresources.ResourceGroup
			rg := &armresourcesfake.ResourceGroupsServer{
				CreateOrUpdate: func(_ context.Context, _ string, params armresources.ResourceGroup, _ *armresources.ResourceGroupsClientCreateOrUpdateOptions) (azfake.Responder[armresources.ResourceGroupsClientCreateOrUpdateResponse], azfake.ErrorResponder) {
					got = params
					if tt.fail {
						return azfake.Responder[armresources.ResourceGroupsClientCreateOrUpdateResponse]{}, mkErr(http.StatusForbidden, "AuthorizationFailed")
					}
					return mkResp(http.StatusOK, armresources.ResourceGroupsClientCreateOrUpdateResponse{}), azfake.ErrorResponder{}
				},
			}
			client := newFakeAzureBackendClient(t, rg, &armstoragefake.ServerFactory{})
			err := client.createResourceGroup(context.Background(), "rg", "centralus", azureBackendTags("st"))
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "centralus", *got.Location)
			assertAzureBackendTags(t, got.Tags, "st")
		})
	}
}

// assertAzureBackendTags asserts the standard Atmos tag set (Name + ManagedBy=Atmos).
func assertAzureBackendTags(t *testing.T, tags map[string]*string, name string) {
	t.Helper()
	require.NotNil(t, tags["Name"])
	require.NotNil(t, tags["ManagedBy"])
	assert.Equal(t, name, *tags["Name"])
	assert.Equal(t, "Atmos", *tags["ManagedBy"])
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
	tests := []struct {
		name             string
		disableSharedKey bool
		fail             bool
		wantErr          bool
		wantSharedKeyNil bool // expect AllowSharedKeyAccess left unset (Azure default)
	}{
		{name: "hardened (use_azuread_auth) disables shared key", disableSharedKey: true, wantSharedKeyNil: false},
		{name: "default leaves shared key unset", disableSharedKey: false, wantSharedKeyNil: true},
		{name: "error propagates", fail: true, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got armstorage.AccountCreateParameters
			acct := armstoragefake.AccountsServer{
				BeginCreate: func(_ context.Context, _, _ string, params armstorage.AccountCreateParameters, _ *armstorage.AccountsClientBeginCreateOptions) (azfake.PollerResponder[armstorage.AccountsClientCreateResponse], azfake.ErrorResponder) {
					got = params
					if tt.fail {
						return azfake.PollerResponder[armstorage.AccountsClientCreateResponse]{}, mkErr(http.StatusForbidden, "AuthorizationFailed")
					}
					var resp azfake.PollerResponder[armstorage.AccountsClientCreateResponse]
					resp.SetTerminalResponse(http.StatusOK, armstorage.AccountsClientCreateResponse{Account: armstorage.Account{}}, nil)
					return resp, azfake.ErrorResponder{}
				},
			}
			client := newFakeAzureBackendClient(t, &armresourcesfake.ResourceGroupsServer{}, &armstoragefake.ServerFactory{AccountsServer: acct})
			err := client.createStorageAccount(context.Background(), azureStorageAccountParams{
				resourceGroup: "rg", account: "st", location: "eastus", disableSharedKey: tt.disableSharedKey, tags: azureBackendTags("st"),
			})
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			// Full secure-default contract — a regression that drops any of these must fail.
			assert.Equal(t, armstorage.KindStorageV2, *got.Kind)
			assert.Equal(t, armstorage.SKUNameStandardLRS, *got.SKU.Name)
			assert.Equal(t, "eastus", *got.Location)
			assert.Equal(t, armstorage.MinimumTLSVersionTLS12, *got.Properties.MinimumTLSVersion)
			assert.True(t, *got.Properties.EnableHTTPSTrafficOnly, "HTTPS-only must be enabled")
			assert.False(t, *got.Properties.AllowBlobPublicAccess, "public blob access must be blocked")
			assertAzureBackendTags(t, got.Tags, "st")

			if tt.wantSharedKeyNil {
				assert.Nil(t, got.Properties.AllowSharedKeyAccess, "AllowSharedKeyAccess stays at the Azure default")
			} else {
				require.NotNil(t, got.Properties.AllowSharedKeyAccess)
				assert.False(t, *got.Properties.AllowSharedKeyAccess, "disableSharedKey=true → AllowSharedKeyAccess=false")
			}
		})
	}
}

func TestAzureBackendClient_ApplyBlobDataProtection(t *testing.T) {
	tests := []struct {
		name    string
		fail    bool
		wantErr bool
	}{
		{name: "enables versioning + soft delete"},
		{name: "error propagates", fail: true, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got armstorage.BlobServiceProperties
			svc := armstoragefake.BlobServicesServer{
				SetServiceProperties: func(_ context.Context, _, _ string, params armstorage.BlobServiceProperties, _ *armstorage.BlobServicesClientSetServicePropertiesOptions) (azfake.Responder[armstorage.BlobServicesClientSetServicePropertiesResponse], azfake.ErrorResponder) {
					got = params
					if tt.fail {
						return azfake.Responder[armstorage.BlobServicesClientSetServicePropertiesResponse]{}, mkErr(http.StatusForbidden, "AuthorizationFailed")
					}
					return mkResp(http.StatusOK, armstorage.BlobServicesClientSetServicePropertiesResponse{}), azfake.ErrorResponder{}
				},
			}
			client := newFakeAzureBackendClient(t, &armresourcesfake.ResourceGroupsServer{}, &armstoragefake.ServerFactory{BlobServicesServer: svc})
			err := client.applyBlobDataProtection(context.Background(), "rg", "st")
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			props := got.BlobServiceProperties
			require.NotNil(t, props)
			assert.True(t, *props.IsVersioningEnabled, "blob versioning must be enabled")
			require.NotNil(t, props.DeleteRetentionPolicy)
			assert.True(t, *props.DeleteRetentionPolicy.Enabled, "blob soft delete must be enabled")
			assert.Equal(t, blobSoftDeleteRetentionDays, *props.DeleteRetentionPolicy.Days)
			require.NotNil(t, props.ContainerDeleteRetentionPolicy)
			assert.True(t, *props.ContainerDeleteRetentionPolicy.Enabled, "container soft delete must be enabled")
			assert.Equal(t, blobSoftDeleteRetentionDays, *props.ContainerDeleteRetentionPolicy.Days)
		})
	}
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
	tests := []struct {
		name    string
		fail    bool
		wantErr bool
	}{
		{name: "creates private container"},
		{name: "error propagates", fail: true, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got armstorage.BlobContainer
			cont := armstoragefake.BlobContainersServer{
				Create: func(_ context.Context, _, _, _ string, params armstorage.BlobContainer, _ *armstorage.BlobContainersClientCreateOptions) (azfake.Responder[armstorage.BlobContainersClientCreateResponse], azfake.ErrorResponder) {
					got = params
					if tt.fail {
						return azfake.Responder[armstorage.BlobContainersClientCreateResponse]{}, mkErr(http.StatusForbidden, "AuthorizationFailed")
					}
					return mkResp(http.StatusOK, armstorage.BlobContainersClientCreateResponse{}), azfake.ErrorResponder{}
				},
			}
			client := newFakeAzureBackendClient(t, &armresourcesfake.ResourceGroupsServer{}, &armstoragefake.ServerFactory{BlobContainersServer: cont})
			err := client.createContainer(context.Background(), "rg", "st", "tfstate")
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, got.ContainerProperties)
			assert.Equal(t, armstorage.PublicAccessNone, *got.ContainerProperties.PublicAccess, "container must be private")
		})
	}
}

func TestAzureBackendClient_DeleteStorageAccount(t *testing.T) {
	tests := []struct {
		name    string
		fail    bool
		wantErr bool
	}{
		{name: "success"},
		{name: "error propagates", fail: true, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			acct := armstoragefake.AccountsServer{
				Delete: func(_ context.Context, _, _ string, _ *armstorage.AccountsClientDeleteOptions) (azfake.Responder[armstorage.AccountsClientDeleteResponse], azfake.ErrorResponder) {
					if tt.fail {
						return azfake.Responder[armstorage.AccountsClientDeleteResponse]{}, mkErr(http.StatusForbidden, "AuthorizationFailed")
					}
					return mkResp(http.StatusOK, armstorage.AccountsClientDeleteResponse{}), azfake.ErrorResponder{}
				},
			}
			client := newFakeAzureBackendClient(t, &armresourcesfake.ResourceGroupsServer{}, &armstoragefake.ServerFactory{AccountsServer: acct})
			err := client.deleteStorageAccount(context.Background(), "rg", "st")
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
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
