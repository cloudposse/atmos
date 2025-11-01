package terraform_backend

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// MaxRetryCountAzure defines the max attempts to read a state file from Azure Blob Storage.
	maxRetryCountAzure = 2
	// StatusCodeNotFoundAzure represents the HTTP 404 status code.
	statusCodeNotFoundAzure = 404
	// StatusCodeForbiddenAzure represents the HTTP 403 status code.
	statusCodeForbiddenAzure = 403
	// Error format for wrapping errors with context.
	errWrapFormat = "%w: %w"
)

//go:generate mockgen -source=$GOFILE -destination=mock_$GOFILE -package=$GOPACKAGE

// AzureBlobAPI defines an interface for interacting with Azure Blob Storage.
type AzureBlobAPI interface {
	DownloadStream(
		ctx context.Context,
		containerName string,
		blobName string,
		options *azblob.DownloadStreamOptions,
	) (AzureBlobDownloadResponse, error)
}

// AzureBlobDownloadResponse defines the response from a blob download operation.
type AzureBlobDownloadResponse interface {
	GetBody() io.ReadCloser
}

// azureBlobDownloadResponseWrapper wraps the actual Azure SDK response.
type azureBlobDownloadResponseWrapper struct {
	response azblob.DownloadStreamResponse
}

func (w *azureBlobDownloadResponseWrapper) GetBody() io.ReadCloser {
	defer perf.Track(nil, "terraform_backend.azureBlobDownloadResponseWrapper.GetBody")()

	return w.response.Body
}

// azureBlobClientWrapper wraps the actual Azure SDK client to implement AzureBlobAPI.
type azureBlobClientWrapper struct {
	client *azblob.Client
}

func (w *azureBlobClientWrapper) DownloadStream(
	ctx context.Context,
	containerName string,
	blobName string,
	options *azblob.DownloadStreamOptions,
) (AzureBlobDownloadResponse, error) {
	defer perf.Track(nil, "terraform_backend.azureBlobClientWrapper.DownloadStream")()

	response, err := w.client.DownloadStream(ctx, containerName, blobName, options)
	if err != nil {
		return nil, err
	}
	return &azureBlobDownloadResponseWrapper{response: response}, nil
}

// azureBlobClientCache caches the Azure Blob Storage clients based on a deterministic cache key.
var azureBlobClientCache sync.Map

func getCachedAzureBlobClient(backend *map[string]any) (AzureBlobAPI, error) {
	defer perf.Track(nil, "terraform_backend.getCachedAzureBlobClient")()

	storageAccountName := GetBackendAttribute(backend, "storage_account_name")

	// Build the Azure Blob client if not cached.
	if storageAccountName == "" {
		return nil, errUtils.ErrStorageAccountRequired
	}

	// Cache by storage account only (client can access any container in the account).
	cacheKey := storageAccountName

	// Check the cache.
	if cached, ok := azureBlobClientCache.Load(cacheKey); ok {
		return cached.(AzureBlobAPI), nil
	}

	// Construct the blob service URL.
	serviceURL := fmt.Sprintf("https://%s.blob.core.windows.net/", storageAccountName)

	// Use DefaultAzureCredential for authentication.
	// This supports multiple authentication methods:
	// 1. Environment variables (AZURE_TENANT_ID, AZURE_CLIENT_ID, AZURE_CLIENT_SECRET)
	// 2. Managed Identity (when running in Azure)
	// 3. Azure CLI credentials
	// 4. Visual Studio Code credentials
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf(errWrapFormat, errUtils.ErrCreateAzureCredential, err)
	}

	// Configure client options with telemetry.
	clientOptions := &azblob.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Telemetry: policy.TelemetryOptions{
				ApplicationID: "atmos",
			},
		},
	}

	client, err := azblob.NewClient(serviceURL, cred, clientOptions)
	if err != nil {
		return nil, fmt.Errorf(errWrapFormat, errUtils.ErrCreateAzureClient, err)
	}

	wrappedClient := &azureBlobClientWrapper{client: client}
	azureBlobClientCache.Store(cacheKey, wrappedClient)
	return wrappedClient, nil
}

