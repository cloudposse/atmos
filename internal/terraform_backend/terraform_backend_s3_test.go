package terraform_backend_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
	tb "github.com/cloudposse/atmos/internal/terraform_backend"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/tests"
)

func TestReadTerraformBackendS3_InvalidConfig(t *testing.T) {
	// Check for AWS profile precondition
	tests.RequireAWSProfile(t, "cplive-core-gbl-identity")
	tests := []struct {
		name          string
		componentData map[string]any
		wantErr       bool
		errType       error
	}{
		{
			name: "backend info with role arn",
			componentData: map[string]any{
				"workspace": "test-workspace",
				"backend": map[string]any{
					"type": "s3",
					"s3": map[string]any{
						"bucket":               "test-bucket",
						"region":               "us-east-2",
						"key":                  "terraform.tfstate",
						"role_arn":             "arn:aws:iam::123456789012:role/test-role",
						"workspace_key_prefix": "test-prefix",
					},
				},
			},
			wantErr: true,
			errType: errUtils.ErrGetObjectFromS3,
		},
		{
			name: "backend info without role arn",
			componentData: map[string]any{
				"workspace": "test-workspace",
				"backend": map[string]any{
					"type": "s3",
					"s3": map[string]any{
						"bucket":               "test-bucket",
						"region":               "us-east-2",
						"key":                  "terraform.tfstate",
						"workspace_key_prefix": "test-prefix",
					},
				},
			},
			wantErr: true,
			errType: errUtils.ErrGetObjectFromS3,
		},
		{
			name: "invalid backend info - missing bucket",
			componentData: map[string]any{
				"workspace": "test-workspace",
				"backend": map[string]any{
					"type": "s3",
					"s3": map[string]any{
						"region":               "us-east-2",
						"key":                  "terraform.tfstate",
						"workspace_key_prefix": "test-prefix",
					},
				},
			},
			wantErr: true,
			errType: errUtils.ErrGetObjectFromS3,
		},
		{
			name: "invalid backend info - missing region",
			componentData: map[string]any{
				"workspace": "test-workspace",
				"backend": map[string]any{
					"type": "s3",
					"s3": map[string]any{
						"bucket":               "test-bucket",
						"key":                  "terraform.tfstate",
						"workspace_key_prefix": "test-prefix",
					},
				},
			},
			wantErr: true,
			errType: errUtils.ErrGetObjectFromS3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear AWS_PROFILE to prevent conflicts with local AWS configuration.
			t.Setenv("AWS_PROFILE", "")

			atmosConfig := &schema.AtmosConfiguration{}

			result, err := tb.ReadTerraformBackendS3(atmosConfig, &tt.componentData, nil)

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

// mockS3Client implements only the GetObject method used in readTerraformBackendS3Internal.
type mockS3Client struct{}

func (m *mockS3Client) GetObject(ctx context.Context, input *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	if *input.Bucket == "mock-bucket" && *input.Key == "test-prefix/test-workspace/terraform.tfstate" {
		body := `{
			"version": 4,
			"terraform_version": "1.4.0",
			"outputs": {
				"example": {
					"value": "mocked-output",
					"type": "string"
				}
			}
		}`
		return &s3.GetObjectOutput{
			Body: io.NopCloser(strings.NewReader(body)),
		}, nil
	}
	return nil, &s3types.NoSuchKey{}
}

func Test_ReadTerraformBackendS3Internal(t *testing.T) {
	componentSections := map[string]any{
		"workspace": "test-workspace",
	}
	backend := map[string]any{
		"bucket":               "mock-bucket",
		"region":               "us-east-1",
		"key":                  "terraform.tfstate",
		"workspace_key_prefix": "test-prefix",
	}

	client := &mockS3Client{}

	content, err := tb.ReadTerraformBackendS3Internal(client, &componentSections, &backend)
	assert.NoError(t, err)
	assert.NotNil(t, content)
	assert.Contains(t, string(content), "mocked-output")
}

// Unified interface-compatible mock.
type erroringS3Client struct {
	err  error
	body io.ReadCloser
}

// Simulate failure in io.ReadAll.
type badReader struct{}

func (badReader) Read([]byte) (int, error) { return 0, errors.New("read failure") }
func (badReader) Close() error             { return nil }

func (m *erroringS3Client) GetObject(ctx context.Context, input *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &s3.GetObjectOutput{Body: m.body}, nil
}

func Test_ReadTerraformBackendS3Internal_Errors(t *testing.T) {
	tests := []struct {
		name            string
		client          *erroringS3Client
		expectedErrSub  string
		expectedNilBody bool
	}{
		{
			name: "no such key (missing file)",
			client: &erroringS3Client{
				err: &s3types.NoSuchKey{},
			},
			expectedErrSub:  "",
			expectedNilBody: true,
		},
		{
			name: "access denied",
			client: &erroringS3Client{
				err: &smithy.GenericAPIError{
					Code:    "AccessDenied",
					Message: "Access Denied",
				},
			},
			expectedErrSub:  "AccessDenied",
			expectedNilBody: true,
		},
		{
			name: "no such bucket",
			client: &erroringS3Client{
				err: &smithy.GenericAPIError{
					Code:    "NoSuchBucket",
					Message: "The specified bucket does not exist",
				},
			},
			expectedErrSub:  "NoSuchBucket",
			expectedNilBody: true,
		},
		{
			name: "signature mismatch",
			client: &erroringS3Client{
				err: &smithy.GenericAPIError{
					Code:    "SignatureDoesNotMatch",
					Message: "Signature error",
				},
			},
			expectedErrSub:  "SignatureDoesNotMatch",
			expectedNilBody: true,
		},
		{
			name: "timeout (context deadline exceeded)",
			client: &erroringS3Client{
				err: context.DeadlineExceeded,
			},
			expectedErrSub:  "context deadline",
			expectedNilBody: true,
		},
		{
			name: "read failure on body",
			client: &erroringS3Client{
				body: io.NopCloser(badReader{}),
			},
			expectedErrSub:  "read failure",
			expectedNilBody: true,
		},
	}

	componentSections := map[string]any{"workspace": "test-workspace"}
	backend := map[string]any{
		"bucket":               "mock-bucket",
		"region":               "us-east-1",
		"key":                  "terraform.tfstate",
		"workspace_key_prefix": "test-prefix",
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content, err := tb.ReadTerraformBackendS3Internal(tt.client, &componentSections, &backend)
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

// mockS3ClientForDefaultWorkspace is a mock that captures the requested key
// to verify path construction for default workspace.
type mockS3ClientForDefaultWorkspace struct {
	requestedKey string
}

func (m *mockS3ClientForDefaultWorkspace) GetObject(ctx context.Context, input *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	m.requestedKey = *input.Key
	// Return valid state for the expected key.
	body := `{
		"version": 4,
		"terraform_version": "1.4.0",
		"outputs": {
			"vpc_id": {
				"value": "vpc-12345",
				"type": "string"
			}
		}
	}`
	return &s3.GetObjectOutput{
		Body: io.NopCloser(strings.NewReader(body)),
	}, nil
}

// TestReadTerraformBackendS3Internal_DefaultWorkspace verifies that when workspace
// is "default" (meaning workspaces are disabled), the state file path should be
// just the key, not workspace_key_prefix/default/key.
//
// This is based on Terraform S3 backend documentation:
// - workspace_key_prefix is only used for non-default workspaces
// - For the default workspace, state is stored directly at the key path.
//
// See: https://github.com/cloudposse/atmos/issues/1920
func TestReadTerraformBackendS3Internal_DefaultWorkspace(t *testing.T) {
	tests := []struct {
		name               string
		workspace          string
		workspaceKeyPrefix string
		key                string
		expectedPath       string
	}{
		{
			name:               "default workspace - should use key only",
			workspace:          "default",
			workspaceKeyPrefix: "my-component",
			key:                "terraform.tfstate",
			expectedPath:       "terraform.tfstate",
		},
		{
			name:               "default workspace with env prefix - should use key only",
			workspace:          "default",
			workspaceKeyPrefix: "env:",
			key:                "state/terraform.tfstate",
			expectedPath:       "state/terraform.tfstate",
		},
		{
			name:               "named workspace - should use full path",
			workspace:          "prod-us-east-1",
			workspaceKeyPrefix: "my-component",
			key:                "terraform.tfstate",
			expectedPath:       "my-component/prod-us-east-1/terraform.tfstate",
		},
		{
			name:               "named workspace with env prefix - should use full path",
			workspace:          "staging",
			workspaceKeyPrefix: "env:",
			key:                "terraform.tfstate",
			expectedPath:       "env:/staging/terraform.tfstate",
		},
		{
			name:               "empty workspace_key_prefix with named workspace",
			workspace:          "prod",
			workspaceKeyPrefix: "",
			key:                "terraform.tfstate",
			expectedPath:       "prod/terraform.tfstate",
		},
		{
			name:               "empty workspace_key_prefix with default workspace",
			workspace:          "default",
			workspaceKeyPrefix: "",
			key:                "terraform.tfstate",
			expectedPath:       "terraform.tfstate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mockS3ClientForDefaultWorkspace{}
			componentSections := map[string]any{
				"workspace": tt.workspace,
			}
			backend := map[string]any{
				"bucket":               "test-bucket",
				"region":               "us-east-1",
				"key":                  tt.key,
				"workspace_key_prefix": tt.workspaceKeyPrefix,
			}

			// Call the function.
			_, _ = tb.ReadTerraformBackendS3Internal(client, &componentSections, &backend)

			// Verify the requested path matches the expected path.
			assert.Equal(t, tt.expectedPath, client.requestedKey,
				"For workspace '%s' with workspace_key_prefix '%s', expected path '%s' but got '%s'",
				tt.workspace, tt.workspaceKeyPrefix, tt.expectedPath, client.requestedKey)
		})
	}
}

func TestGetS3BackendAssumeRoleArn(t *testing.T) {
	tests := []struct {
		name     string
		backend  map[string]any
		expected string
	}{
		{
			name: "role_arn in assume_role section",
			backend: map[string]any{
				"assume_role": map[string]any{
					"role_arn": "arn:aws:iam::123456789012:role/terraform",
				},
			},
			expected: "arn:aws:iam::123456789012:role/terraform",
		},
		{
			name: "role_arn in root section",
			backend: map[string]any{
				"role_arn": "arn:aws:iam::123456789012:role/root",
			},
			expected: "arn:aws:iam::123456789012:role/root",
		},
		{
			name: "prefer assume_role.role_arn over root role_arn",
			backend: map[string]any{
				"role_arn": "arn:aws:iam::123456789012:role/root",
				"assume_role": map[string]any{
					"role_arn": "arn:aws:iam::123456789012:role/terraform",
				},
			},
			expected: "arn:aws:iam::123456789012:role/terraform",
		},
		{
			name: "empty assume_role section",
			backend: map[string]any{
				"assume_role": map[string]any{},
			},
			expected: "",
		},
		{
			name:     "empty backend",
			backend:  map[string]any{},
			expected: "",
		},
		{
			name: "assume_role section with nil value",
			backend: map[string]any{
				"assume_role": nil,
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tb.GetS3BackendAssumeRoleArn(&tt.backend)
			if got != tt.expected {
				t.Errorf("GetS3BackendAssumeRoleArn() = %v, want %v", got, tt.expected)
			}
		})
	}
}
