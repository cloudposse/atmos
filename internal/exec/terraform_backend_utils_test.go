package exec

import (
	errUtils "github.com/cloudposse/atmos/errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	cfg "github.com/cloudposse/atmos/pkg/config"
)

func TestGetTerraformBackendInfo(t *testing.T) {
	err := os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
	if err != nil {
		t.Fatalf("Failed to unset 'ATMOS_CLI_CONFIG_PATH': %v", err)
	}

	err = os.Unsetenv("ATMOS_BASE_PATH")
	if err != nil {
		t.Fatalf("Failed to unset 'ATMOS_BASE_PATH': %v", err)
	}

	stack := "nonprod"

	// Capture the starting working directory
	startingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get the current working directory: %v", err)
	}

	defer func() {
		// Change back to the original working directory after the test
		if err = os.Chdir(startingDir); err != nil {
			t.Fatalf("Failed to change back to the starting directory: %v", err)
		}
	}()

	// Define the working directory
	workDir := "../../tests/fixtures/scenarios/terraform-backend"
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("Failed to change directory to %q: %v", workDir, err)
	}

	sections, err := ExecuteDescribeComponent(
		"component-1",
		stack,
		false,
		false,
		nil,
	)
	assert.NoError(t, err)
	backendInfo := GetTerraformBackendInfo(sections)
	assert.Equal(t, cfg.BackendTypeLocal, backendInfo.Type)
	assert.Equal(t, stack, backendInfo.Workspace)

	sections, err = ExecuteDescribeComponent(
		"component-2",
		stack,
		false,
		false,
		nil,
	)
	assert.NoError(t, err)
	backendInfo = GetTerraformBackendInfo(sections)
	assert.Equal(t, cfg.BackendTypeS3, backendInfo.Type)
	assert.Equal(t, "nonprod-tfstate", backendInfo.S3.Bucket)
	assert.Equal(t, "us-east-2", backendInfo.S3.Region)
	assert.Equal(t, "terraform.tfstate", backendInfo.S3.Key)
	assert.Equal(t, "arn:aws:iam::123456789123:role/nonprod-tfstate", backendInfo.S3.RoleArn)
	assert.Equal(t, "component-2", backendInfo.S3.WorkspaceKeyPrefix)

	sections, err = ExecuteDescribeComponent(
		"component-3",
		stack,
		false,
		false,
		nil,
	)
	assert.NoError(t, err)
	backendInfo = GetTerraformBackendInfo(sections)
	assert.Equal(t, cfg.BackendTypeAzurerm, backendInfo.Type)

	sections, err = ExecuteDescribeComponent(
		"component-4",
		stack,
		false,
		false,
		nil,
	)
	assert.NoError(t, err)
	backendInfo = GetTerraformBackendInfo(sections)
	assert.Equal(t, cfg.BackendTypeGCS, backendInfo.Type)

	sections, err = ExecuteDescribeComponent(
		"component-5",
		stack,
		false,
		false,
		nil,
	)
	assert.NoError(t, err)
	backendInfo = GetTerraformBackendInfo(sections)
	assert.Equal(t, cfg.BackendTypeCloud, backendInfo.Type)
}

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
