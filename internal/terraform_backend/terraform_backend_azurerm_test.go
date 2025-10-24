package terraform_backend

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/tests"
)

// mockAzureBlobClient implements AzureBlobAPI for testing.
type mockAzureBlobClient struct {
	downloadStreamFunc func(ctx context.Context, containerName string, blobName string, options *azblob.DownloadStreamOptions) (AzureBlobDownloadResponse, error)
}

func (m *mockAzureBlobClient) DownloadStream(ctx context.Context, containerName string, blobName string, options *azblob.DownloadStreamOptions) (AzureBlobDownloadResponse, error) {
	return m.downloadStreamFunc(ctx, containerName, blobName, options)
}

// mockDownloadResponse implements AzureBlobDownloadResponse for testing.
type mockDownloadResponse struct {
	body io.ReadCloser
}

func (m *mockDownloadResponse) GetBody() io.ReadCloser {
	return m.body
}

// createMockDownloadResponse creates a mock download response with the given body content.
func createMockDownloadResponse(body string) AzureBlobDownloadResponse {
	return &mockDownloadResponse{
		body: io.NopCloser(strings.NewReader(body)),
	}
}

func TestReadTerraformBackendAzurermInternal_Success(t *testing.T) {
	tests := []struct {
		name              string
		componentSections map[string]any
		backend           map[string]any
		mockResponse      string
		expectedBlobName  string
		description       string
	}{
		{
			name: "successful_read_default_workspace",
			componentSections: map[string]any{
				"component": "test-component",
				"workspace": "default",
			},
			backend: map[string]any{
				"storage_account_name": "testaccount",
				"container_name":       "tfstate",
				"key":                  "terraform.tfstate",
			},
			mockResponse:     `{"version": 4, "outputs": {"test": {"value": "test-value"}}}`,
			expectedBlobName: "terraform.tfstate",
			description:      "Default workspace uses key as-is.",
		},
		{
			name: "successful_read_dev_workspace",
			componentSections: map[string]any{
				"component": "vpc",
				"workspace": "dev",
			},
			backend: map[string]any{
				"storage_account_name": "testaccount",
				"container_name":       "tfstate",
				"key":                  "terraform.tfstate",
			},
			mockResponse:     `{"version": 4, "outputs": {"vpc_id": {"value": "vpc-123"}}}`,
			expectedBlobName: "terraform.tfstateenv:dev",
			description:      "Non-default workspace uses {key}env:{workspace} format.",
		},
		{
			name: "successful_read_prod_workspace",
			componentSections: map[string]any{
				"component": "database",
				"workspace": "prod",
			},
			backend: map[string]any{
				"storage_account_name": "prodaccount",
				"container_name":       "prod-tfstate",
				"key":                  "prod.tfstate",
			},
			mockResponse:     `{"version": 4, "outputs": {"endpoint": {"value": "prod-db.example.com"}}}`,
			expectedBlobName: "prod.tfstateenv:prod",
			description:      "Production workspace with custom key.",
		},
		{
			name: "successful_read_empty_workspace",
			componentSections: map[string]any{
				"component": "test",
				"workspace": "",
			},
			backend: map[string]any{
				"storage_account_name": "testaccount",
				"container_name":       "tfstate",
				"key":                  "terraform.tfstate",
			},
			mockResponse:     `{"version": 4, "outputs": {}}`,
			expectedBlobName: "terraform.tfstate",
			description:      "Empty workspace treated as default.",
		},
		{
			name: "successful_read_default_key",
			componentSections: map[string]any{
				"component": "app",
				"workspace": "staging",
			},
			backend: map[string]any{
				"storage_account_name": "testaccount",
				"container_name":       "tfstate",
				// key not specified, should default to terraform.tfstate.
			},
			mockResponse:     `{"version": 4, "outputs": {"app_url": {"value": "https://staging.example.com"}}}`,
			expectedBlobName: "terraform.tfstateenv:staging",
			description:      "Missing key defaults to terraform.tfstate.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &mockAzureBlobClient{
				downloadStreamFunc: func(ctx context.Context, containerName string, blobName string, options *azblob.DownloadStreamOptions) (AzureBlobDownloadResponse, error) {
					// Verify blob name matches expected pattern.
					assert.Equal(t, tt.expectedBlobName, blobName, tt.description)
					assert.Equal(t, tt.backend["container_name"], containerName)

					return createMockDownloadResponse(tt.mockResponse), nil
				},
			}

			result, err := ReadTerraformBackendAzurermInternal(mockClient, &tt.componentSections, &tt.backend)

			require.NoError(t, err)
			assert.NotNil(t, result)
			assert.Equal(t, tt.mockResponse, string(result))
		})
	}
}

