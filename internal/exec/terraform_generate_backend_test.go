package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	provWorkdir "github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestValidateBackendConfig(t *testing.T) {
	tests := []struct {
		name        string
		info        *schema.ConfigAndStacksInfo
		expectedErr error
	}{
		{
			name: "Valid backend config",
			info: &schema.ConfigAndStacksInfo{
				ComponentBackendType:    "s3",
				ComponentBackendSection: map[string]any{"bucket": "test-bucket"},
			},
			expectedErr: nil,
		},
		{
			name: "Missing backend type",
			info: &schema.ConfigAndStacksInfo{
				ComponentBackendType:    "",
				ComponentBackendSection: map[string]any{"bucket": "test-bucket"},
			},
			expectedErr: errUtils.ErrMissingTerraformBackendType,
		},
		{
			name: "Missing backend section",
			info: &schema.ConfigAndStacksInfo{
				ComponentBackendType:    "s3",
				ComponentBackendSection: nil,
			},
			expectedErr: errUtils.ErrMissingTerraformBackendConfig,
		},
		{
			name: "Both missing",
			info: &schema.ConfigAndStacksInfo{
				ComponentBackendType:    "",
				ComponentBackendSection: nil,
			},
			expectedErr: errUtils.ErrMissingTerraformBackendType,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBackendConfig(tt.info)
			if tt.expectedErr != nil {
				assert.ErrorIs(t, err, tt.expectedErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateBackendTypeRequirements(t *testing.T) {
	tests := []struct {
		name        string
		info        *schema.ConfigAndStacksInfo
		expectedErr error
	}{
		{
			name: "S3 backend with workspace_key_prefix",
			info: &schema.ConfigAndStacksInfo{
				ComponentBackendType: cfg.BackendTypeS3,
				ComponentBackendSection: map[string]any{
					"workspace_key_prefix": "terraform-state",
				},
			},
			expectedErr: nil,
		},
		{
			name: "S3 backend without workspace_key_prefix",
			info: &schema.ConfigAndStacksInfo{
				ComponentBackendType:    cfg.BackendTypeS3,
				ComponentBackendSection: map[string]any{},
			},
			expectedErr: errUtils.ErrMissingTerraformWorkspaceKeyPrefix,
		},
		{
			name: "GCS backend with bucket",
			info: &schema.ConfigAndStacksInfo{
				ComponentBackendType: cfg.BackendTypeGCS,
				ComponentBackendSection: map[string]any{
					"bucket": "test-bucket",
				},
			},
			expectedErr: nil,
		},
		{
			name: "GCS backend without bucket",
			info: &schema.ConfigAndStacksInfo{
				ComponentBackendType:    cfg.BackendTypeGCS,
				ComponentBackendSection: map[string]any{},
			},
			expectedErr: errUtils.ErrGCSBucketRequired,
		},
		{
			name: "Azure backend - no specific requirements",
			info: &schema.ConfigAndStacksInfo{
				ComponentBackendType:    cfg.BackendTypeAzurerm,
				ComponentBackendSection: map[string]any{},
			},
			expectedErr: nil,
		},
		{
			name: "Local backend - no specific requirements",
			info: &schema.ConfigAndStacksInfo{
				ComponentBackendType:    "local",
				ComponentBackendSection: map[string]any{},
			},
			expectedErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBackendTypeRequirements(tt.info)
			if tt.expectedErr != nil {
				assert.ErrorIs(t, err, tt.expectedErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestExecuteTerraformGenerateBackendCmd_Deprecated tests the deprecated command returns an error.
func TestExecuteTerraformGenerateBackendCmd_Deprecated(t *testing.T) {
	err := ExecuteTerraformGenerateBackendCmd(nil, nil)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrDeprecatedCmdNotCallable)
}

// TestWriteBackendConfigFile tests the writeBackendConfigFile function across dry-run, normal write, and workdir cases.
func TestWriteBackendConfigFile(t *testing.T) {
	tempDir := t.TempDir()
	componentDir := filepath.Join(tempDir, "components", "terraform", "vpc")
	workDir := filepath.Join(tempDir, "workdir", "vpc")
	require.NoError(t, os.MkdirAll(componentDir, 0o755))
	require.NoError(t, os.MkdirAll(workDir, 0o755))

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	tests := []struct {
		name         string
		info         *schema.ConfigAndStacksInfo
		config       map[string]any
		wantPath     string
		wantContains string
	}{
		{
			name:   "dry-run skips writing",
			info:   &schema.ConfigAndStacksInfo{FinalComponent: "vpc", DryRun: true},
			config: map[string]any{"terraform": map[string]any{"backend": map[string]any{"s3": map[string]any{"bucket": "test-bucket"}}}},
		},
		{
			name:         "writes to component dir",
			info:         &schema.ConfigAndStacksInfo{FinalComponent: "vpc", ComponentSection: map[string]any{}},
			config:       map[string]any{"terraform": map[string]any{"backend": map[string]any{"s3": map[string]any{"bucket": "my-state-bucket", "key": "vpc/terraform.tfstate", "region": "us-east-1"}}}},
			wantPath:     filepath.Join(componentDir, "backend.tf.json"),
			wantContains: "my-state-bucket",
		},
		{
			name: "writes to workdir path",
			info: &schema.ConfigAndStacksInfo{
				FinalComponent:   "vpc",
				ComponentSection: map[string]any{provWorkdir.WorkdirPathKey: workDir},
			},
			config:   map[string]any{"terraform": map[string]any{"backend": map[string]any{"s3": map[string]any{"bucket": "test"}}}},
			wantPath: filepath.Join(workDir, "backend.tf.json"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := writeBackendConfigFile(atmosConfig, tt.info, tt.config)
			assert.NoError(t, err)

			if tt.wantPath != "" {
				assert.FileExists(t, tt.wantPath)
				if tt.wantContains != "" {
					content, readErr := os.ReadFile(tt.wantPath)
					require.NoError(t, readErr)
					assert.Contains(t, string(content), tt.wantContains)
				}
			}
		})
	}
}

func TestValidateBackendTypeRequirementsTypeAssertions(t *testing.T) {
	tests := []struct {
		name        string
		info        *schema.ConfigAndStacksInfo
		expectedErr error
	}{
		{
			name: "S3 backend with workspace_key_prefix as int (wrong type)",
			info: &schema.ConfigAndStacksInfo{
				ComponentBackendType: cfg.BackendTypeS3,
				ComponentBackendSection: map[string]any{
					"workspace_key_prefix": 123, // Wrong type, should be string.
				},
			},
			expectedErr: errUtils.ErrMissingTerraformWorkspaceKeyPrefix,
		},
		{
			name: "GCS backend with bucket as int (wrong type)",
			info: &schema.ConfigAndStacksInfo{
				ComponentBackendType: cfg.BackendTypeGCS,
				ComponentBackendSection: map[string]any{
					"bucket": 123, // Wrong type, should be string.
				},
			},
			expectedErr: errUtils.ErrGCSBucketRequired,
		},
		{
			name: "S3 backend with empty string workspace_key_prefix",
			info: &schema.ConfigAndStacksInfo{
				ComponentBackendType: cfg.BackendTypeS3,
				ComponentBackendSection: map[string]any{
					"workspace_key_prefix": "",
				},
			},
			expectedErr: nil, // Empty string is still a string type.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBackendTypeRequirements(tt.info)
			if tt.expectedErr != nil {
				assert.ErrorIs(t, err, tt.expectedErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
