package exec

import (
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
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