func TestReadTerraformBackendAzurermInternal_BlobNotFound(t *testing.T) {
	tests := []struct {
		name              string
		componentSections map[string]any
		backend           map[string]any
	}{
		{
			name: "blob_not_found_default_workspace",
			componentSections: map[string]any{
				"component": "test-component",
				"workspace": "default",
			},
			backend: map[string]any{
				"storage_account_name": "testaccount",
				"container_name":       "tfstate",
				"key":                  "terraform.tfstate",
			},
		},
		{
			name: "blob_not_found_dev_workspace",
			componentSections: map[string]any{
				"component": "vpc",
				"workspace": "dev",
			},
			backend: map[string]any{
				"storage_account_name": "testaccount",
				"container_name":       "tfstate",
				"key":                  "terraform.tfstate",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &mockAzureBlobClient{
				downloadStreamFunc: func(ctx context.Context, containerName string, blobName string, options *azblob.DownloadStreamOptions) (AzureBlobDownloadResponse, error) {
					return nil, &azcore.ResponseError{StatusCode: statusCodeNotFoundAzure}
				},
			}

			result, err := ReadTerraformBackendAzurermInternal(mockClient, &tt.componentSections, &tt.backend)

			assert.NoError(t, err, "Should return nil error when blob not found")
			assert.Nil(t, result, "Should return nil content when blob not found")
		})
	}
}

func TestReadTerraformBackendAzurermInternal_PermissionDenied(t *testing.T) {
	componentSections := map[string]any{
		"component": "test-component",
		"workspace": "dev",
	}
	backend := map[string]any{
		"storage_account_name": "testaccount",
		"container_name":       "restricted-tfstate",
		"key":                  "terraform.tfstate",
	}

	mockClient := &mockAzureBlobClient{
		downloadStreamFunc: func(ctx context.Context, containerName string, blobName string, options *azblob.DownloadStreamOptions) (AzureBlobDownloadResponse, error) {
			return nil, &azcore.ResponseError{StatusCode: statusCodeForbiddenAzure}
		},
	}

	result, err := ReadTerraformBackendAzurermInternal(mockClient, &componentSections, &backend)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, errUtils.ErrAzurePermissionDenied)
	assert.Contains(t, err.Error(), "terraform.tfstateenv:dev")
	assert.Contains(t, err.Error(), "restricted-tfstate")
}

func TestReadTerraformBackendAzurermInternal_NetworkError(t *testing.T) {
	componentSections := map[string]any{
		"component": "test-component",
		"workspace": "default",
	}
	backend := map[string]any{
		"storage_account_name": "testaccount",
		"container_name":       "tfstate",
		"key":                  "terraform.tfstate",
	}

	attemptCount := 0
	mockClient := &mockAzureBlobClient{
		downloadStreamFunc: func(ctx context.Context, containerName string, blobName string, options *azblob.DownloadStreamOptions) (AzureBlobDownloadResponse, error) {
			attemptCount++
			return nil, errors.New("network timeout")
		},
	}

	result, err := ReadTerraformBackendAzurermInternal(mockClient, &componentSections, &backend)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, errUtils.ErrGetBlobFromAzure)
	assert.Equal(t, maxRetryCountAzure+1, attemptCount, "Should retry the maximum number of times")
}

