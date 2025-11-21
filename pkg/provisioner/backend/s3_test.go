package backend

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/provisioner"
)

func TestExtractS3Config(t *testing.T) {
	tests := []struct {
		name          string
		backendConfig map[string]any
		want          *s3Config
		wantErr       error
	}{
		{
			name: "valid config with all fields",
			backendConfig: map[string]any{
				"bucket": "my-terraform-state",
				"region": "us-west-2",
				"assume_role": map[string]any{
					"role_arn": "arn:aws:iam::123456789012:role/TerraformRole",
				},
			},
			want: &s3Config{
				bucket:  "my-terraform-state",
				region:  "us-west-2",
				roleArn: "arn:aws:iam::123456789012:role/TerraformRole",
			},
			wantErr: nil,
		},
		{
			name: "valid config without role ARN",
			backendConfig: map[string]any{
				"bucket": "my-terraform-state",
				"region": "us-east-1",
			},
			want: &s3Config{
				bucket:  "my-terraform-state",
				region:  "us-east-1",
				roleArn: "",
			},
			wantErr: nil,
		},
		{
			name: "missing bucket",
			backendConfig: map[string]any{
				"region": "us-west-2",
			},
			want:    nil,
			wantErr: provisioner.ErrBucketRequired,
		},
		{
			name: "empty bucket",
			backendConfig: map[string]any{
				"bucket": "",
				"region": "us-west-2",
			},
			want:    nil,
			wantErr: provisioner.ErrBucketRequired,
		},
		{
			name: "missing region",
			backendConfig: map[string]any{
				"bucket": "my-terraform-state",
			},
			want:    nil,
			wantErr: provisioner.ErrRegionRequired,
		},
		{
			name: "empty region",
			backendConfig: map[string]any{
				"bucket": "my-terraform-state",
				"region": "",
			},
			want:    nil,
			wantErr: provisioner.ErrRegionRequired,
		},
		{
			name: "invalid bucket type",
			backendConfig: map[string]any{
				"bucket": 12345,
				"region": "us-west-2",
			},
			want:    nil,
			wantErr: provisioner.ErrBucketRequired,
		},
		{
			name: "invalid region type",
			backendConfig: map[string]any{
				"bucket": "my-terraform-state",
				"region": 12345,
			},
			want:    nil,
			wantErr: provisioner.ErrRegionRequired,
		},
		{
			name: "assume_role with empty role_arn",
			backendConfig: map[string]any{
				"bucket": "my-terraform-state",
				"region": "us-west-2",
				"assume_role": map[string]any{
					"role_arn": "",
				},
			},
			want: &s3Config{
				bucket:  "my-terraform-state",
				region:  "us-west-2",
				roleArn: "",
			},
			wantErr: nil,
		},
		{
			name: "assume_role with invalid type",
			backendConfig: map[string]any{
				"bucket":      "my-terraform-state",
				"region":      "us-west-2",
				"assume_role": "not-a-map",
			},
			want: &s3Config{
				bucket:  "my-terraform-state",
				region:  "us-west-2",
				roleArn: "",
			},
			wantErr: nil,
		},
		{
			name: "complex role ARN",
			backendConfig: map[string]any{
				"bucket": "my-terraform-state",
				"region": "eu-west-1",
				"assume_role": map[string]any{
					"role_arn":     "arn:aws:iam::987654321098:role/CrossAccountRole",
					"session_name": "terraform-session", // Extra field (ignored)
				},
			},
			want: &s3Config{
				bucket:  "my-terraform-state",
				region:  "eu-west-1",
				roleArn: "arn:aws:iam::987654321098:role/CrossAccountRole",
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractS3Config(tt.backendConfig)

			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, got)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestS3ProvisionerRegistration(t *testing.T) {
	// Test that S3 provisioner is registered in init().
	provisioner := GetBackendProvisioner("s3")
	assert.NotNil(t, provisioner, "S3 provisioner should be registered")
}

func TestS3Config_FieldValues(t *testing.T) {
	// Test s3Config structure holds correct values.
	config := &s3Config{
		bucket:  "test-bucket",
		region:  "us-west-2",
		roleArn: "arn:aws:iam::123456789012:role/TestRole",
	}

	assert.Equal(t, "test-bucket", config.bucket)
	assert.Equal(t, "us-west-2", config.region)
	assert.Equal(t, "arn:aws:iam::123456789012:role/TestRole", config.roleArn)
}

func TestExtractS3Config_AllRegions(t *testing.T) {
	// Test various AWS regions.
	regions := []string{
		"us-east-1",
		"us-east-2",
		"us-west-1",
		"us-west-2",
		"eu-west-1",
		"eu-central-1",
		"ap-southeast-1",
		"ap-northeast-1",
	}

	for _, region := range regions {
		t.Run(region, func(t *testing.T) {
			backendConfig := map[string]any{
				"bucket": "test-bucket",
				"region": region,
			}

			got, err := extractS3Config(backendConfig)
			require.NoError(t, err)
			assert.Equal(t, region, got.region)
		})
	}
}

func TestExtractS3Config_BucketNameValidation(t *testing.T) {
	// Test various bucket name scenarios.
	tests := []struct {
		name       string
		bucketName any
		shouldPass bool
	}{
		{
			name:       "valid bucket name",
			bucketName: "my-terraform-state-bucket",
			shouldPass: true,
		},
		{
			name:       "bucket with dots",
			bucketName: "my.terraform.state.bucket",
			shouldPass: true,
		},
		{
			name:       "bucket with numbers",
			bucketName: "terraform-state-123456",
			shouldPass: true,
		},
		{
			name:       "nil bucket",
			bucketName: nil,
			shouldPass: false,
		},
		{
			name:       "empty string bucket",
			bucketName: "",
			shouldPass: false,
		},
		{
			name:       "int bucket",
			bucketName: 123,
			shouldPass: false,
		},
		{
			name:       "bool bucket",
			bucketName: true,
			shouldPass: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backendConfig := map[string]any{
				"bucket": tt.bucketName,
				"region": "us-west-2",
			}

			_, err := extractS3Config(backendConfig)
			if tt.shouldPass {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.ErrorIs(t, err, provisioner.ErrBucketRequired)
			}
		})
	}
}

func TestBeforeTerraformInitConstant(t *testing.T) {
	// Verify the constant matches expected value.
	assert.Equal(t, "before.terraform.init", beforeTerraformInitEvent)
}

func TestErrFormatConstant(t *testing.T) {
	// Verify error format constant.
	assert.Equal(t, "%w: %w", errFormat)
}

// Note: Integration tests for S3 bucket operations (bucketExists, createBucket, etc.)
// would require either:
// 1. Real AWS credentials and live AWS resources (not suitable for unit tests)
// 2. Complex mocking of the AWS S3 SDK (beyond scope of basic unit tests)
// 3. Integration tests with localstack or similar (placed in tests/ directory)
//
// The functions above provide good coverage of the configuration parsing and
// validation logic, which is the most critical part for unit testing.
// AWS SDK integration is tested via integration tests in tests/ directory.
