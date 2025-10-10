package terraform_backend

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// maxRetryCountAzure defines the max attempts to read a state file from Azure Blob Storage.
	maxRetryCountAzure = 2
	// statusCodeNotFoundAzure represents the HTTP 404 status code.
	statusCodeNotFoundAzure = 404
	// statusCodeForbiddenAzure represents the HTTP 403 status code.
	statusCodeForbiddenAzure = 403
)

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
	containerName := GetBackendAttribute(backend, "container_name")

	// Build a deterministic cache key.
	cacheKey := fmt.Sprintf("account=%s;container=%s", storageAccountName, containerName)

	// Check the cache.
	if cached, ok := azureBlobClientCache.Load(cacheKey); ok {
		return cached.(AzureBlobAPI), nil
	}

	// Build the Azure Blob client if not cached.
	if storageAccountName == "" {
		return nil, errUtils.ErrStorageAccountRequired
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
		return nil, fmt.Errorf("%w: %v", errUtils.ErrCreateAzureCredential, err)
	}

	client, err := azblob.NewClient(serviceURL, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errUtils.ErrCreateAzureClient, err)
	}

	wrappedClient := &azureBlobClientWrapper{client: client}
	azureBlobClientCache.Store(cacheKey, wrappedClient)
	return wrappedClient, nil
}

// ReadTerraformBackendAzurerm reads the Terraform state file from the configured Azure Blob Storage backend.
// If the state file does not exist in the container, the function returns `nil`.
func ReadTerraformBackendAzurerm(
	_ *schema.AtmosConfiguration,
	componentSections *map[string]any,
) ([]byte, error) {
	defer perf.Track(nil, "terraform_backend.ReadTerraformBackendAzurerm")()

	backend := GetComponentBackend(componentSections)

	azureClient, err := getCachedAzureBlobClient(&backend)
	if err != nil {
		return nil, err
	}

	return ReadTerraformBackendAzurermInternal(azureClient, componentSections, &backend)
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

	// Construct blob path (Azure uses workspace in blob path).
	// Azure backend: for non-default workspaces, the key becomes env:/{workspace}/{key}.
	// For default workspace, use key as-is.
	key := GetBackendAttribute(backend, "key")
	if key == "" {
		key = "terraform.tfstate"
	}

	workspace := GetTerraformWorkspace(componentSections)
	var tfStateFilePath string

	// Azure Blob paths always use forward slashes, so path.Join is appropriate here.
	//nolint:forbidigo // Azure Blob paths require forward slashes regardless of OS
	if workspace != "" && workspace != "default" {
		// Non-default workspace: key is modified to include workspace.
		// Format: env:/{workspace}/{key}
		tfStateFilePath = path.Join("env:", workspace, key)
	} else {
		// Default workspace: use key as-is.
		tfStateFilePath = key
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetryCountAzure; attempt++ {
		// 30 sec timeout to read the state file from Azure Blob Storage.
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		downloadResponse, err := azureClient.DownloadStream(ctx, containerName, tfStateFilePath, nil)
		if err != nil {
			// Check if the error is because the blob doesn't exist.
			// If the state file does not exist (the component in the stack has not been provisioned yet), return a `nil` result and no error.
			var respErr *azcore.ResponseError
			if errors.As(err, &respErr) {
				switch respErr.StatusCode {
				case statusCodeNotFoundAzure:
					log.Debug(
						"Terraform state file doesn't exist in Azure Blob Storage; returning 'null'",
						"file", tfStateFilePath,
						"container", containerName,
					)
					return nil, nil
				case statusCodeForbiddenAzure:
					return nil, fmt.Errorf(
						"%w: blob '%s' in container '%s': %v",
						errUtils.ErrAzurePermissionDenied,
						tfStateFilePath,
						containerName,
						err,
					)
				}
			}

			lastErr = err
			if attempt < maxRetryCountAzure {
				log.Debug(
					"Failed to read Terraform state file from Azure Blob Storage",
					"attempt", attempt+1,
					"file", tfStateFilePath,
					"container", containerName,
					"error", err,
				)
				time.Sleep(time.Second * 2) // backoff
				continue
			}
			return nil, fmt.Errorf("%w: %v", errUtils.ErrGetBlobFromAzure, lastErr)
		}

		body := downloadResponse.GetBody()
		content, err := io.ReadAll(body)
		_ = body.Close()
		if err != nil {
			return nil, fmt.Errorf("%w: %v", errUtils.ErrReadAzureBlobBody, err)
		}
		return content, nil
	}

	return nil, fmt.Errorf("%w: %v", errUtils.ErrGetBlobFromAzure, lastErr)
}
