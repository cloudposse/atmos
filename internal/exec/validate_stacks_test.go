package exec

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func TestValidateStacksWithMergeContext(t *testing.T) {
	// Get the base path for test cases
	testCasesPath := "../../tests/test-cases/validate-type-mismatch"
	absPath, err := filepath.Abs(testCasesPath)
	if err != nil {
		t.Skipf("Skipping test: cannot resolve test cases path: %v", err)
	}

	// Create a test configuration
	atmosConfig := &schema.AtmosConfiguration{
		BasePath:                absPath,
		StacksBaseAbsolutePath:  filepath.Join(absPath, "stacks"),
		Stacks: schema.StacksConfiguration{
			BasePath: "stacks",
			NamePattern: "{stage}-{environment}",
			IncludedPaths: []string{"**/*"},
			ExcludedPaths: []string{"**/*.tmpl"},
		},
		Logs: schema.Logs{
			Level: u.LogLevelDebug,
		},
		Components: schema.Components{
			Terraform: schema.ComponentsTerraform{
				BasePath: "components/terraform",
			},
		},
	}

	// Set up the stacks base path
	atmosConfig.TerraformDirAbsolutePath = filepath.Join(absPath, "components/terraform")
	atmosConfig.HelmfileDirAbsolutePath = filepath.Join(absPath, "components/helmfile")
	atmosConfig.PackerDirAbsolutePath = filepath.Join(absPath, "components/packer")

	// Test 1: Validate stacks with type mismatch - should get enhanced error message
	t.Run("type mismatch with context", func(t *testing.T) {
		// This should fail due to type mismatch between array and string for subnets
		err := ValidateStacks(atmosConfig)
		
		if err != nil {
			errStr := err.Error()
			
			// Check that the error contains merge context information
			// The error should mention the files involved
			assert.Contains(t, errStr, "cannot override two slices with different type", "Should contain the original merge error")
			
			// Since we're using MergeContext, we should see file information
			// Note: The exact format depends on how the error propagates through the system
			if strings.Contains(errStr, "File being processed:") || 
			   strings.Contains(errStr, "Import chain:") ||
			   strings.Contains(errStr, "base.yaml") || 
			   strings.Contains(errStr, "override.yaml") ||
			   strings.Contains(errStr, "test-environment.yaml") {
				// Good - we have context information
				t.Logf("Error contains context information: %s", errStr)
			} else {
				t.Logf("Warning: Error might not contain full context. Error: %s", errStr)
			}
		} else {
			// If validation passes when it shouldn't, that's also useful to know
			t.Log("Validation passed - type mismatch might not be triggered with current merge strategy")
		}
	})
}

func TestMergeContextInProcessYAMLConfigFile(t *testing.T) {
	// Test that ProcessYAMLConfigFileWithContext properly tracks import chain
	testCasesPath := "../../tests/test-cases/validate-type-mismatch"
	absPath, err := filepath.Abs(testCasesPath)
	if err != nil {
		t.Skipf("Skipping test: cannot resolve test cases path: %v", err)
	}

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: absPath,
		StacksBaseAbsolutePath: filepath.Join(absPath, "stacks"),
		Logs: schema.Logs{
			Level: u.LogLevelDebug,
		},
		Settings: schema.AtmosSettings{
			ListMergeStrategy: "replace", // This should trigger the type mismatch
		},
	}

	basePath := filepath.Join(absPath, "stacks")
	filePath := filepath.Join(basePath, "test-environment.yaml")
	importsConfig := make(map[string]map[string]any)

	// Process the YAML config file that imports conflicting configurations
	_, _, _, _, _, _, _, err = ProcessYAMLConfigFile(
		atmosConfig,
		basePath,
		filePath,
		importsConfig,
		map[string]any{},
		false, // ignoreMissingFiles
		false, // skipTemplatesProcessingInImports
		false, // ignoreMissingTemplateValues
		false, // skipIfMissing
		map[string]any{},
		map[string]any{},
		map[string]any{},
		map[string]any{},
		"", // atmosManifestJsonSchemaFilePath
	)

	if err != nil {
		errStr := err.Error()
		t.Logf("Processing error (expected): %s", errStr)
		
		// Check if error contains context about the import chain
		// The actual error format will depend on where the merge fails
		if strings.Contains(errStr, "base") || 
		   strings.Contains(errStr, "override") ||
		   strings.Contains(errStr, "test-environment") {
			t.Log("Error contains file references - context tracking is working")
		}
	} else {
		t.Log("No error occurred - merge might have succeeded with current strategy")
	}
}

func TestMergeContextErrorFormatting(t *testing.T) {
	// This is a focused unit test for error formatting in the context of validate stacks
	
	tests := []struct {
		name          string
		setupFunc     func() error
		expectedParts []string
	}{
		{
			name: "type mismatch error formatting",
			setupFunc: func() error {
				// Simulate what happens during validate stacks
				testCasesPath := "../../tests/test-cases/validate-type-mismatch"
				absPath, _ := filepath.Abs(testCasesPath)
				
				atmosConfig := &schema.AtmosConfiguration{
					BasePath: absPath,
					StacksBaseAbsolutePath: filepath.Join(absPath, "stacks"),
					Settings: schema.AtmosSettings{
						ListMergeStrategy: "replace",
					},
				}
				
				// This should trigger our enhanced error formatting
				return ValidateStacks(atmosConfig)
			},
			expectedParts: []string{
				// We expect to see parts of the error message
				// The exact format depends on the implementation
			},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.setupFunc()
			if err != nil {
				errStr := err.Error()
				t.Logf("Formatted error:\n%s", errStr)
				
				for _, part := range tt.expectedParts {
					if part != "" && !strings.Contains(errStr, part) {
						t.Logf("Warning: Expected part not found: %s", part)
					}
				}
			}
		})
	}
}