package backend

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage/v4"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	backendTypeAzurerm = "azurerm"
	// HTTP 404 status Azure Resource Manager returns when a resource group,
	// storage account, or container does not exist.
	azureStatusNotFound = 404
	// Number of days deleted blobs/containers stay recoverable. Blob versioning is the
	// primary S3-versioning analog; soft delete is the safety net for accidental deletes.
	// Mirrors the opinionated defaults of the S3 provisioner.
	blobSoftDeleteRetentionDays int32 = 30
)

// azureBackendAPI is a narrow, high-level interface over the Azure Resource Manager SDK
// that the azurerm backend provisioner depends on. It hides the SDK's pollers and paged
// responses behind simple methods so the provisioner logic is trivially mockable in tests —
// the same wrapper pattern used by internal/terraform_backend/terraform_backend_azurerm.go.
type azureBackendAPI interface {
	// resourceGroupExists reports whether the resource group exists and, when it does, its location.
	resourceGroupExists(ctx context.Context, resourceGroup string) (exists bool, location string, err error)
	// createResourceGroup creates (or updates) the resource group in the given location.
	createResourceGroup(ctx context.Context, resourceGroup, location string, tags map[string]*string) error
	// storageAccountExists reports whether the storage account exists in the resource group.
	storageAccountExists(ctx context.Context, resourceGroup, account string) (bool, error)
	// createStorageAccount creates a storage account with the provisioner's secure defaults.
	createStorageAccount(ctx context.Context, params azureStorageAccountParams) error
	// applyBlobDataProtection enables blob versioning + soft delete (the S3-versioning analog).
	applyBlobDataProtection(ctx context.Context, resourceGroup, account string) error
	// containerExists reports whether the container exists in the storage account.
	containerExists(ctx context.Context, resourceGroup, account, container string) (bool, error)
	// createContainer creates a private (no public access) blob container.
	createContainer(ctx context.Context, resourceGroup, account, container string) error
	// deleteStorageAccount permanently deletes the storage account and all its contents.
	deleteStorageAccount(ctx context.Context, resourceGroup, account string) error
}

// azureStorageAccountParams carries the inputs for creating a storage account.
type azureStorageAccountParams struct {
	resourceGroup string
	account       string
	location      string
	// disableSharedKey creates the account with Entra ID (AAD) data-plane auth only
	// (AllowSharedKeyAccess=false). Derived from the backend's `use_azuread_auth` so a
	// key-based backend keeps working while an AAD-based backend gets the hardened default.
	disableSharedKey bool
	tags             map[string]*string
}

// azurermConfig holds the azurerm backend configuration needed for provisioning.
type azurermConfig struct {
	resourceGroupName  string
	storageAccountName string
	containerName      string
	subscriptionID     string
	// location is the Azure region for the resource group / storage account. It is NOT a
	// valid azurerm backend attribute (Terraform would reject it in backend.tf.json), so it
	// is sourced from the active Azure identity (authContext.Azure.Location) — or, when the
	// resource group already exists, inherited from the resource group itself.
	location string
	// useAzureADAuth mirrors the backend's `use_azuread_auth` setting so the provisioner can
	// harden the storage account (disable shared keys) only when the backend authenticates
	// with Entra ID.
	useAzureADAuth bool
}

// azureBackendClientFactory builds the Azure backend API client for a subscription.
// Override in tests to inject a fake client. Protected by azureTestMu.
var azureBackendClientFactory = newAzureBackendClient

// azureTestMu protects the test-overridable client factory for concurrent test execution.
var azureTestMu sync.RWMutex

// getAzureBackendClientFactory returns the current factory with thread-safe read access.
func getAzureBackendClientFactory() func(string, *schema.AuthContext) (azureBackendAPI, error) {
	azureTestMu.RLock()
	defer azureTestMu.RUnlock()
	return azureBackendClientFactory
}

// SetAzureBackendClientFactory sets a custom Azure backend client factory for testing.
func SetAzureBackendClientFactory(f func(string, *schema.AuthContext) (azureBackendAPI, error)) {
	defer perf.Track(nil, "backend.SetAzureBackendClientFactory")()

	azureTestMu.Lock()
	defer azureTestMu.Unlock()
	azureBackendClientFactory = f
}

// ResetAzureBackendClientFactory resets the Azure backend client factory to default.
func ResetAzureBackendClientFactory() {
	defer perf.Track(nil, "backend.ResetAzureBackendClientFactory")()

	azureTestMu.Lock()
	defer azureTestMu.Unlock()
	azureBackendClientFactory = newAzureBackendClient
}