func TestReadTerraformBackendAzurermInternal_RetrySuccess(t *testing.T) {
	componentSections := map[string]any{
		"component": "test-component",
		"workspace": "default",
	}
	backend := map[string]any{
		"storage_account_name": "testaccount",
		"container_name":       "tfstate",
		"key":                  "terraform.tfstate",
	}

	attemptCount := 0
	expectedContent := `{"version": 4, "outputs": {"success": {"value": "retry-worked"}}}`

	mockClient := &mockAzureBlobClient{
		downloadStreamFunc: func(ctx context.Context, containerName string, blobName string, options *azblob.DownloadStreamOptions) (AzureBlobDownloadResponse, error) {
			attemptCount++
			// Fail first attempt, succeed on second.
			if attemptCount == 1 {
				return nil, errors.New("temporary network error")
			}
			return createMockDownloadResponse(expectedContent), nil
		},
	}

	result, err := ReadTerraformBackendAzurermInternal(mockClient, &componentSections, &backend)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, expectedContent, string(result))
	assert.Equal(t, 2, attemptCount, "Should succeed on second attempt")
}

func TestReadTerraformBackendAzurermInternal_MissingContainerName(t *testing.T) {
	componentSections := map[string]any{
		"component": "test-component",
		"workspace": "default",
	}
	backend := map[string]any{
		"storage_account_name": "testaccount",
		// container_name is missing.
		"key": "terraform.tfstate",
	}

	mockClient := &mockAzureBlobClient{
		downloadStreamFunc: func(ctx context.Context, containerName string, blobName string, options *azblob.DownloadStreamOptions) (AzureBlobDownloadResponse, error) {
			t.Fatal("Should not call Azure client when container_name is missing")
			return nil, nil
		},
	}

	result, err := ReadTerraformBackendAzurermInternal(mockClient, &componentSections, &backend)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, errUtils.ErrAzureContainerRequired)
}

func TestReadTerraformBackendAzurermInternal_ReadBodyError(t *testing.T) {
	componentSections := map[string]any{
		"component": "test-component",
		"workspace": "default",
	}
	backend := map[string]any{
		"storage_account_name": "testaccount",
		"container_name":       "tfstate",
		"key":                  "terraform.tfstate",
	}

	mockClient := &mockAzureBlobClient{
		downloadStreamFunc: func(ctx context.Context, containerName string, blobName string, options *azblob.DownloadStreamOptions) (AzureBlobDownloadResponse, error) {
			// Return a reader that fails on read.
			return &mockDownloadResponse{
				body: io.NopCloser(&errorReader{}),
			}, nil
		},
	}

	result, err := ReadTerraformBackendAzurermInternal(mockClient, &componentSections, &backend)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, errUtils.ErrReadAzureBlobBody)
}

func TestReadTerraformBackendAzurerm_MissingBackend(t *testing.T) {
	componentSections := map[string]any{
		"component": "test-component",
		"workspace": "dev",
		// No backend section.
	}

	result, err := ReadTerraformBackendAzurerm(nil, &componentSections, nil)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, errUtils.ErrBackendConfigRequired)
}

func TestReadTerraformBackendAzurerm_EmptyStorageAccount(t *testing.T) {
	componentSections := map[string]any{
		"component": "test-component",
		"workspace": "dev",
		"backend": map[string]any{
			"storage_account_name": "",
			"container_name":       "tfstate",
			"key":                  "terraform.tfstate",
		},
	}

	result, err := ReadTerraformBackendAzurerm(nil, &componentSections, nil)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, errUtils.ErrStorageAccountRequired)
}

func TestReadTerraformBackendAzurerm_MissingStorageAccount(t *testing.T) {
	componentSections := map[string]any{
		"component": "test-component",
		"workspace": "dev",
		"backend": map[string]any{
			"container_name": "tfstate",
			"key":            "terraform.tfstate",
		},
	}

	result, err := ReadTerraformBackendAzurerm(nil, &componentSections, nil)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, errUtils.ErrStorageAccountRequired)
}

