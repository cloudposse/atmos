package backend

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// DeleteAzurermBackend deletes an azurerm backend by permanently deleting the storage account
// and everything in it — the Azure analog of DeleteS3Backend deleting an S3 bucket.
//
// The storage account is the backend's top-level provisioned artifact and holds the container
// and every state blob (one per component `key`). Deleting it removes them all, exactly as
// deleting an S3 bucket removes every state object. The resource group is intentionally left
// in place, since it commonly holds unrelated resources.
//
// This operation is irreversible. All Terraform state stored in the account will be lost.
func DeleteAzurermBackend(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	backendConfig map[string]any,
	authContext *schema.AuthContext,
	force bool,
) error {
	defer perf.Track(atmosConfig, "backend.DeleteAzurermBackend")()

	if !force {
		return errForceRequired()
	}

	config, err := extractAzurermConfig(backendConfig, authContext)
	if err != nil {
		return err
	}

	ui.Info(fmt.Sprintf("Deleting azurerm backend: storage_account=%s resource_group=%s",
		config.storageAccountName, config.resourceGroupName))

	client, err := getAzureBackendClientFactory()(config.subscriptionID, authContext)
	if err != nil {
		return errUtils.Build(errUtils.ErrLoadAzureConfig).
			WithCause(err).
			WithExplanation("Failed to build Azure client for backend deletion").
			WithContext("subscription_id", config.subscriptionID).
			WithContext("storage_account", config.storageAccountName).
			Err()
	}

	if err := validateStorageAccountExistsForDeletion(ctx, client, config); err != nil {
		return err
	}

	showAzureDeletionWarning(config.storageAccountName, config.containerName)

	if err := client.deleteStorageAccount(ctx, config.resourceGroupName, config.storageAccountName); err != nil {
		return errUtils.Build(errUtils.ErrDeleteStorageAccount).
			WithCause(err).
			WithContext("storage_account", config.storageAccountName).
			WithContext("resource_group", config.resourceGroupName).
			WithHint("Check RBAC permissions (Storage Account Contributor) on the storage account").
			Err()
	}

	ui.Successf("Backend deleted: storage account '%s' and all contents removed", config.storageAccountName)
	return nil
}

// validateStorageAccountExistsForDeletion checks the storage account exists before deletion.
func validateStorageAccountExistsForDeletion(ctx context.Context, client azureBackendAPI, config *azurermConfig) error {
	exists, err := client.storageAccountExists(ctx, config.resourceGroupName, config.storageAccountName)
	if err != nil {
		return errUtils.Build(errUtils.ErrCheckStorageAccountExist).
			WithCause(err).
			WithContext("storage_account", config.storageAccountName).
			WithContext("resource_group", config.resourceGroupName).
			Err()
	}
	if !exists {
		return errUtils.Build(errUtils.ErrBackendNotFound).
			WithExplanation("Cannot delete backend - storage account does not exist").
			WithContext("storage_account", config.storageAccountName).
			WithContext("resource_group", config.resourceGroupName).
			WithHint("Verify the storage account name in your backend configuration").
			Err()
	}
	return nil
}

// showAzureDeletionWarning displays a warning about the pending destructive deletion.
func showAzureDeletionWarning(account, container string) {
	ui.Warning(fmt.Sprintf("⚠ Deleting backend will permanently remove storage account '%s' "+
		"and all Terraform state in container '%s'", account, container))
	ui.Warning("This action cannot be undone")
}