func init() {
	// Register azurerm backend create function.
	RegisterBackendCreate(backendTypeAzurerm, CreateAzurermBackend)
	// Register azurerm backend delete function.
	RegisterBackendDelete(backendTypeAzurerm, DeleteAzurermBackend)
	// Register azurerm backend exists function.
	RegisterBackendExists(backendTypeAzurerm, AzurermBackendExists)
	// Register azurerm backend name function.
	RegisterBackendName(backendTypeAzurerm, AzurermBackendName)
}

// AzurermBackendName returns the storage account name from azurerm backend config.
// The storage account is the primary provisioned artifact (the S3-bucket analog).
func AzurermBackendName(backendConfig map[string]any) string {
	defer perf.Track(nil, "backend.AzurermBackendName")()

	if account, ok := backendConfig["storage_account_name"].(string); ok && account != "" {
		return account
	}
	return ""
}

// CreateAzurermBackend creates an azurerm backend with opinionated, hardcoded defaults —
// the Azure analog of CreateS3Backend.
//
// Hardcoded features:
//   - Resource group: created if missing (in the identity's location).
//   - Storage account: StorageV2, Standard_LRS, TLS 1.2 minimum, HTTPS-only,
//     public blob access BLOCKED. Shared-key access is disabled when the backend uses
//     `use_azuread_auth: true` (Entra-ID-only), otherwise left at the Azure default.
//   - Blob data protection: versioning ENABLED (the S3-versioning analog) + 30-day soft delete.
//   - Container: created if missing, PRIVATE (no public access).
//   - Tags: Name + ManagedBy=Atmos.
//
// State locking needs NO extra resource on Azure: the azurerm backend serializes writes with
// native blob leases on the state blob (the DynamoDB-lock analog is built into Blob Storage).
//
// No configuration options beyond `provision.backend.enabled: true`.
// For production use, migrate to a managed module (e.g. Azure/avm-res-storage-storageaccount).
func CreateAzurermBackend(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	backendConfig map[string]any,
	authContext *schema.AuthContext,
) (*ProvisionResult, error) {
	defer perf.Track(atmosConfig, "backend.CreateAzurermBackend")()

	config, err := extractAzurermConfig(backendConfig, authContext)
	if err != nil {
		return nil, err
	}

	client, err := getAzureBackendClientFactory()(config.subscriptionID, authContext)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrLoadAzureConfig).
			WithCause(err).
			WithHint("Check Azure credentials are configured (run `atmos auth login`)").
			WithContext("subscription_id", config.subscriptionID).
			WithContext("storage_account", config.storageAccountName).
			Err()
	}

	// Ensure the resource group exists; its location is where the storage account is created.
	location, err := ensureResourceGroup(ctx, client, config)
	if err != nil {
		return nil, err
	}

	var warnings []string

	// Ensure the storage account exists; create with secure defaults if missing.
	accountExisted, err := ensureStorageAccount(ctx, client, config, location)
	if err != nil {
		return nil, err
	}
	if accountExisted {
		warnings = append(warnings, fmt.Sprintf("Applying Atmos blob data-protection defaults to existing storage account `%s`\n\n"+
			"- Blob versioning will be ENABLED\n"+
			"- Soft delete will be ENABLED (%d-day retention)", config.storageAccountName, blobSoftDeleteRetentionDays))
	}

	// Enable blob versioning + soft delete (idempotent — safe to re-apply).
	if err := client.applyBlobDataProtection(ctx, config.resourceGroupName, config.storageAccountName); err != nil {
		return nil, errUtils.Build(errUtils.ErrApplyBlobDataProtection).
			WithCause(err).
			WithContext("storage_account", config.storageAccountName).
			WithContext("resource_group", config.resourceGroupName).
			Err()
	}

	// Ensure the container exists.
	if err := ensureContainer(ctx, client, config); err != nil {
		return nil, err
	}

	return &ProvisionResult{Warnings: warnings}, nil
}