func TestReadTerraformBackendAzurerm_CachedClient(t *testing.T) {
	// Pre-populate cache with a mock client.
	cacheKey := "cachedaccount"
	mockContent := `{"version": 4, "outputs": {"cached": {"value": "from-cache"}}}`

	mockClient := &mockAzureBlobClient{
		downloadStreamFunc: func(ctx context.Context, containerName string, blobName string, options *azblob.DownloadStreamOptions) (AzureBlobDownloadResponse, error) {
			assert.Equal(t, "cachedcontainer", containerName)
			assert.Equal(t, "terraform.tfstate", blobName)
			return createMockDownloadResponse(mockContent), nil
		},
	}

	// Store in cache.
	azureBlobClientCache.Store(cacheKey, mockClient)
	defer azureBlobClientCache.Delete(cacheKey) // Clean up after test.

	componentSections := map[string]any{
		"component": "cached-component",
		"workspace": "default",
		"backend": map[string]any{
			"storage_account_name": "cachedaccount",
			"container_name":       "cachedcontainer",
			"key":                  "terraform.tfstate",
		},
	}

	result, err := ReadTerraformBackendAzurerm(nil, &componentSections, nil)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, mockContent, string(result))
}

// errorReader is a reader that always returns an error.
type errorReader struct{}

func (e *errorReader) Read(p []byte) (n int, err error) {
	return 0, errors.New("read error")
}

func TestConstructAzureBlobPath(t *testing.T) {
	tests := []struct {
		name              string
		componentSections map[string]any
		backend           map[string]any
		expectedPath      string
	}{
		{
			name: "default_workspace_with_explicit_key",
			componentSections: map[string]any{
				"workspace": "default",
			},
			backend: map[string]any{
				"key": "test.tfstate",
			},
			expectedPath: "test.tfstate",
		},
		{
			name: "default_workspace_without_key",
			componentSections: map[string]any{
				"workspace": "default",
			},
			backend:      map[string]any{},
			expectedPath: "terraform.tfstate",
		},
		{
			name: "empty_workspace_with_key",
			componentSections: map[string]any{
				"workspace": "",
			},
			backend: map[string]any{
				"key": "custom.tfstate",
			},
			expectedPath: "custom.tfstate",
		},
		{
			name: "non_default_workspace",
			componentSections: map[string]any{
				"workspace": "dev",
			},
			backend: map[string]any{
				"key": "app.tfstate",
			},
			expectedPath: "app.tfstateenv:dev",
		},
		{
			name: "non_default_workspace_no_key",
			componentSections: map[string]any{
				"workspace": "staging",
			},
			backend:      map[string]any{},
			expectedPath: "terraform.tfstateenv:staging",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := constructAzureBlobPath(&tt.componentSections, &tt.backend)
			assert.Equal(t, tt.expectedPath, result)
		})
	}
}

func TestHandleAzureDownloadError(t *testing.T) {
	tests := []struct {
		name            string
		err             error
		expectedError   error
		shouldReturnNil bool
	}{
		{
			name:            "not_found_error",
			err:             &azcore.ResponseError{StatusCode: statusCodeNotFoundAzure},
			expectedError:   nil,
			shouldReturnNil: true,
		},
		{
			name:            "permission_denied_error",
			err:             &azcore.ResponseError{StatusCode: statusCodeForbiddenAzure},
			expectedError:   errUtils.ErrAzurePermissionDenied,
			shouldReturnNil: false,
		},
		{
			name:            "other_response_error",
			err:             &azcore.ResponseError{StatusCode: 500},
			expectedError:   nil,
			shouldReturnNil: false,
		},
		{
			name:            "non_response_error",
			err:             errors.New("generic error"),
			expectedError:   nil,
			shouldReturnNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handleAzureDownloadError(tt.err, "test.tfstate", "test-container")

			if tt.shouldReturnNil {
				assert.Nil(t, result)
			} else if tt.expectedError != nil {
				assert.Error(t, result)
				assert.ErrorIs(t, result, tt.expectedError)
			} else {
				assert.Equal(t, tt.err, result)
			}
		})
	}
}

