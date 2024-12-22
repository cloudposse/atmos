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
		if !assert.FileExists(t, filePath) {
			t.Logf("Failed to find file: %s", filePath)
			t.Logf("Component path: %s", componentPath)
			if files, err := os.ReadDir(componentPath); err == nil {
				t.Log("Available files:")
				for _, f := range files {
					t.Logf("  - %s", f.Name())
				}
			}
		}
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
		moduleDirPath := filepath.Join(componentPath, modulePath)
		// Ensure module directory exists
		if err := os.MkdirAll(moduleDirPath, 0755); err != nil {
			t.Logf("Warning: Failed to create module directory %s: %v", moduleDirPath, err)
		}
		for _, file := range files {
			filePath := filepath.Join(moduleDirPath, file)
			if !assert.FileExists(t, filePath) {
				t.Logf("Failed to find module file: %s", filePath)
				t.Logf("Module path: %s", moduleDirPath)
				if files, err := os.ReadDir(moduleDirPath); err == nil {
					t.Log("Available files in module:")
					for _, f := range files {
						t.Logf("  - %s", f.Name())
					}
				}
			}
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
		moduleDirPath := filepath.Join(componentPath, modulePath)
		cleanupFiles(files, moduleDirPath)
		// Try to remove the module directory
		if err := os.RemoveAll(moduleDirPath); err != nil {
			t.Logf("Warning: Failed to remove module directory %s: %v", moduleDirPath, err)
		}
	}
}