// AzurermBackendExists reports whether the azurerm backend is fully provisioned — i.e. both
// the storage account and the container exist. Returning false when the container is missing
// lets the (idempotent) create path finish provisioning it. Registered in the backend
// registry and called during auto-provisioning to decide whether to skip creation.
func AzurermBackendExists(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	backendConfig map[string]any,
	authContext *schema.AuthContext,
) (bool, error) {
	defer perf.Track(atmosConfig, "backend.AzurermBackendExists")()

	config, err := extractAzurermConfig(backendConfig, authContext)
	if err != nil {
		return false, err
	}

	client, err := getAzureBackendClientFactory()(config.subscriptionID, authContext)
	if err != nil {
		return false, errUtils.Build(errUtils.ErrLoadAzureConfig).
			WithCause(err).
			WithContext("subscription_id", config.subscriptionID).
			WithContext("storage_account", config.storageAccountName).
			Err()
	}

	accountExists, err := client.storageAccountExists(ctx, config.resourceGroupName, config.storageAccountName)
	if err != nil {
		return false, errUtils.Build(errUtils.ErrCheckStorageAccountExist).
			WithCause(err).
			WithContext("storage_account", config.storageAccountName).
			WithContext("resource_group", config.resourceGroupName).
			Err()
	}
	if !accountExists {
		return false, nil
	}

	containerExists, err := client.containerExists(ctx, config.resourceGroupName, config.storageAccountName, config.containerName)
	if err != nil {
		return false, errUtils.Build(errUtils.ErrCheckContainerExist).
			WithCause(err).
			WithContext("container", config.containerName).
			WithContext("storage_account", config.storageAccountName).
			Err()
	}
	return containerExists, nil
}

// extractAzurermConfig extracts and validates the azurerm backend configuration required for
// provisioning. Location is optional here — it is only required later if the resource group
// must be created (see ensureResourceGroup).
func extractAzurermConfig(backendConfig map[string]any, authContext *schema.AuthContext) (*azurermConfig, error) {
	storageAccount, ok := backendConfig["storage_account_name"].(string)
	if !ok || storageAccount == "" {
		return nil, errUtils.Build(errUtils.ErrStorageAccountRequired).
			WithHint("Set `backend.storage_account_name` in the component configuration").
			Err()
	}

	container, ok := backendConfig["container_name"].(string)
	if !ok || container == "" {
		return nil, errUtils.Build(errUtils.ErrAzureContainerRequired).
			WithHint("Set `backend.container_name` in the component configuration").
			Err()
	}

	resourceGroup, ok := backendConfig["resource_group_name"].(string)
	if !ok || resourceGroup == "" {
		return nil, errUtils.Build(errUtils.ErrResourceGroupRequired).
			WithHint("Set `backend.resource_group_name` in the component configuration").
			Err()
	}

	subscriptionID := extractAzureSubscriptionID(backendConfig, authContext)
	if subscriptionID == "" {
		return nil, errUtils.Build(errUtils.ErrAzureSubscriptionRequired).
			WithHint("Set `backend.subscription_id`, or a subscription on the active Azure identity").
			Err()
	}

	var location string
	if authContext != nil && authContext.Azure != nil {
		location = authContext.Azure.Location
	}

	return &azurermConfig{
		resourceGroupName:  resourceGroup,
		storageAccountName: storageAccount,
		containerName:      container,
		subscriptionID:     subscriptionID,
		location:           location,
		useAzureADAuth:     extractUseAzureADAuth(backendConfig),
	}, nil
}

// extractAzureSubscriptionID resolves the subscription id from the backend config
// (`subscription_id`, a valid azurerm backend attribute) or, failing that, from the active
// Azure identity's auth context.
func extractAzureSubscriptionID(backendConfig map[string]any, authContext *schema.AuthContext) string {
	if sub, ok := backendConfig["subscription_id"].(string); ok && sub != "" {
		return sub
	}
	if authContext != nil && authContext.Azure != nil {
		return authContext.Azure.SubscriptionID
	}
	return ""
}

// extractUseAzureADAuth reads the backend's `use_azuread_auth` setting, tolerating both the
// bool form and the string form ("true"/"false") that YAML/JSON may produce.
func extractUseAzureADAuth(backendConfig map[string]any) bool {
	switch v := backendConfig["use_azuread_auth"].(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(v, "true")
	default:
		return false
	}
}

// ensureResourceGroup ensures the resource group exists and returns the location the storage
// account should be created in. If the group exists, its own location is used (so no location
// needs to be configured). If it must be created, config.location (from the Azure identity)
// is required.
func ensureResourceGroup(ctx context.Context, client azureBackendAPI, config *azurermConfig) (string, error) {
	exists, location, err := client.resourceGroupExists(ctx, config.resourceGroupName)
	if err != nil {
		return "", errUtils.Build(errUtils.ErrCheckResourceGroupExist).
			WithCause(err).
			WithContext("resource_group", config.resourceGroupName).
			Err()
	}
	if exists {
		return location, nil
	}

	if config.location == "" {
		return "", errUtils.Build(errUtils.ErrAzureLocationRequired).
			WithExplanation("The resource group does not exist and no Azure location is available to create it").
			WithHint("Set a `location` on the active Azure identity, or pre-create the resource group").
			WithContext("resource_group", config.resourceGroupName).
			Err()
	}

	tags := azureBackendTags(config.storageAccountName)
	if err := client.createResourceGroup(ctx, config.resourceGroupName, config.location, tags); err != nil {
		return "", errUtils.Build(errUtils.ErrCreateResourceGroup).
			WithCause(err).
			WithContext("resource_group", config.resourceGroupName).
			WithContext("location", config.location).
			Err()
	}
	return config.location, nil
}