func TestReadTerraformBackendAzurermInternal_ContextTimeout(t *testing.T) {
	componentSections := map[string]any{
		"component": "test-component",
		"workspace": "default",
	}
	backend := map[string]any{
		"storage_account_name": "testaccount",
		"container_name":       "tfstate",
		"key":                  "terraform.tfstate",
	}

	// Mock client that simulates context timeout.
	mockClient := &mockAzureBlobClient{
		downloadStreamFunc: func(ctx context.Context, containerName string, blobName string, options *azblob.DownloadStreamOptions) (AzureBlobDownloadResponse, error) {
			return nil, context.DeadlineExceeded
		},
	}

	result, err := ReadTerraformBackendAzurermInternal(mockClient, &componentSections, &backend)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, errUtils.ErrGetBlobFromAzure)
}

func TestReadTerraformBackendAzurermInternal_MaxRetriesExceeded(t *testing.T) {
	componentSections := map[string]any{
		"component": "test-component",
		"workspace": "default",
	}
	backend := map[string]any{
		"storage_account_name": "testaccount",
		"container_name":       "tfstate",
		"key":                  "terraform.tfstate",
	}

	attemptCount := 0
	mockClient := &mockAzureBlobClient{
		downloadStreamFunc: func(ctx context.Context, containerName string, blobName string, options *azblob.DownloadStreamOptions) (AzureBlobDownloadResponse, error) {
			attemptCount++
			// Always fail to test retry exhaustion.
			return nil, errors.New("persistent error")
		},
	}

	result, err := ReadTerraformBackendAzurermInternal(mockClient, &componentSections, &backend)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, errUtils.ErrGetBlobFromAzure)
	assert.Equal(t, maxRetryCountAzure+1, attemptCount, "Should exhaust all retries")
}

func TestReadTerraformBackendAzurermInternal_SuccessWithLargeBlob(t *testing.T) {
	componentSections := map[string]any{
		"component": "large-component",
		"workspace": "prod",
	}
	backend := map[string]any{
		"storage_account_name": "prodaccount",
		"container_name":       "prod-tfstate",
		"key":                  "large.tfstate",
	}

	// Create a large JSON blob (simulating a complex terraform state).
	largeState := `{"version": 4, "outputs": {`
	for i := 0; i < 100; i++ {
		if i > 0 {
			largeState += ","
		}
		largeState += fmt.Sprintf(`"output_%d": {"value": "value_%d"}`, i, i)
	}
	largeState += `}}`

	mockClient := &mockAzureBlobClient{
		downloadStreamFunc: func(ctx context.Context, containerName string, blobName string, options *azblob.DownloadStreamOptions) (AzureBlobDownloadResponse, error) {
			assert.Equal(t, "prod-tfstate", containerName)
			assert.Equal(t, "large.tfstateenv:prod", blobName)
			return createMockDownloadResponse(largeState), nil
		},
	}

	result, err := ReadTerraformBackendAzurermInternal(mockClient, &componentSections, &backend)

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, largeState, string(result))
	assert.Greater(t, len(result), 1000, "Should handle large blobs")
}

func TestReadTerraformBackendAzurermInternal_EmptyBlobContent(t *testing.T) {
	componentSections := map[string]any{
		"component": "empty-component",
		"workspace": "default",
	}
	backend := map[string]any{
		"storage_account_name": "testaccount",
		"container_name":       "tfstate",
		"key":                  "empty.tfstate",
	}

	mockClient := &mockAzureBlobClient{
		downloadStreamFunc: func(ctx context.Context, containerName string, blobName string, options *azblob.DownloadStreamOptions) (AzureBlobDownloadResponse, error) {
			return createMockDownloadResponse(""), nil
		},
	}

	result, err := ReadTerraformBackendAzurermInternal(mockClient, &componentSections, &backend)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "", string(result))
}

