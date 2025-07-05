package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
)

func TestGetTerraformBackendS3(t *testing.T) {
	tests := []struct {
		name        string
		backendInfo TerraformBackendInfo
		wantErr     bool
		errType     error
	}{
		{
			name: "valid backend info with role arn",
			backendInfo: TerraformBackendInfo{
				Type:      cfg.BackendTypeS3,
				Workspace: "test-workspace",
				S3: TerraformS3BackendInfo{
					Bucket:             "test-bucket",
					Region:             "us-east-2",
					Key:                "terraform.tfstate",
					RoleArn:            "arn:aws:iam::123456789012:role/test-role",
					WorkspaceKeyPrefix: "test-prefix",
				},
			},
			wantErr: true,
			errType: errUtils.ErrGetObjectFromS3,
		},
		{
			name: "valid backend info without role arn",
			backendInfo: TerraformBackendInfo{
				Type:      cfg.BackendTypeS3,
				Workspace: "test-workspace",
				S3: TerraformS3BackendInfo{
					Bucket:             "test-bucket",
					Region:             "us-east-2",
					Key:                "terraform.tfstate",
					WorkspaceKeyPrefix: "test-prefix",
				},
			},
			wantErr: true,
			errType: errUtils.ErrGetObjectFromS3,
		},
		{
			name: "invalid backend info - missing bucket",
			backendInfo: TerraformBackendInfo{
				Type:      cfg.BackendTypeS3,
				Workspace: "test-workspace",
				S3: TerraformS3BackendInfo{
					Region:             "us-east-2",
					Key:                "terraform.tfstate",
					WorkspaceKeyPrefix: "test-prefix",
				},
			},
			wantErr: true,
			errType: errUtils.ErrGetObjectFromS3,
		},
		{
			name: "invalid backend info - missing region",
			backendInfo: TerraformBackendInfo{
				Type:      cfg.BackendTypeS3,
				Workspace: "test-workspace",
				S3: TerraformS3BackendInfo{
					Bucket:             "test-bucket",
					Key:                "terraform.tfstate",
					WorkspaceKeyPrefix: "test-prefix",
				},
			},
			wantErr: true,
			errType: errUtils.ErrGetObjectFromS3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := GetTerraformBackendS3(tt.backendInfo)

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