// ensureStorageAccount ensures the storage account exists, creating it with secure defaults
// if missing. Returns whether the account already existed (so callers can warn).
func ensureStorageAccount(ctx context.Context, client azureBackendAPI, config *azurermConfig, location string) (bool, error) {
	exists, err := client.storageAccountExists(ctx, config.resourceGroupName, config.storageAccountName)
	if err != nil {
		return false, errUtils.Build(errUtils.ErrCheckStorageAccountExist).
			WithCause(err).
			WithContext("storage_account", config.storageAccountName).
			WithContext("resource_group", config.resourceGroupName).
			Err()
	}
	if exists {
		return true, nil
	}

	params := azureStorageAccountParams{
		resourceGroup:    config.resourceGroupName,
		account:          config.storageAccountName,
		location:         location,
		disableSharedKey: config.useAzureADAuth,
		tags:             azureBackendTags(config.storageAccountName),
	}
	if err := client.createStorageAccount(ctx, params); err != nil {
		return false, errUtils.Build(errUtils.ErrCreateStorageAccount).
			WithCause(err).
			WithHint("Storage account names must be globally unique, 3-24 lowercase alphanumeric characters").
			WithContext("storage_account", config.storageAccountName).
			WithContext("resource_group", config.resourceGroupName).
			WithContext("location", location).
			Err()
	}
	return false, nil
}

// ensureContainer ensures the blob container exists, creating a private container if missing.
func ensureContainer(ctx context.Context, client azureBackendAPI, config *azurermConfig) error {
	exists, err := client.containerExists(ctx, config.resourceGroupName, config.storageAccountName, config.containerName)
	if err != nil {
		return errUtils.Build(errUtils.ErrCheckContainerExist).
			WithCause(err).
			WithContext("container", config.containerName).
			WithContext("storage_account", config.storageAccountName).
			Err()
	}
	if exists {
		return nil
	}

	if err := client.createContainer(ctx, config.resourceGroupName, config.storageAccountName, config.containerName); err != nil {
		return errUtils.Build(errUtils.ErrCreateStorageContainer).
			WithCause(err).
			WithContext("container", config.containerName).
			WithContext("storage_account", config.storageAccountName).
			Err()
	}
	return nil
}

// azureBackendTags returns the standard tag set applied to provisioned Azure backend
// resources, mirroring the S3 provisioner's Name + ManagedBy tags.
func azureBackendTags(name string) map[string]*string {
	return map[string]*string{
		"Name":      to.Ptr(name),
		"ManagedBy": to.Ptr("Atmos"),
	}
}

// isAzureNotFound reports whether an Azure Resource Manager error is a 404 (resource missing).
func isAzureNotFound(err error) bool {
	var respErr *azcore.ResponseError
	if errors.As(err, &respErr) {
		return respErr.StatusCode == azureStatusNotFound
	}
	return false
}

// azureBackendClient is the production implementation of azureBackendAPI backed by the Azure
// Resource Manager SDKs. It hides pollers behind synchronous methods.
type azureBackendClient struct {
	resourceGroups *armresources.ResourceGroupsClient
	accounts       *armstorage.AccountsClient
	blobServices   *armstorage.BlobServicesClient
	blobContainers *armstorage.BlobContainersClient
}

// newAzureBackendClient builds the Azure Resource Manager clients using DefaultAzureCredential,
// which honors the credentials seeded by `atmos auth login` (Azure CLI), environment
// variables, and Managed Identity — the same credential chain the state reader uses.
func newAzureBackendClient(subscriptionID string, _ *schema.AuthContext) (azureBackendAPI, error) {
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrCreateAzureCredential, err)
	}

	resourceGroups, err := armresources.NewResourceGroupsClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrCreateAzureClient, err)
	}

	factory, err := armstorage.NewClientFactory(subscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrCreateAzureClient, err)
	}

	return &azureBackendClient{
		resourceGroups: resourceGroups,
		accounts:       factory.NewAccountsClient(),
		blobServices:   factory.NewBlobServicesClient(),
		blobContainers: factory.NewBlobContainersClient(),
	}, nil
}