// ReadTerraformBackendAzurerm reads the Terraform state file from the configured Azure Blob Storage backend.
// If the state file does not exist in the container, the function returns `nil`.
func ReadTerraformBackendAzurerm(
	atmosConfig *schema.AtmosConfiguration,
	componentSections *map[string]any,
	authContext *schema.AuthContext,
) ([]byte, error) {
	defer perf.Track(atmosConfig, "terraform_backend.ReadTerraformBackendAzurerm")()

	backend := GetComponentBackend(componentSections)
	if backend == nil {
		return nil, errUtils.ErrBackendConfigRequired
	}

	azureClient, err := getCachedAzureBlobClient(&backend)
	if err != nil {
		return nil, err
	}

	return ReadTerraformBackendAzurermInternal(azureClient, componentSections, &backend)
}

// constructAzureBlobPath constructs the blob path based on workspace and key.
func constructAzureBlobPath(componentSections *map[string]any, backend *map[string]any) string {
	defer perf.Track(nil, "terraform_backend.constructAzureBlobPath")()

	key := GetBackendAttribute(backend, "key")
	if key == "" {
		key = "terraform.tfstate"
	}

	workspace := GetTerraformWorkspace(componentSections)

	if workspace != "" && workspace != "default" {
		// Non-default workspace: Azure appends the workspace as a suffix to the key.
		// Format: {key}env:{workspace}
		// Example: apimanagement.terraform.tfstateenv:dev-wus3-apimanagement-be
		return fmt.Sprintf("%senv:%s", key, workspace)
	}
	// Default workspace: use key as-is.
	return key
}

// handleAzureDownloadError handles errors from Azure Blob Storage download operations.
func handleAzureDownloadError(err error, tfStateFilePath, containerName string) error {
	defer perf.Track(nil, "terraform_backend.handleAzureDownloadError")()

	var respErr *azcore.ResponseError
	if errors.As(err, &respErr) {
		switch respErr.StatusCode {
		case statusCodeNotFoundAzure:
			log.Debug(
				"Terraform state file doesn't exist in Azure Blob Storage; returning 'null'",
				"file", tfStateFilePath,
				"container", containerName,
			)
			return nil
		case statusCodeForbiddenAzure:
			return fmt.Errorf(
				"%w: blob '%s' in container '%s': %v",
				errUtils.ErrAzurePermissionDenied,
				tfStateFilePath,
				containerName,
				err,
			)
		}
	}
	return err
}

// ReadTerraformBackendAzurermInternal accepts an Azure Blob client and reads the Terraform state file from the configured Azure Blob Storage backend.
func ReadTerraformBackendAzurermInternal(
	azureClient AzureBlobAPI,
	componentSections *map[string]any,
	backend *map[string]any,
) ([]byte, error) {
	defer perf.Track(nil, "terraform_backend.ReadTerraformBackendAzurermInternal")()

	// Get backend configuration attributes.
	containerName := GetBackendAttribute(backend, "container_name")
	if containerName == "" {
		return nil, errUtils.ErrAzureContainerRequired
	}

	tfStateFilePath := constructAzureBlobPath(componentSections, backend)

	var lastErr error
	for attempt := 0; attempt <= maxRetryCountAzure; attempt++ {
		// 30 sec timeout to read the state file from Azure Blob Storage.
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

		downloadResponse, err := azureClient.DownloadStream(ctx, containerName, tfStateFilePath, nil)
		if err != nil {
			handledErr := handleAzureDownloadError(err, tfStateFilePath, containerName)
			if handledErr == nil {
				// Blob not found - component not provisioned yet.
				cancel()
				return nil, nil
			}
			if errors.Is(handledErr, errUtils.ErrAzurePermissionDenied) {
				// Permission denied - return immediately.
				cancel()
				return nil, handledErr
			}

			lastErr = err
			if attempt < maxRetryCountAzure {
				// Exponential backoff: 1s, 2s, 4s for attempts 0, 1, 2.
				backoff := time.Second * time.Duration(1<<attempt)
				log.Debug(
					"Failed to read Terraform state file from Azure Blob Storage",
					"attempt", attempt+1,
					"file", tfStateFilePath,
					"container", containerName,
					"error", err,
					"backoff", backoff,
				)
				cancel()
				time.Sleep(backoff)
				continue
			}
			cancel()
			return nil, fmt.Errorf(errWrapFormat, errUtils.ErrGetBlobFromAzure, lastErr)
		}

		body := downloadResponse.GetBody()
		content, err := io.ReadAll(body)
		_ = body.Close()
		if err != nil {
			cancel()
			return nil, fmt.Errorf(errWrapFormat, errUtils.ErrReadAzureBlobBody, err)
		}
		cancel()
		return content, nil
	}

	return nil, fmt.Errorf(errWrapFormat, errUtils.ErrGetBlobFromAzure, lastErr)
}
