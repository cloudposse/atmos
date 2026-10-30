package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/edition"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestParsePlanDiffFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		args         []string
		expectOrig   string
		expectNew    string
		expectError  bool
		errorMessage string
	}{
		{
			name:         "valid flags",
			args:         []string{"--orig=orig-plan.out", "--new=new-plan.out"},
			expectOrig:   "orig-plan.out",
			expectNew:    "new-plan.out",
			expectError:  false,
			errorMessage: "",
		},
		{
			name:         "valid flags with space",
			args:         []string{"--orig", "orig-plan.out", "--new", "new-plan.out"},
			expectOrig:   "orig-plan.out",
			expectNew:    "new-plan.out",
			expectError:  false,
			errorMessage: "",
		},
		{
			name:         "missing orig flag",
			args:         []string{"--new=new-plan.out"},
			expectOrig:   "",
			expectNew:    "",
			expectError:  true,
			errorMessage: "original plan file (--orig) is required",
		},
		{
			name:         "missing value for orig but next arg is a flag",
			args:         []string{"--orig", "--new=new-plan.out"},
			expectOrig:   "--new=new-plan.out",
			expectNew:    "new-plan.out",
			expectError:  false,
			errorMessage: "",
		},
		{
			name:         "missing new flag is ok",
			args:         []string{"--orig=orig-plan.out"},
			expectOrig:   "orig-plan.out",
			expectNew:    "",
			expectError:  false,
			errorMessage: "",
		},
		{
			name:         "extra flags are ignored",
			args:         []string{"--orig=orig-plan.out", "--new=new-plan.out", "--other=value"},
			expectOrig:   "orig-plan.out",
			expectNew:    "new-plan.out",
			expectError:  false,
			errorMessage: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			origPlan, newPlan, err := parsePlanDiffFlags(tc.args)

			if tc.expectError {
				assert.Error(t, err)
				if err != nil {
					assert.Contains(t, err.Error(), tc.errorMessage)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectOrig, origPlan)
				assert.Equal(t, tc.expectNew, newPlan)
			}
		})
	}
}

func TestValidateOriginalPlanFile(t *testing.T) {
	t.Parallel()

	// Create a temporary directory for test files
	tmpDir := t.TempDir()

	// Create component path
	componentPath := filepath.Join(tmpDir, "component")
	err := os.MkdirAll(componentPath, 0o755)
	require.NoError(t, err)

	// Create a plan file in the component directory
	relPlanFile := "plan.out"
	planFilePath := filepath.Join(componentPath, relPlanFile)
	err = os.WriteFile(planFilePath, []byte("mock plan"), 0o644)
	require.NoError(t, err)

	tests := []struct {
		name           string
		origPlanFile   string
		componentPath  string
		expectedResult string
		expectError    bool
	}{
		{
			name:           "absolute path exists",
			origPlanFile:   planFilePath,
			componentPath:  componentPath,
			expectedResult: planFilePath,
			expectError:    false,
		},
		{
			name:           "relative path exists",
			origPlanFile:   relPlanFile,
			componentPath:  componentPath,
			expectedResult: planFilePath,
			expectError:    false,
		},
		{
			name:           "absolute path does not exist",
			origPlanFile:   filepath.Join(tmpDir, "non-existent.out"),
			componentPath:  componentPath,
			expectedResult: "",
			expectError:    true,
		},
		{
			name:           "relative path does not exist",
			origPlanFile:   "non-existent.out",
			componentPath:  componentPath,
			expectedResult: "",
			expectError:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result, err := validateOriginalPlanFile(tc.origPlanFile, tc.componentPath)

			if tc.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "does not exist")
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedResult, result)
			}
		})
	}
}

func TestExtractJSONFromOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		showOutput     string
		expectError    bool
		expectedResult string
	}{
		{
			name: "valid JSON output",
			showOutput: `
Terraform show output before JSON
{
  "terraform_version": "1.0.0",
  "format_version": "0.2"
}
`,
			expectError:    false,
			expectedResult: "{\n  \"terraform_version\": \"1.0.0\",\n  \"format_version\": \"0.2\"\n}\n",
		},
		{
			name: "valid JSON with multiple braces",
			showOutput: `
Terraform show output
{
  "terraform_version": "1.0.0",
  "resource": {
    "nested": {
      "value": true
    }
  }
}
`,
			expectError:    false,
			expectedResult: "{\n  \"terraform_version\": \"1.0.0\",\n  \"resource\": {\n    \"nested\": {\n      \"value\": true\n    }\n  }\n}\n",
		},
		{
			name:           "no JSON in output",
			showOutput:     "Terraform show output without JSON",
			expectError:    true,
			expectedResult: "",
		},
		{
			name: "invalid JSON",
			showOutput: `
Terraform show output
{
  "terraform_version": "1.0.0",
  invalid json
}
`,
			expectError:    false,
			expectedResult: "{\n  \"terraform_version\": \"1.0.0\",\n  invalid json\n}\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result, err := extractJSONFromOutput(tc.showOutput)

			if tc.expectError {
				assert.Error(t, err)
				assert.ErrorIs(t, err, ErrNoJSONOutput)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedResult, result)
			}
		})
	}
}

func TestPlanDiffRejectsInvalidPolicyBeforeExecution(t *testing.T) {
	t.Parallel()
	config := &schema.AtmosConfiguration{}
	config.Components.Terraform.PlanDiff.IgnoreChanges = []string{"*.latest_restorable_time"}
	info := &schema.ConfigAndStacksInfo{}
	assert.ErrorIs(t, TerraformPlanDiff(config, info), schema.ErrInvalidPlanDiffIgnoreChange)
	assert.ErrorIs(t, comparePlansAndGenerateDiff(config, info, "", "", ""), schema.ErrInvalidPlanDiffIgnoreChange)
}

