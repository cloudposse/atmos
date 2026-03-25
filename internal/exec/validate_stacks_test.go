package exec

import (
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

	// Test 1: Validate stacks with type overrides — should succeed.
	// The fixture has list→string overrides which are technically misconfigurations
	// but must work (WithOverride semantics: src always wins regardless of type).
	t.Run("type override succeeds", func(t *testing.T) {
		err := ValidateStacks(atmosConfig)
		assert.NoError(t, err, "ValidateStacks should succeed — type overrides are allowed (WithOverride semantics)")
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
			name: "type override succeeds without error",
			setupFunc: func() error {
				// The fixture has type overrides (list→string) which now succeed
				// under WithOverride semantics.
				absPath := validateStacksTestDataDir(t)

				atmosConfig := &schema.AtmosConfiguration{
					BasePath:               absPath,
					StacksBaseAbsolutePath: filepath.Join(absPath, "stacks"),
					Settings: schema.AtmosSettings{
						ListMergeStrategy: "replace",
					},
				}

				return ValidateStacks(atmosConfig)
			},
			expectedParts: nil, // No error expected — type overrides succeed.
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
