package terraform_backend_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
	tb "github.com/cloudposse/atmos/internal/terraform_backend"
)

func TestReadTerraformBackendGCS_InvalidConfig(t *testing.T) {
	tests := []struct {
		name          string
		componentData map[string]any
		gcsBackend    map[string]any
		wantErr       bool
		errType       error
	}{
		{
			name: "missing bucket configuration",
			componentData: map[string]any{
				"workspace": "test-workspace",
			},
			gcsBackend: map[string]any{
				"prefix": "test-prefix",
			},
			wantErr: true,
			errType: errUtils.ErrInvalidBackendConfig,
		},
		{
			name: "empty GCS configuration",
			componentData: map[string]any{
				"workspace": "test-workspace",
			},
			gcsBackend: map[string]any{},
			wantErr:    true,
			errType:    errUtils.ErrInvalidBackendConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock client that should never be called due to validation errors
			mockClient := &mockGCSClient{}

			result, err := tb.ReadTerraformBackendGCSInternal(mockClient, &tt.componentData, &tt.gcsBackend)

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

// Mock implementations for testing
type mockGCSClient struct{}

func (m *mockGCSClient) Bucket(name string) tb.GCSBucketHandle {
	return &mockGCSBucketHandle{bucketName: name}
}

type mockGCSBucketHandle struct {
	bucketName string
}

func (b *mockGCSBucketHandle) Object(name string) tb.GCSObjectHandle {
	return &mockGCSObjectHandle{bucketName: b.bucketName, objectName: name}
}

type mockGCSObjectHandle struct {
	bucketName string
	objectName string
}

func (o *mockGCSObjectHandle) NewReader(ctx context.Context) (io.ReadCloser, error) {
	// Accept multiple path patterns for different test scenarios
	// Correct GCS backend path format: <prefix>/<workspace>.tfstate
	validPaths := []string{
		"test-prefix/test-workspace.tfstate",
		"test-workspace.tfstate", // For no prefix
	}
	
	pathMatches := false
	for _, validPath := range validPaths {
		if o.bucketName == "mock-bucket" && o.objectName == validPath {
			pathMatches = true
			break
		}
		// Also check test-bucket for nested config test
		if o.bucketName == "test-bucket" && o.objectName == validPath {
			pathMatches = true
			break
		}
	}
	
	if pathMatches {
		body := `{
			"version": 4,
			"terraform_version": "1.4.0",
			"outputs": {
				"example": {
					"value": "mocked-gcs-output",
					"type": "string"
				}
			}
		}`
		return io.NopCloser(strings.NewReader(body)), nil
	}
	return nil, status.Error(codes.NotFound, "object not found")
}

func Test_ReadTerraformBackendGCSInternal(t *testing.T) {
	componentSections := map[string]any{
		"workspace": "test-workspace",
	}
	backend := map[string]any{
		"bucket": "mock-bucket",
		"prefix": "test-prefix",
	}

	client := &mockGCSClient{}

	content, err := tb.ReadTerraformBackendGCSInternal(client, &componentSections, &backend)
	assert.NoError(t, err)
	assert.NotNil(t, content)
	assert.Contains(t, string(content), "mocked-gcs-output")
}

func TestReadTerraformBackendGCS_NestedConfig(t *testing.T) {
	// Test with nested configuration (like when called from !terraform.state)
	componentData := map[string]any{
		"workspace": "test-workspace",
		"backend": map[string]any{
			"type": "gcs",
			"gcs": map[string]any{
				"bucket": "test-bucket",
				"prefix": "test-prefix",
			},
		},
	}

	client := &mockGCSClient{}

	// We need to test the internal function since the main function would try to create a real GCS client
	backend := map[string]any{
		"bucket": "test-bucket",
		"prefix": "test-prefix",
	}

	content, err := tb.ReadTerraformBackendGCSInternal(client, &componentData, &backend)
	assert.NoError(t, err)
	assert.NotNil(t, content)
	assert.Contains(t, string(content), "mocked-gcs-output")
}

// Error testing mock
type erroringGCSClient struct {
	err  error
	body io.ReadCloser
}

func (m *erroringGCSClient) Bucket(name string) tb.GCSBucketHandle {
	return &erroringGCSBucketHandle{client: m}
}

type erroringGCSBucketHandle struct {
	client *erroringGCSClient
}

func (b *erroringGCSBucketHandle) Object(name string) tb.GCSObjectHandle {
	return &erroringGCSObjectHandle{client: b.client}
}

type erroringGCSObjectHandle struct {
	client *erroringGCSClient
}

func (o *erroringGCSObjectHandle) NewReader(ctx context.Context) (io.ReadCloser, error) {
	if o.client.err != nil {
		return nil, o.client.err
	}
	return o.client.body, nil
}

// Simulate failure in io.ReadAll
type badGCSReader struct{}

func (badGCSReader) Read([]byte) (int, error) { return 0, errors.New("gcs read failure") }
func (badGCSReader) Close() error             { return nil }

func Test_ReadTerraformBackendGCSInternal_Errors(t *testing.T) {
	tests := []struct {
		name            string
		client          *erroringGCSClient
		backend         map[string]any
		expectedErrSub  string
		expectedNilBody bool
	}{
		{
			name: "object not found (missing file)",
			client: &erroringGCSClient{
				err: status.Error(codes.NotFound, "object not found"),
			},
			backend: map[string]any{
				"bucket": "mock-bucket",
				"prefix": "test-prefix",
			},
			expectedErrSub:  "",
			expectedNilBody: true,
		},
		{
			name: "permission denied",
			client: &erroringGCSClient{
				err: status.Error(codes.PermissionDenied, "permission denied"),
			},
			backend: map[string]any{
				"bucket": "mock-bucket",
				"prefix": "test-prefix",
			},
			expectedErrSub:  "permission denied",
			expectedNilBody: true,
		},
		{
			name: "invalid bucket",
			client: &erroringGCSClient{
				err: status.Error(codes.InvalidArgument, "invalid bucket name"),
			},
			backend: map[string]any{
				"bucket": "mock-bucket",
				"prefix": "test-prefix",
			},
			expectedErrSub:  "invalid bucket",
			expectedNilBody: true,
		},
		{
			name: "timeout (context deadline exceeded)",
			client: &erroringGCSClient{
				err: context.DeadlineExceeded,
			},
			backend: map[string]any{
				"bucket": "mock-bucket",
				"prefix": "test-prefix",
			},
			expectedErrSub:  "context deadline",
			expectedNilBody: true,
		},
		{
			name: "read failure on body",
			client: &erroringGCSClient{
				body: io.NopCloser(badGCSReader{}),
			},
			backend: map[string]any{
				"bucket": "mock-bucket",
				"prefix": "test-prefix",
			},
			expectedErrSub:  "gcs read failure",
			expectedNilBody: true,
		},
		{
			name: "missing bucket configuration",
			client: &erroringGCSClient{},
			backend: map[string]any{
				"prefix": "test-prefix",
			},
			expectedErrSub:  "bucket name is required",
			expectedNilBody: true,
		},
	}

	componentSections := map[string]any{"workspace": "test-workspace"}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content, err := tb.ReadTerraformBackendGCSInternal(tt.client, &componentSections, &tt.backend)
			if tt.expectedErrSub == "" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErrSub)
			}

			if tt.expectedNilBody {
				assert.Nil(t, content)
			}
		})
	}
}

