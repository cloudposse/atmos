package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestParsePlanDiffFlags(t *testing.T) {
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
			result, err := extractJSONFromOutput(tc.showOutput)

			if tc.expectError {
				assert.Error(t, err)
				if err != nil {
					assert.Equal(t, ErrNoJSONOutput, err)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedResult, result)
			}
		})
	}
}

func TestTerraformPlanDiff_FlagParsing(t *testing.T) {
	// Save the original OsExit function and restore it after the test
	originalOsExit := errUtils.OsExit
	defer func() { errUtils.OsExit = originalOsExit }()

	// Mock OsExit to prevent the test from exiting
	var exitCalled bool
	var exitCode int
	errUtils.OsExit = func(code int) {
		exitCalled = true
		exitCode = code
		// Don't actually exit
	}
	_ = exitCalled // Ensure variable is used
	_ = exitCode   // Ensure variable is used

	// Create test atmosphere configuration
	atmosConfig := &schema.AtmosConfiguration{
		TerraformDirAbsolutePath: "terraform",
	}

	// Test missing required flags
	info := &schema.ConfigAndStacksInfo{
		ComponentFolderPrefix:  "",
		FinalComponent:         "test-component",
		AdditionalArgsAndFlags: []string{
			// Missing --orig flag
		},
	}

	err := TerraformPlanDiff(atmosConfig, info)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "original plan file (--orig) is required")
}
