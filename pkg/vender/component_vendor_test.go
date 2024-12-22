package vender

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestVendorComponentPullCommand(t *testing.T) {
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	assert.Nil(t, err)

	atmosConfig.Logs.Level = "Trace"

	componentType := "terraform"

	// Test 'infra/vpc-flow-logs-bucket' component
	component := "infra/vpc-flow-logs-bucket"
	componentConfig, componentPath, err := e.ReadAndProcessComponentVendorConfigFile(atmosConfig, component, componentType)
	assert.Nil(t, err)

	err = e.ExecuteComponentVendorInternal(atmosConfig, componentConfig.Spec, component, componentPath, false)
	assert.Nil(t, err)

	// Check if the correct files were pulled and written to the correct folder
	assert.FileExists(t, filepath.Join(componentPath, "context.tf"))
	assert.FileExists(t, filepath.Join(componentPath, "main.tf"))
	assert.FileExists(t, filepath.Join(componentPath, "outputs.tf"))
	assert.FileExists(t, filepath.Join(componentPath, "providers.tf"))
	assert.FileExists(t, filepath.Join(componentPath, "variables.tf"))
	assert.FileExists(t, filepath.Join(componentPath, "versions.tf"))

	// Test 'infra/account-map' component
	component = "infra/account-map"
	componentConfig, componentPath, err = e.ReadAndProcessComponentVendorConfigFile(atmosConfig, component, componentType)
	assert.Nil(t, err)

	err = e.ExecuteComponentVendorInternal(atmosConfig, componentConfig.Spec, component, componentPath, false)
	assert.Nil(t, err)

	// Check if the correct files were pulled and written to the correct folder
	filesToCheck := []string{
		"context.tf",
		"dynamic-roles.tf",
		"main.tf",
		"outputs.tf",
		"providers.tf",
		"README.md",
		"remote-state.tf",
		"variables.tf",
		"versions.tf",
	}

	for _, file := range filesToCheck {
		filePath := filepath.Join(componentPath, file)
		assert.FileExists(t, filePath)
	}

	// Check module files
	moduleFiles := map[string][]string{
		filepath.Join("modules", "iam-roles"): {
			"context.tf",
			"main.tf",
			"outputs.tf",
			"variables.tf",
		},
		filepath.Join("modules", "roles-to-principals"): {
			"context.tf",
			"main.tf",
			"outputs.tf",
			"variables.tf",
		},
	}

	for modulePath, files := range moduleFiles {
		for _, file := range files {
			filePath := filepath.Join(componentPath, modulePath, file)
			assert.FileExists(t, filePath)
		}
	}

	// Clean up files using a helper function that handles errors gracefully
	cleanupFiles := func(files []string, basePath string) {
		for _, file := range files {
			filePath := filepath.Join(basePath, file)
			if err := os.Remove(filePath); err != nil {
				if !os.IsNotExist(err) {
					t.Logf("Warning: Failed to remove file %s: %v", filePath, err)
				}
			}
		}
	}

	// Clean up main component files
	cleanupFiles(filesToCheck, componentPath)

	// Clean up module files
	for modulePath, files := range moduleFiles {
		fullModulePath := filepath.Join(componentPath, modulePath)
		cleanupFiles(files, fullModulePath)
		
		// Remove module directory after files are removed
		if err := os.RemoveAll(filepath.Join(componentPath, modulePath)); err != nil {
			t.Logf("Warning: Failed to remove module directory %s: %v", modulePath, err)
		}
	}

	// Finally remove the modules directory
	if err := os.RemoveAll(filepath.Join(componentPath, "modules")); err != nil {
		t.Logf("Warning: Failed to remove modules directory: %v", err)
	}
}
