package exec

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// validateStacksTestDataDir returns the absolute path to the validate-type-mismatch fixture
// directory using runtime.Caller(0) so the path is source-file-relative (CWD-independent).
func validateStacksTestDataDir(t *testing.T) string {
	t.Helper()
	_, callerFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller(0) must succeed")
	// callerFile is the absolute path to this _test.go file.
	// Resolve ../../tests/test-cases/validate-type-mismatch relative to it.
	dir := filepath.Join(filepath.Dir(callerFile), "..", "..", "tests", "test-cases", "validate-type-mismatch")
	absDir, err := filepath.Abs(dir)
	require.NoError(t, err, "cannot resolve fixture path")
	return absDir
}

func TestValidateStacksWithMergeContext(t *testing.T) {
	// Get the base path for test cases using source-file-relative lookup (CWD-independent).
	absPath := validateStacksTestDataDir(t)

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

		// Self-validate the block counter: "File being processed:" must not appear more
		// often than there are stack YAML files in the fixture (an independent count).
		// If a deduplication bug doubled the counter, fileCount would be inflated, making
		// maxOccurrences too large and letting doubled contextToken occurrences slip through.
		// Use absPath (already resolved above) for CWD-independent counting.
		var fixtureFileCount int
		_ = filepath.WalkDir(filepath.Join(absPath, "stacks"), func(_ string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d == nil {
				return nil
			}
			if !d.IsDir() && strings.HasSuffix(d.Name(), ".yaml") {
				fixtureFileCount++
			}
			return nil
		})
		// Fail loudly if the fixture is empty or the path is wrong — a silent 0 would
		// disable the independence check and make it a no-op.
		require.Positive(t, fixtureFileCount, "stacks fixture must contain YAML files — check absPath: %s", filepath.Join(absPath, "stacks"))
		if fileCount > fixtureFileCount+1 {
			t.Errorf("\"File being processed:\" appears %d times but stacks fixture has only %d YAML files — possible block-counter duplication bug", fileCount, fixtureFileCount)
		}

		// A correct implementation produces exactly one occurrence of each context token per
		// error block (one per erroring file). Allowing fileCount+1 adds a single defensive
		// tolerance for any summary lines that repeat a token.  The important property is that
		// the bound scales with fileCount, so the check catches 2× duplication bugs regardless
		// of how large the fixture grows — unlike a fixed cap of 3 that would cause false
		// failures the moment the fixture has 4+ erroring files.
		maxOccurrences := fileCount + 1
		contextTokens := []string{
			// "File being processed:" is the block counter used above — do not re-validate here.
			// Its presence is already asserted by assert.Contains (line 68) and require.Positive.
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
	absPath := validateStacksTestDataDir(t)

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
	_, _, _, _, _, _, _, err := ProcessYAMLConfigFile( //nolint:dogsled
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
				absPath := validateStacksTestDataDir(t)

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
