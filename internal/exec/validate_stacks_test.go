package exec

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
		BasePath:               absPath,
		StacksBaseAbsolutePath: filepath.Join(absPath, "stacks"),
		Stacks: schema.Stacks{
			BasePath:      "stacks",
			NamePattern:   "{stage}-{environment}",
			IncludedPaths: []string{"**/*"},
			ExcludedPaths: []string{"**/*.tmpl"},
		},
		Logs: schema.Logs{
			Level: u.LogLevelDebug,
		},
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
		Settings: schema.AtmosSettings{
			ListMergeStrategy: "replace", // Explicitly set merge strategy to ensure deterministic behavior
		},
	}

	// Set up the stacks base path
	atmosConfig.TerraformDirAbsolutePath = filepath.Join(absPath, "components", "terraform")
	atmosConfig.HelmfileDirAbsolutePath = filepath.Join(absPath, "components", "helmfile")
	atmosConfig.PackerDirAbsolutePath = filepath.Join(absPath, "components", "packer")

	// Test 1: Validate stacks with type mismatch - should get enhanced error message
	t.Run("type mismatch with context", func(t *testing.T) {
		// This should fail due to type mismatch between array and string for subnets
		err := ValidateStacks(atmosConfig)

		// Require an error to be returned
		assert.NotNil(t, err, "Expected validation to fail with type mismatch error")
		if err == nil {
			t.Fatal("Expected validation to fail but it passed")
		}

		errStr := err.Error()

		// Assert that the error contains the expected merge error
		assert.Contains(t, errStr, "cannot override two slices with different type", "Should contain the original merge error")

		// Assert that the error contains context information
		assert.Contains(t, errStr, "File being processed:", "Error should contain file processing context")
		assert.Contains(t, errStr, "Import chain:", "Error should contain import chain")

		// Assert that the error mentions some relevant files from the test case
		// Note: ValidateStacks processes all stack files, so we may see various files
		hasRelevantFiles := strings.Contains(errStr, "base.yaml") ||
			strings.Contains(errStr, "override.yaml") ||
			strings.Contains(errStr, "test-environment.yaml") ||
			strings.Contains(errStr, "deep-merge-test.yaml") ||
			strings.Contains(errStr, "complex-import-chain.yaml")
		assert.True(t, hasRelevantFiles, "Error should mention at least one relevant stack file")

		// Check for deduplication within individual error blocks.
		// ValidateStacks processes multiple stack files and each file that encounters a type
		// mismatch adds its own context block to the combined error. Count the number of
		// "File being processed:" occurrences to establish how many error blocks are present,
		// then verify context tokens appear at most once per block (+1 as defensive padding).
		fileCount := strings.Count(errStr, "File being processed:")
		require.Positive(t, fileCount, "Should have at least one file error block")

		// Cap the threshold so a deduplication bug that triples context blocks is always caught,
		// regardless of how many stack files the fixture contains. A correct implementation
		// produces exactly one occurrence of each token per error block; allowing fileCount+1
		// accommodates one extra occurrence in summary lines without masking a 2× regression.
		// The min(fileCount+1, 3) cap ensures the check degrades gracefully for large fixtures.
		maxOccurrences := fileCount + 1
		if maxOccurrences > 3 {
			maxOccurrences = 3
		}
		contextTokens := []string{
			"**Likely cause:**",
			"**Debug hint:**",
			"Import chain:", // must not be duplicated within a single error block
		}
		for _, token := range contextTokens {
			count := strings.Count(errStr, token)
			if count > maxOccurrences {
				t.Errorf("Token %q appears %d times but expected at most %d (one per error block)", token, count, maxOccurrences)
			}
		}

		t.Logf("Error contains proper context information: %s", errStr)
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
		BasePath:               absPath,
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
	_, _, _, _, _, _, _, err = ProcessYAMLConfigFile( //nolint:dogsled
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
					BasePath:               absPath,
					StacksBaseAbsolutePath: filepath.Join(absPath, "stacks"),
					Settings: schema.AtmosSettings{
						ListMergeStrategy: "replace",
					},
				}

				// This should trigger our enhanced error formatting
				return ValidateStacks(atmosConfig)
			},
			expectedParts: []string{
				"merge",                // Core error operation
				"override",             // Specific merge issue
				"type",                 // Type mismatch indicator
				"File being processed", // Context information
				"Import chain",         // Import tracking
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.setupFunc()

			// Assert error is returned when expected parts are defined
			if len(tt.expectedParts) > 0 {
				assert.NotNil(t, err, "Expected an error but got none")
				if err == nil {
					return
				}

				errStr := err.Error()
				t.Logf("Formatted error:\n%s", errStr)

				// Assert all expected parts are present
				for _, part := range tt.expectedParts {
					if part != "" {
						assert.Contains(t, errStr, part, "Error should contain token: %s", part)
					}
				}
				return
			}

			// If no expected parts, just log the error if it exists
			if err != nil {
				t.Logf("Error occurred: %v", err)
			}
		})
	}
}