func TestGetGCSBackendServiceAccount(t *testing.T) {
	tests := []struct {
		name     string
		backend  map[string]any
		expected string
	}{
		{
			name: "credentials file path",
			backend: map[string]any{
				"credentials": "/path/to/service-account.json",
			},
			expected: "/path/to/service-account.json",
		},
		{
			name: "no credentials",
			backend: map[string]any{
				"bucket": "test-bucket",
			},
			expected: "",
		},
		{
			name:     "empty backend",
			backend:  map[string]any{},
			expected: "",
		},
		{
			name: "empty credentials",
			backend: map[string]any{
				"credentials": "",
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tb.GetGCSBackendServiceAccount(&tt.backend)
			if got != tt.expected {
				t.Errorf("GetGCSBackendServiceAccount() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestGetGCSBackendImpersonateServiceAccount(t *testing.T) {
	tests := []struct {
		name     string
		backend  map[string]any
		expected string
	}{
		{
			name: "impersonate service account",
			backend: map[string]any{
				"impersonate_service_account": "test-sa@project.iam.gserviceaccount.com",
			},
			expected: "test-sa@project.iam.gserviceaccount.com",
		},
		{
			name: "no impersonation",
			backend: map[string]any{
				"bucket": "test-bucket",
			},
			expected: "",
		},
		{
			name:     "empty backend",
			backend:  map[string]any{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tb.GetGCSBackendImpersonateServiceAccount(&tt.backend)
			if got != tt.expected {
				t.Errorf("GetGCSBackendImpersonateServiceAccount() = %v, want %v", got, tt.expected)
			}
		})
	}
}