func TestPlanDiffRejectsInvalidEditionBeforeExecution(t *testing.T) {
	t.Parallel()
	config := &schema.AtmosConfiguration{Edition: "not-a-date"}
	info := &schema.ConfigAndStacksInfo{}
	assert.ErrorIs(t, TerraformPlanDiff(config, info), edition.ErrInvalidEdition)
	assert.ErrorIs(t, comparePlansAndGenerateDiff(config, info, "", "", ""), edition.ErrInvalidEdition)
}

func TestTerraformPlanDiffWithNonExistentFile(t *testing.T) {
	t.Parallel()

	// Test validateOriginalPlanFile directly as a unit test
	tmpDir := t.TempDir()

	// Create a component directory
	componentDir := filepath.Join(tmpDir, "test-component")
	err := os.MkdirAll(componentDir, 0o755)
	require.NoError(t, err)

	// Test with non-existent original plan file using a relative path
	// This should be resolved relative to the component directory
	relPath := "non-existent.plan"
	_, err = validateOriginalPlanFile(relPath, componentDir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not exist")
	// The error should mention the path relative to the component directory
	expectedNonExistentPath := filepath.Join(componentDir, relPath)
	assert.Contains(t, err.Error(), expectedNonExistentPath)

	// Test with non-existent original plan file using an absolute path
	// This should be used as-is
	absPath := filepath.Join(tmpDir, "another-non-existent.plan")
	_, err = validateOriginalPlanFile(absPath, componentDir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not exist")
	assert.Contains(t, err.Error(), absPath)

	// Test with an existing file
	existingFile := filepath.Join(componentDir, "existing.plan")
	err = os.WriteFile(existingFile, []byte("test"), 0o644)
	require.NoError(t, err)

	// Relative path should work
	result, err := validateOriginalPlanFile("existing.plan", componentDir)
	assert.NoError(t, err)
	assert.Equal(t, existingFile, result)

	// Absolute path should also work
	result, err = validateOriginalPlanFile(existingFile, componentDir)
	assert.NoError(t, err)
	assert.Equal(t, existingFile, result)
}

// TestTerraformPlanDiffErrorHandling tests the error handling in plan-diff helper functions.
func TestTerraformPlanDiffErrorHandling(t *testing.T) {
	t.Parallel()

	// Test extractJSONFromOutput with empty output (should return ErrNoJSONOutput)
	t.Run("empty_output_returns_ErrNoJSONOutput", func(t *testing.T) {
		t.Parallel()

		_, err := extractJSONFromOutput("")
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrNoJSONOutput)
	})

	// Test extractJSONFromOutput with no JSON content
	t.Run("no_json_content_returns_ErrNoJSONOutput", func(t *testing.T) {
		t.Parallel()

		_, err := extractJSONFromOutput("some random text without json")
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrNoJSONOutput)
	})

	// Test extractJSONFromOutput with valid JSON
	t.Run("valid_json_extracted", func(t *testing.T) {
		t.Parallel()

		output := `Some prefix text
{"key": "value"}
trailing text`
		result, err := extractJSONFromOutput(output)
		assert.NoError(t, err)
		assert.Contains(t, result, `"key": "value"`)
	})

	// Test parsePlanDiffFlags error handling
	t.Run("missing_orig_flag_returns_error", func(t *testing.T) {
		t.Parallel()

		_, _, err := parsePlanDiffFlags([]string{"--new=something.plan"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "original plan file (--orig) is required")
	})
}

func TestCopyPlanFileIfNeeded(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	componentDir := filepath.Join(tmpDir, "components", "terraform", "mycomponent")
	err := os.MkdirAll(componentDir, 0o755)
	require.NoError(t, err)

	// Create a plan file in the component directory
	planFileInComponent := filepath.Join(componentDir, "existing.plan")
	err = os.WriteFile(planFileInComponent, []byte("plan data"), 0o644)
	require.NoError(t, err)

	// Test 1: Plan file already in component directory - no copy needed
	t.Run("plan_file_in_component_dir", func(t *testing.T) {
		t.Parallel()

		result, cleanup, err := copyPlanFileIfNeeded(planFileInComponent, componentDir)
		assert.NoError(t, err)
		assert.Nil(t, cleanup)
		assert.Equal(t, planFileInComponent, result)
	})

	// Test 2: Plan file outside component directory - copy needed
	t.Run("plan_file_outside_component_dir", func(t *testing.T) {
		t.Parallel()

		externalPlanFile := filepath.Join(tmpDir, "external.plan")
		err := os.WriteFile(externalPlanFile, []byte("external plan data"), 0o644)
		require.NoError(t, err)

		result, cleanup, err := copyPlanFileIfNeeded(externalPlanFile, componentDir)
		assert.NoError(t, err)
		assert.NotNil(t, cleanup)
		assert.Equal(t, componentDir, filepath.Dir(result))
		assert.NotEqual(t, filepath.Join(componentDir, "external.plan"), result)

		// Verify the file was copied
		copiedData, err := os.ReadFile(result)
		assert.NoError(t, err)
		assert.Equal(t, "external plan data", string(copiedData))

		// Call cleanup and verify file is removed
		cleanup()
		_, err = os.Stat(result)
		assert.True(t, os.IsNotExist(err))
	})

	// Test 3: Non-existent source file
	t.Run("non_existent_source_file", func(t *testing.T) {
		t.Parallel()

		nonExistent := filepath.Join(tmpDir, "non-existent.plan")
		_, _, err := copyPlanFileIfNeeded(nonExistent, componentDir)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "error opening source plan file")
	})
}