func (c *azureBackendClient) resourceGroupExists(ctx context.Context, resourceGroup string) (bool, string, error) {
	resp, err := c.resourceGroups.Get(ctx, resourceGroup, nil)
	if err != nil {
		if isAzureNotFound(err) {
			return false, "", nil
		}
		return false, "", err
	}
	var location string
	if resp.Location != nil {
		location = *resp.Location
	}
	return true, location, nil
}

func (c *azureBackendClient) createResourceGroup(ctx context.Context, resourceGroup, location string, tags map[string]*string) error {
	_, err := c.resourceGroups.CreateOrUpdate(ctx, resourceGroup, armresources.ResourceGroup{
		Location: to.Ptr(location),
		Tags:     tags,
	}, nil)
	return err
}

func (c *azureBackendClient) storageAccountExists(ctx context.Context, resourceGroup, account string) (bool, error) {
	_, err := c.accounts.GetProperties(ctx, resourceGroup, account, nil)
	if err != nil {
		if isAzureNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (c *azureBackendClient) createStorageAccount(ctx context.Context, params azureStorageAccountParams) error {
	props := &armstorage.AccountPropertiesCreateParameters{
		MinimumTLSVersion:      to.Ptr(armstorage.MinimumTLSVersionTLS12),
		EnableHTTPSTrafficOnly: to.Ptr(true),
		AllowBlobPublicAccess:  to.Ptr(false),
	}
	if params.disableSharedKey {
		props.AllowSharedKeyAccess = to.Ptr(false)
	}

	poller, err := c.accounts.BeginCreate(ctx, params.resourceGroup, params.account, armstorage.AccountCreateParameters{
		Kind:       to.Ptr(armstorage.KindStorageV2),
		Location:   to.Ptr(params.location),
		SKU:        &armstorage.SKU{Name: to.Ptr(armstorage.SKUNameStandardLRS)},
		Properties: props,
		Tags:       params.tags,
	}, nil)
	if err != nil {
		return err
	}
	_, err = poller.PollUntilDone(ctx, nil)
	return err
}

func (c *azureBackendClient) applyBlobDataProtection(ctx context.Context, resourceGroup, account string) error {
	// Read current properties first so we never REDUCE an existing, possibly longer, soft-delete
	// retention. This matters when `provision.backend.enabled` is left on after the account is
	// adopted into a production module that sets a longer retention: SetServiceProperties preserves
	// omitted property groups, but it would overwrite the retention we do send, so we raise the
	// value to at least the default floor rather than clamping it down.
	current, err := c.blobServices.GetServiceProperties(ctx, resourceGroup, account, nil)
	if err != nil {
		return err
	}

	blobDays, containerDays := blobSoftDeleteRetentionDays, blobSoftDeleteRetentionDays
	if props := current.BlobServiceProperties.BlobServiceProperties; props != nil {
		if p := props.DeleteRetentionPolicy; p != nil && p.Days != nil && *p.Days > blobDays {
			blobDays = *p.Days
		}
		if p := props.ContainerDeleteRetentionPolicy; p != nil && p.Days != nil && *p.Days > containerDays {
			containerDays = *p.Days
		}
	}

	_, err = c.blobServices.SetServiceProperties(ctx, resourceGroup, account, armstorage.BlobServiceProperties{
		BlobServiceProperties: &armstorage.BlobServicePropertiesProperties{
			IsVersioningEnabled: to.Ptr(true),
			DeleteRetentionPolicy: &armstorage.DeleteRetentionPolicy{
				Enabled: to.Ptr(true),
				Days:    to.Ptr(blobDays),
			},
			ContainerDeleteRetentionPolicy: &armstorage.DeleteRetentionPolicy{
				Enabled: to.Ptr(true),
				Days:    to.Ptr(containerDays),
			},
		},
	}, nil)
	return err
}

func (c *azureBackendClient) containerExists(ctx context.Context, resourceGroup, account, container string) (bool, error) {
	_, err := c.blobContainers.Get(ctx, resourceGroup, account, container, nil)
	if err != nil {
		if isAzureNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (c *azureBackendClient) createContainer(ctx context.Context, resourceGroup, account, container string) error {
	_, err := c.blobContainers.Create(ctx, resourceGroup, account, container, armstorage.BlobContainer{
		ContainerProperties: &armstorage.ContainerProperties{
			PublicAccess: to.Ptr(armstorage.PublicAccessNone),
		},
	}, nil)
	return err
}

func (c *azureBackendClient) deleteStorageAccount(ctx context.Context, resourceGroup, account string) error {
	_, err := c.accounts.Delete(ctx, resourceGroup, account, nil)
	return err
}
