package terraform_backend_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	e "github.com/cloudposse/atmos/internal/exec"
	tb "github.com/cloudposse/atmos/internal/terraform_backend"
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

	sections, err := e.ExecuteDescribeComponent(
		"component-1",
		stack,
		false,
		false,
		nil,
	)
	assert.NoError(t, err)
	backendInfo := tb.GetTerraformBackendInfo(sections)
	assert.Equal(t, cfg.BackendTypeLocal, backendInfo.Type)
	assert.Equal(t, stack, backendInfo.Workspace)

	sections, err = e.ExecuteDescribeComponent(
		"component-2",
		stack,
		false,
		false,
		nil,
	)
	assert.NoError(t, err)
	backendInfo = tb.GetTerraformBackendInfo(sections)
	assert.Equal(t, cfg.BackendTypeS3, backendInfo.Type)
	assert.Equal(t, "nonprod-tfstate", backendInfo.S3.Bucket)
	assert.Equal(t, "us-east-2", backendInfo.S3.Region)
	assert.Equal(t, "terraform.tfstate", backendInfo.S3.Key)
	assert.Equal(t, "arn:aws:iam::123456789123:role/nonprod-tfstate", backendInfo.S3.RoleArn)
	assert.Equal(t, "component-2", backendInfo.S3.WorkspaceKeyPrefix)

	sections, err = e.ExecuteDescribeComponent(
		"component-3",
		stack,
		false,
		false,
		nil,
	)
	assert.NoError(t, err)
	backendInfo = tb.GetTerraformBackendInfo(sections)
	assert.Equal(t, cfg.BackendTypeAzurerm, backendInfo.Type)

	sections, err = e.ExecuteDescribeComponent(
		"component-4",
		stack,
		false,
		false,
		nil,
	)
	assert.NoError(t, err)
	backendInfo = tb.GetTerraformBackendInfo(sections)
	assert.Equal(t, cfg.BackendTypeGCS, backendInfo.Type)

	sections, err = e.ExecuteDescribeComponent(
		"component-5",
		stack,
		false,
		false,
		nil,
	)
	assert.NoError(t, err)
	backendInfo = tb.GetTerraformBackendInfo(sections)
	assert.Equal(t, cfg.BackendTypeCloud, backendInfo.Type)
}