func TestReadTerraformBackendAzurermInternal_SpecialCharactersInWorkspace(t *testing.T) {
	componentSections := map[string]any{
		"component": "test-component",
		"workspace": "dev-us-east-1-prod",
	}
	backend := map[string]any{
		"storage_account_name": "testaccount",
		"container_name":       "tfstate",
		"key":                  "app.terraform.tfstate",
	}

	var capturedBlobName string
	mockClient := &mockAzureBlobClient{
		downloadStreamFunc: func(ctx context.Context, containerName string, blobName string, options *azblob.DownloadStreamOptions) (AzureBlobDownloadResponse, error) {
			capturedBlobName = blobName
			return createMockDownloadResponse(`{"version": 4, "outputs": {}}`), nil
		},
	}

	result, err := ReadTerraformBackendAzurermInternal(mockClient, &componentSections, &backend)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "app.terraform.tfstateenv:dev-us-east-1-prod", capturedBlobName)
}

// Integration tests - these test the full path including getCachedAzureBlobClient.
// They will skip if Azure credentials are not available.

func TestReadTerraformBackendAzurerm_Integration_InvalidConfig(t *testing.T) {
	// Check for Azure credentials precondition.
	tests.RequireAzureCredentials(t)

	testCases := []struct {
		name          string
		componentData map[string]any
		wantErr       bool
		errType       error
	}{
		{
			name: "missing storage account",
			componentData: map[string]any{
				"workspace": "test-workspace",
				"backend": map[string]any{
					"container_name": "tfstate",
					"key":            "terraform.tfstate",
				},
			},
			wantErr: true,
			errType: errUtils.ErrStorageAccountRequired,
		},
		{
			name: "empty storage account",
			componentData: map[string]any{
				"workspace": "test-workspace",
				"backend": map[string]any{
					"storage_account_name": "",
					"container_name":       "tfstate",
					"key":                  "terraform.tfstate",
				},
			},
			wantErr: true,
			errType: errUtils.ErrStorageAccountRequired,
		},
		{
			name: "missing container name",
			componentData: map[string]any{
				"workspace": "test-workspace",
				"backend": map[string]any{
					"storage_account_name": "nonexistentaccount",
					"key":                  "terraform.tfstate",
				},
			},
			wantErr: true,
			errType: errUtils.ErrAzureContainerRequired,
		},
		{
			name: "nonexistent storage account",
			componentData: map[string]any{
				"workspace": "test-workspace",
				"backend": map[string]any{
					"storage_account_name": "nonexistentaccountxyz123",
					"container_name":       "tfstate",
					"key":                  "terraform.tfstate",
				},
			},
			wantErr: true,
			errType: errUtils.ErrGetBlobFromAzure,
		},
		{
			name: "nonexistent container",
			componentData: map[string]any{
				"workspace": "test-workspace",
				"backend": map[string]any{
					"storage_account_name": "testaccountxyz123",
					"container_name":       "nonexistentcontainer",
					"key":                  "terraform.tfstate",
				},
			},
			wantErr: true,
			errType: errUtils.ErrGetBlobFromAzure,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			// Clear Azure environment variables to prevent conflicts.
			t.Setenv("AZURE_STORAGE_ACCOUNT", "")
			t.Setenv("AZURE_STORAGE_KEY", "")

			atmosConfig := &schema.AtmosConfiguration{}

			result, err := ReadTerraformBackendAzurerm(atmosConfig, &tt.componentData, nil)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errType != nil {
					assert.ErrorIs(t, err, tt.errType)
				}
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
			}
		})
	}
}

