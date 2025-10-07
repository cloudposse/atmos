package vender

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/tests"
)

func TestVendorComponentPullCommand(t *testing.T) {
	// Skip long tests in short mode (this test takes ~6 seconds due to network I/O)
	tests.SkipIfShort(t)

	// Initialize the CLI configuration
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	assert.Nil(t, err)

	atmosConfig.Logs.Level = "Trace"

	componentType := "terraform"

	// Helper function to ensure all paths are absolute
	ensureAbsPath := func(path string) string {
		absPath, err := filepath.Abs(path)
		assert.Nil(t, err)
		return absPath
	}

	// Test 'infra/vpc-flow-logs-bucket' component
	component := "infra/vpc-flow-logs-bucket"
	componentConfig, componentPath, err := e.ReadAndProcessComponentVendorConfigFile(&atmosConfig, component, componentType)
	assert.Nil(t, err)

	componentPath = ensureAbsPath(componentPath)

	err = e.ExecuteComponentVendorInternal(&atmosConfig, &componentConfig.Spec, component, componentPath, false)
	assert.Nil(t, err)

	// Check if the correct files were pulled and written to the correct folder
	filesToCheck := []string{
		"context.tf", "main.tf", "outputs.tf",
		"providers.tf", "variables.tf", "versions.tf",
	}

	for _, file := range filesToCheck {
		assert.FileExists(t, filepath.Join(componentPath, file))
	}

	// Test 'infra/account-map' component
	component = "infra/account-map"
	componentConfig, componentPath, err = e.ReadAndProcessComponentVendorConfigFile(&atmosConfig, component, componentType)
	assert.Nil(t, err)

	componentPath = ensureAbsPath(componentPath)

	err = e.ExecuteComponentVendorInternal(&atmosConfig, &componentConfig.Spec, component, componentPath, false)
	assert.Nil(t, err)

	// Additional files to check
	filesToCheck = append(filesToCheck,
		"dynamic-roles.tf", "README.md", "remote-state.tf",
		"modules/iam-roles/context.tf", "modules/iam-roles/main.tf",
		"modules/iam-roles/outputs.tf", "modules/iam-roles/variables.tf",
		"modules/roles-to-principals/context.tf", "modules/roles-to-principals/main.tf",
		"modules/roles-to-principals/outputs.tf", "modules/roles-to-principals/variables.tf",
	)

	for _, file := range filesToCheck {
		assert.FileExists(t, filepath.Join(componentPath, file))
	}

	// Delete the files and folders
	for _, file := range filesToCheck {
		err := os.Remove(filepath.Join(componentPath, file))
		if err != nil && !os.IsNotExist(err) {
			assert.Failf(t, "Failed to delete file", "Error: %v", err)
		}
	}

	err = os.RemoveAll(filepath.Join(componentPath, "modules"))
	assert.Nil(t, err)
}