func TestReadTerraformBackendAzurerm_Integration_BlobNotFound(t *testing.T) {
	// Check for Azure credentials precondition.
	tests.RequireAzureCredentials(t)

	componentSections := map[string]any{
		"workspace": "test-workspace-nonexistent",
		"backend": map[string]any{
			"storage_account_name": "nonexistentaccountxyz999",
			"container_name":       "nonexistentcontainer999",
			"key":                  "nonexistent.tfstate",
		},
	}

	result, err := ReadTerraformBackendAzurerm(nil, &componentSections, nil)
	// Should either return nil/nil (blob not found) or error (storage account doesn't exist).
	// Both are acceptable for this test since we're testing against nonexistent resources.
	if err != nil {
		assert.ErrorIs(t, err, errUtils.ErrGetBlobFromAzure)
	}
	// If no error, result should be nil (blob not found).
	if err == nil {
		assert.Nil(t, result)
	}
}

func TestReadTerraformBackendAzurerm_Integration_CacheKeyDeterminism(t *testing.T) {
	// Check for Azure credentials precondition.
	tests.RequireAzureCredentials(t)

	// Test that cache keys are deterministic and correctly handle different configs.
	testCases := []struct {
		name         string
		backend1     map[string]any
		backend2     map[string]any
		shouldBeSame bool
		description  string
	}{
		{
			name: "identical backends should use same cache",
			backend1: map[string]any{
				"storage_account_name": "account1",
				"container_name":       "container1",
				"key":                  "terraform.tfstate",
			},
			backend2: map[string]any{
				"storage_account_name": "account1",
				"container_name":       "container1",
				"key":                  "terraform.tfstate",
			},
			shouldBeSame: true,
			description:  "Same account and container should use cached client",
		},
		{
			name: "different storage accounts should use different cache",
			backend1: map[string]any{
				"storage_account_name": "account1",
				"container_name":       "container1",
				"key":                  "terraform.tfstate",
			},
			backend2: map[string]any{
				"storage_account_name": "account2",
				"container_name":       "container1",
				"key":                  "terraform.tfstate",
			},
			shouldBeSame: false,
			description:  "Different accounts should create separate clients",
		},
		{
			name: "different containers same account should use same cache",
			backend1: map[string]any{
				"storage_account_name": "account1",
				"container_name":       "container1",
				"key":                  "terraform.tfstate",
			},
			backend2: map[string]any{
				"storage_account_name": "account1",
				"container_name":       "container2",
				"key":                  "terraform.tfstate",
			},
			shouldBeSame: true,
			description:  "Same account with different containers should share client",
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			// Generate cache keys manually to verify determinism.
			// Cache keys are now based on storage account only.
			cacheKey1 := GetBackendAttribute(&tt.backend1, "storage_account_name")
			cacheKey2 := GetBackendAttribute(&tt.backend2, "storage_account_name")

			if tt.shouldBeSame {
				assert.Equal(t, cacheKey1, cacheKey2, tt.description)
			} else {
				assert.NotEqual(t, cacheKey1, cacheKey2, tt.description)
			}
		})
	}
}

func TestReadTerraformBackendAzurerm_Integration_WorkspaceNaming(t *testing.T) {
	// Check for Azure credentials precondition.
	tests.RequireAzureCredentials(t)

	testCases := []struct {
		name             string
		workspace        string
		key              string
		expectedBlobName string
	}{
		{
			name:             "default workspace",
			workspace:        "default",
			key:              "terraform.tfstate",
			expectedBlobName: "terraform.tfstate",
		},
		{
			name:             "empty workspace treated as default",
			workspace:        "",
			key:              "terraform.tfstate",
			expectedBlobName: "terraform.tfstate",
		},
		{
			name:             "named workspace",
			workspace:        "dev",
			key:              "terraform.tfstate",
			expectedBlobName: "terraform.tfstateenv:dev",
		},
		{
			name:             "complex workspace name",
			workspace:        "dev-us-east-1-prod",
			key:              "app.terraform.tfstate",
			expectedBlobName: "app.terraform.tfstateenv:dev-us-east-1-prod",
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			componentSections := map[string]any{
				"workspace": tt.workspace,
			}
			backend := map[string]any{
				"key": tt.key,
			}

			// Test the blob path construction directly.
			actualBlobName := constructAzureBlobPath(&componentSections, &backend)
			assert.Equal(t, tt.expectedBlobName, actualBlobName)
		})
	}
}
