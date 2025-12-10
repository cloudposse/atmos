package exec

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mvdan.cc/sh/v3/shell"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestIsKnownWorkflowError tests the IsKnownWorkflowError function.
func TestIsKnownWorkflowError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "ErrWorkflowNoSteps",
			err:      ErrWorkflowNoSteps,
			expected: true,
		},
		{
			name:     "ErrInvalidWorkflowStepType",
			err:      ErrInvalidWorkflowStepType,
			expected: true,
		},
		{
			name:     "ErrInvalidFromStep",
			err:      ErrInvalidFromStep,
			expected: true,
		},
		{
			name:     "ErrWorkflowStepFailed",
			err:      ErrWorkflowStepFailed,
			expected: true,
		},
		{
			name:     "ErrWorkflowNoWorkflow",
			err:      ErrWorkflowNoWorkflow,
			expected: true,
		},
		{
			name:     "ErrWorkflowFileNotFound",
			err:      ErrWorkflowFileNotFound,
			expected: true,
		},
		{
			name:     "ErrInvalidWorkflowManifest",
			err:      ErrInvalidWorkflowManifest,
			expected: true,
		},
		{
			name:     "wrapped known error",
			err:      errors.Join(ErrWorkflowNoSteps, errors.New("additional context")),
			expected: true,
		},
		{
			name:     "unknown error",
			err:      errors.New("some random error"),
			expected: false,
		},
		{
			name:     "wrapped unknown error",
			err:      errors.Join(errors.New("unknown"), errors.New("more context")),
			expected: false,
		},
		{
			name:     "ExitCodeError is known",
			err:      errUtils.ExitCodeError{Code: 1},
			expected: true,
		},
		{
			name:     "wrapped ExitCodeError is known",
			err:      errors.Join(errors.New("wrapper"), errUtils.ExitCodeError{Code: 2}),
			expected: true,
		},
		{
			name: "ErrorBuilder wrapped error is known",
			err: errUtils.Build(ErrWorkflowNoSteps).
				WithExplanationf("Workflow %s is empty", "test").
				Err(),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsKnownWorkflowError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestFormatList tests the FormatList function.
func TestFormatList(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected string
	}{
		{
			name:     "empty list",
			input:    []string{},
			expected: "",
		},
		{
			name:     "single item",
			input:    []string{"item1"},
			expected: "- `item1`\n",
		},
		{
			name:     "multiple items",
			input:    []string{"item1", "item2", "item3"},
			expected: "- `item1`\n- `item2`\n- `item3`\n",
		},
		{
			name:     "items with spaces",
			input:    []string{"item with spaces", "another item"},
			expected: "- `item with spaces`\n- `another item`\n",
		},
		{
			name:     "items with special characters",
			input:    []string{"item-1", "item_2", "item.3"},
			expected: "- `item-1`\n- `item_2`\n- `item.3`\n",
		},
		{
			name:     "items with backticks in name",
			input:    []string{"item`with`backticks"},
			expected: "- `item`with`backticks`\n",
		},
		{
			name:     "empty string item",
			input:    []string{""},
			expected: "- ``\n",
		},
		{
			name:     "mixed empty and non-empty items",
			input:    []string{"item1", "", "item3"},
			expected: "- `item1`\n- ``\n- `item3`\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatList(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestCheckAndMergeDefaultIdentity tests the checkAndMergeDefaultIdentity function.
func TestCheckAndMergeDefaultIdentity(t *testing.T) {
	tests := []struct {
		name           string
		atmosConfig    *schema.AtmosConfiguration
		expectedResult bool
	}{
		{
			name: "no identities configured",
			atmosConfig: &schema.AtmosConfiguration{
				Auth: schema.AuthConfig{
					Identities: map[string]schema.Identity{},
				},
			},
			expectedResult: false,
		},
		{
			name: "identities without default",
			atmosConfig: &schema.AtmosConfiguration{
				Auth: schema.AuthConfig{
					Identities: map[string]schema.Identity{
						"test-identity": {
							Kind:    "aws/assume-role",
							Default: false,
						},
					},
				},
			},
			expectedResult: false,
		},
		{
			name: "identity with default true",
			atmosConfig: &schema.AtmosConfiguration{
				Auth: schema.AuthConfig{
					Identities: map[string]schema.Identity{
						"test-identity": {
							Kind:    "aws/assume-role",
							Default: true,
						},
					},
				},
			},
			expectedResult: true,
		},
		{
			name: "multiple identities one with default",
			atmosConfig: &schema.AtmosConfiguration{
				Auth: schema.AuthConfig{
					Identities: map[string]schema.Identity{
						"identity-1": {
							Kind:    "aws/assume-role",
							Default: false,
						},
						"identity-2": {
							Kind:    "aws/assume-role",
							Default: true,
						},
					},
				},
			},
			expectedResult: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := checkAndMergeDefaultIdentity(tt.atmosConfig)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

// TestCheckAndMergeDefaultIdentity_WithStackLoading tests checkAndMergeDefaultIdentity with stack file loading.
func TestCheckAndMergeDefaultIdentity_WithStackLoading(t *testing.T) {
	// Create a temporary directory with stack files.
	tmpDir := t.TempDir()

	// Create a stack file with default identity.
	stacksDir := filepath.Join(tmpDir, "stacks")
	err := os.MkdirAll(stacksDir, 0o755)
	assert.NoError(t, err)

	stackContent := `auth:
  identities:
    stack-default-identity:
      default: true
`
	err = os.WriteFile(filepath.Join(stacksDir, "_defaults.yaml"), []byte(stackContent), 0o644)
	assert.NoError(t, err)

	// Create atmos config with identity but no default.
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tmpDir,
		Auth: schema.AuthConfig{
			Identities: map[string]schema.Identity{
				"stack-default-identity": {
					Kind:    "aws/assume-role",
					Default: false, // Not default in atmos.yaml.
				},
			},
		},
		IncludeStackAbsolutePaths: []string{filepath.Join(stacksDir, "*.yaml")},
	}

	// checkAndMergeDefaultIdentity should load stack files and find the default.
	result := checkAndMergeDefaultIdentity(atmosConfig)
	assert.True(t, result)

	// Verify that the identity was updated to have default=true.
	identity, exists := atmosConfig.Auth.Identities["stack-default-identity"]
	assert.True(t, exists)
	assert.True(t, identity.Default)
}

// TestCheckAndMergeDefaultIdentity_LoadError tests behavior when stack loading fails.
func TestCheckAndMergeDefaultIdentity_LoadError(t *testing.T) {
	// Create config with invalid include paths (will cause load to return error).
	atmosConfig := &schema.AtmosConfiguration{
		Auth: schema.AuthConfig{
			Identities: map[string]schema.Identity{
				"test-identity": {
					Kind:    "aws/assume-role",
					Default: true, // Has default in atmos.yaml.
				},
			},
		},
		// Invalid path that will cause glob to return error.
		IncludeStackAbsolutePaths: []string{"/nonexistent/path/[invalid/glob"},
	}

	// Should still return true because atmos.yaml has a default.
	result := checkAndMergeDefaultIdentity(atmosConfig)
	assert.True(t, result)
}

// TestCheckAndMergeDefaultIdentity_LoadErrorNoDefault tests behavior when stack loading fails and no default in atmos.yaml.
func TestCheckAndMergeDefaultIdentity_LoadErrorNoDefault(t *testing.T) {
	// Create config with invalid include paths and no default in atmos.yaml.
	atmosConfig := &schema.AtmosConfiguration{
		Auth: schema.AuthConfig{
			Identities: map[string]schema.Identity{
				"test-identity": {
					Kind:    "aws/assume-role",
					Default: false, // No default in atmos.yaml.
				},
			},
		},
		// Invalid path that will cause glob to return error.
		IncludeStackAbsolutePaths: []string{"/nonexistent/path/[invalid/glob"},
	}

	// Should return false because no default anywhere.
	result := checkAndMergeDefaultIdentity(atmosConfig)
	assert.False(t, result)
}

// TestCheckAndMergeDefaultIdentity_StackNoDefaults tests with stack files that have no defaults.
func TestCheckAndMergeDefaultIdentity_StackNoDefaults(t *testing.T) {
	// Create a temporary directory with stack files that have no defaults.
	tmpDir := t.TempDir()

	// Create a stack file without default identity.
	stacksDir := filepath.Join(tmpDir, "stacks")
	err := os.MkdirAll(stacksDir, 0o755)
	assert.NoError(t, err)

	stackContent := `auth:
  identities:
    some-identity:
      kind: aws/assume-role
`
	err = os.WriteFile(filepath.Join(stacksDir, "_defaults.yaml"), []byte(stackContent), 0o644)
	assert.NoError(t, err)

	// Create atmos config with identity but no default.
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tmpDir,
		Auth: schema.AuthConfig{
			Identities: map[string]schema.Identity{
				"test-identity": {
					Kind:    "aws/assume-role",
					Default: false,
				},
			},
		},
		IncludeStackAbsolutePaths: []string{filepath.Join(stacksDir, "*.yaml")},
	}

	// Should return false because no default in either atmos.yaml or stack configs.
	result := checkAndMergeDefaultIdentity(atmosConfig)
	assert.False(t, result)
}

// TestCheckAndMergeDefaultIdentity_EmptyStackDefaults tests with empty stack defaults.
func TestCheckAndMergeDefaultIdentity_EmptyStackDefaults(t *testing.T) {
	// Create a temporary directory with empty stack files.
	tmpDir := t.TempDir()

	// Create an empty stack file.
	stacksDir := filepath.Join(tmpDir, "stacks")
	err := os.MkdirAll(stacksDir, 0o755)
	assert.NoError(t, err)

	stackContent := `# Empty stack file
`
	err = os.WriteFile(filepath.Join(stacksDir, "_defaults.yaml"), []byte(stackContent), 0o644)
	assert.NoError(t, err)

	// Create atmos config with identity but no default.
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tmpDir,
		Auth: schema.AuthConfig{
			Identities: map[string]schema.Identity{
				"test-identity": {
					Kind:    "aws/assume-role",
					Default: false,
				},
			},
		},
		IncludeStackAbsolutePaths: []string{filepath.Join(stacksDir, "*.yaml")},
	}

	// Should return false because no default anywhere.
	result := checkAndMergeDefaultIdentity(atmosConfig)
	assert.False(t, result)
}

// TestKnownWorkflowErrorsSlice tests that the KnownWorkflowErrors slice is properly defined.
func TestKnownWorkflowErrorsSlice(t *testing.T) {
	// Verify all expected errors are in the slice.
	expectedErrors := []error{
		ErrWorkflowNoSteps,
		ErrInvalidWorkflowStepType,
		ErrInvalidFromStep,
		ErrWorkflowStepFailed,
		ErrWorkflowNoWorkflow,
		ErrWorkflowFileNotFound,
		ErrInvalidWorkflowManifest,
	}

	assert.Equal(t, len(expectedErrors), len(KnownWorkflowErrors))

	for _, expected := range expectedErrors {
		found := false
		for _, actual := range KnownWorkflowErrors {
			if errors.Is(expected, actual) {
				found = true
				break
			}
		}
		assert.True(t, found, "Expected error %v to be in KnownWorkflowErrors", expected)
	}
}

// TestWorkflowErrTitle tests the error title constant.
func TestWorkflowErrTitle(t *testing.T) {
	assert.Equal(t, "Workflow Error", WorkflowErrTitle)
}

// TestStringsFieldsQuotedArguments documents the historical issue with strings.Fields().
// It demonstrates why shell.Fields() from mvdan.cc/sh is used instead for workflow commands.
// Multi-component flags like --query with quoted expressions were incorrectly split.
func TestStringsFieldsQuotedArguments(t *testing.T) {
	// This test documents why strings.Fields() is NOT used for workflow command parsing.
	// The production code in pkg/workflow/executor.go uses shell.Fields() instead.
	command := "terraform plan --query '.metadata.component == \"gcp/compute/v0.2.0\"' -s dev"

	// strings.Fields incorrectly splits quoted expressions.
	stringsFieldsResult := strings.Fields(command)

	// The quoted expression is broken into multiple parts (incorrect).
	assert.Contains(t, stringsFieldsResult, "'.metadata.component")
	assert.Contains(t, stringsFieldsResult, "==")
	assert.Contains(t, stringsFieldsResult, "\"gcp/compute/v0.2.0\"'")

	// shell.Fields correctly preserves quoted expressions.
	shellFieldsResult, err := shell.Fields(command, nil)
	assert.NoError(t, err)

	// The quoted expression is preserved as a single argument (correct).
	assert.Contains(t, shellFieldsResult, ".metadata.component == \"gcp/compute/v0.2.0\"")
}

// TestShellFieldsCorrectParsing verifies that shell.Fields() from mvdan.cc/sh correctly parses.
// It tests workflow commands with quoted arguments that would break with strings.Fields().
// This is the fix for the multi-component operation issue.
func TestShellFieldsCorrectParsing(t *testing.T) {
	tests := []struct {
		name         string
		command      string
		expectedArgs []string
	}{
		{
			name:    "query with single-quoted expression containing spaces",
			command: "terraform plan --query '.metadata.component == \"gcp/compute/v0.2.0\"' -s dev",
			expectedArgs: []string{
				"terraform", "plan",
				"--query", ".metadata.component == \"gcp/compute/v0.2.0\"",
				"-s", "dev",
			},
		},
		{
			name:    "query with double-quoted expression containing spaces",
			command: `terraform plan --query ".settings.enabled == true" -s nonprod`,
			expectedArgs: []string{
				"terraform", "plan",
				"--query", ".settings.enabled == true",
				"-s", "nonprod",
			},
		},
		{
			name:    "components with comma-separated values (no spaces)",
			command: "terraform plan --components gcp/compute/001,gcp/compute/101 -s dev",
			expectedArgs: []string{
				"terraform", "plan",
				"--components", "gcp/compute/001,gcp/compute/101",
				"-s", "dev",
			},
		},
		{
			name:    "components with spaces after commas (quoted)",
			command: `terraform plan --components "gcp/compute/001, gcp/compute/101" -s dev`,
			expectedArgs: []string{
				"terraform", "plan",
				"--components", "gcp/compute/001, gcp/compute/101",
				"-s", "dev",
			},
		},
		{
			name:    "simple command without quotes",
			command: "terraform plan vpc -s dev",
			expectedArgs: []string{
				"terraform", "plan", "vpc", "-s", "dev",
			},
		},
		{
			name:    "all flag (no arguments)",
			command: "terraform plan --all -s dev",
			expectedArgs: []string{
				"terraform", "plan", "--all", "-s", "dev",
			},
		},
		{
			name:    "complex query with nested quotes",
			command: `terraform plan --query '.metadata.component == "mock" and .settings.enabled == true' -s nonprod`,
			expectedArgs: []string{
				"terraform", "plan",
				"--query", ".metadata.component == \"mock\" and .settings.enabled == true",
				"-s", "nonprod",
			},
		},
		{
			name:    "command with double-dash separator",
			command: "terraform plan vpc -s dev -- -var foo=bar",
			expectedArgs: []string{
				"terraform", "plan", "vpc", "-s", "dev", "--", "-var", "foo=bar",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use shell.Fields() - the same function now used in workflow_utils.go
			actualArgs, err := shell.Fields(tt.command, nil)
			assert.NoError(t, err, "shell.Fields should not return an error")
			assert.Equal(t, tt.expectedArgs, actualArgs,
				"shell.Fields should correctly parse quoted arguments")
		})
	}
}

// TestBuildWorkflowStepError tests the buildWorkflowStepError function.
func TestBuildWorkflowStepError(t *testing.T) {
	tests := []struct {
		name            string
		err             error
		ctx             *workflowStepErrorContext
		expectContains  []string
		expectResumeCmd bool
		expectSentinel  error
		expectExitCode  int
	}{
		{
			name: "shell command failure with stack",
			err:  errors.New("command failed"),
			ctx: &workflowStepErrorContext{
				WorkflowPath:     "/path/to/stacks/workflows/deploy.yaml",
				WorkflowBasePath: "/path/to/stacks/workflows",
				Workflow:         "deploy-all",
				StepName:         "step1",
				Command:          "echo hello",
				CommandType:      "shell",
				FinalStack:       "dev-us-east-1",
			},
			expectContains:  []string{"deploy-all", "step1", "echo hello", "dev-us-east-1"},
			expectResumeCmd: true,
			expectSentinel:  ErrWorkflowStepFailed,
		},
		{
			name: "atmos command failure without stack",
			err:  errors.New("terraform failed"),
			ctx: &workflowStepErrorContext{
				WorkflowPath:     "/base/stacks/workflows/infra.yaml",
				WorkflowBasePath: "/base/stacks/workflows",
				Workflow:         "provision",
				StepName:         "terraform-step",
				Command:          "terraform plan vpc",
				CommandType:      "atmos",
				FinalStack:       "",
			},
			expectContains:  []string{"provision", "terraform-step", "atmos terraform plan vpc"},
			expectResumeCmd: true,
			expectSentinel:  ErrWorkflowStepFailed,
		},
		{
			name: "atmos command failure with stack",
			err:  errors.New("plan failed"),
			ctx: &workflowStepErrorContext{
				WorkflowPath:     "/workflows/deploy.yaml",
				WorkflowBasePath: "/workflows",
				Workflow:         "deploy",
				StepName:         "plan-step",
				Command:          "terraform plan mycomponent",
				CommandType:      "atmos",
				FinalStack:       "prod",
			},
			expectContains:  []string{"deploy", "plan-step", "atmos", "prod"},
			expectResumeCmd: true,
			expectSentinel:  ErrWorkflowStepFailed,
		},
		{
			name: "error with exit code",
			err:  errUtils.WithExitCode(errors.New("exit error"), 2),
			ctx: &workflowStepErrorContext{
				WorkflowPath:     "/workflows/test.yaml",
				WorkflowBasePath: "/workflows",
				Workflow:         "test-wf",
				StepName:         "failing-step",
				Command:          "exit 2",
				CommandType:      "shell",
				FinalStack:       "",
			},
			expectContains:  []string{"test-wf", "failing-step"},
			expectResumeCmd: true,
			expectSentinel:  ErrWorkflowStepFailed,
			expectExitCode:  2,
		},
		{
			name: "workflow file in nested directory",
			err:  errors.New("nested failure"),
			ctx: &workflowStepErrorContext{
				WorkflowPath:     "/base/stacks/workflows/team/project/deploy.yaml",
				WorkflowBasePath: "/base/stacks/workflows",
				Workflow:         "deploy",
				StepName:         "step1",
				Command:          "echo test",
				CommandType:      "shell",
				FinalStack:       "",
			},
			expectContains:  []string{"deploy", "step1", "team/project/deploy"},
			expectResumeCmd: true,
			expectSentinel:  ErrWorkflowStepFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildWorkflowStepError(tt.err, tt.ctx)

			assert.Error(t, result)
			assert.ErrorIs(t, result, tt.expectSentinel)

			// Use Format to get the full formatted error including hints.
			formattedErr := errUtils.Format(result, errUtils.DefaultFormatterConfig())
			for _, expected := range tt.expectContains {
				assert.Contains(t, formattedErr, expected)
			}

			if tt.expectExitCode > 0 {
				exitCode := errUtils.GetExitCode(result)
				assert.Equal(t, tt.expectExitCode, exitCode)
			}
		})
	}
}

// TestPromptForWorkflowFile_EmptyMatches tests promptForWorkflowFile with no matches.
func TestPromptForWorkflowFile_EmptyMatches(t *testing.T) {
	result, err := promptForWorkflowFile([]WorkflowMatch{})

	assert.ErrorIs(t, err, ErrNoWorkflowFilesToSelect)
	assert.Empty(t, result)
}

// TestPromptForWorkflowFile_NonTTY tests promptForWorkflowFile in non-TTY environment.
func TestPromptForWorkflowFile_NonTTY(t *testing.T) {
	// In CI/test environment, IsTTYSupportForStdin returns false.
	matches := []WorkflowMatch{
		{File: "deploy.yaml", Name: "deploy", Description: "Deploy workflow"},
		{File: "test.yaml", Name: "deploy", Description: "Test workflow"},
	}

	result, err := promptForWorkflowFile(matches)

	// Should get non-TTY error since tests run without TTY.
	assert.ErrorIs(t, err, ErrNonTTYWorkflowSelection)
	assert.Empty(t, result)
}

// TestFindWorkflowAcrossFiles_Coverage tests the findWorkflowAcrossFiles function with additional coverage.
func TestFindWorkflowAcrossFiles_Coverage(t *testing.T) {
	// Set up test fixture.
	stacksPath := "../../tests/fixtures/scenarios/workflows"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)

	configInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configInfo, false)
	require.NoError(t, err)

	tests := []struct {
		name          string
		workflowName  string
		expectMatches int
		expectError   bool
	}{
		{
			name:          "find existing workflow",
			workflowName:  "shell-pass",
			expectMatches: 1,
			expectError:   false,
		},
		{
			name:          "workflow not found",
			workflowName:  "nonexistent-workflow",
			expectMatches: 0,
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches, err := findWorkflowAcrossFiles(tt.workflowName, &atmosConfig)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Len(t, matches, tt.expectMatches)
			}
		})
	}
}

// TestExecuteDescribeWorkflows_Coverage tests the ExecuteDescribeWorkflows function with additional coverage.
func TestExecuteDescribeWorkflows_Coverage(t *testing.T) {
	t.Run("with valid workflow directory", func(t *testing.T) {
		stacksPath := "../../tests/fixtures/scenarios/workflows"
		t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
		t.Setenv("ATMOS_BASE_PATH", stacksPath)

		configInfo := schema.ConfigAndStacksInfo{}
		atmosConfig, err := cfg.InitCliConfig(configInfo, false)
		require.NoError(t, err)

		listResult, mapResult, allResult, err := ExecuteDescribeWorkflows(atmosConfig)

		assert.NoError(t, err)
		assert.NotEmpty(t, listResult)
		assert.NotEmpty(t, mapResult)
		assert.NotEmpty(t, allResult)
	})

	t.Run("with empty workflows base path", func(t *testing.T) {
		atmosConfig := schema.AtmosConfiguration{
			Workflows: schema.Workflows{
				BasePath: "",
			},
		}

		_, _, _, err := ExecuteDescribeWorkflows(atmosConfig)

		assert.ErrorIs(t, err, errUtils.ErrWorkflowBasePathNotConfigured)
	})

	t.Run("with nonexistent workflow directory", func(t *testing.T) {
		atmosConfig := schema.AtmosConfiguration{
			BasePath: "/nonexistent/path",
			Workflows: schema.Workflows{
				BasePath: "workflows",
			},
		}

		_, _, _, err := ExecuteDescribeWorkflows(atmosConfig)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "does not exist")
	})

	t.Run("with absolute workflow path", func(t *testing.T) {
		tmpDir := t.TempDir()
		workflowsDir := filepath.Join(tmpDir, "workflows")
		err := os.MkdirAll(workflowsDir, 0o755)
		require.NoError(t, err)

		// Create a valid workflow file.
		workflowContent := `workflows:
  test-workflow:
    description: Test workflow
    steps:
      - name: step1
        command: echo hello
        type: shell
`
		err = os.WriteFile(filepath.Join(workflowsDir, "test.yaml"), []byte(workflowContent), 0o644)
		require.NoError(t, err)

		atmosConfig := schema.AtmosConfiguration{
			Workflows: schema.Workflows{
				BasePath: workflowsDir,
			},
		}

		listResult, mapResult, allResult, err := ExecuteDescribeWorkflows(atmosConfig)

		assert.NoError(t, err)
		assert.NotEmpty(t, listResult)
		assert.NotEmpty(t, mapResult)
		assert.NotEmpty(t, allResult)

		// Verify the workflow was found.
		found := false
		for _, item := range listResult {
			if item.Workflow == "test-workflow" {
				found = true
				break
			}
		}
		assert.True(t, found, "Expected test-workflow to be found")
	})
}

// TestCheckAndGenerateWorkflowStepNames_Coverage tests the checkAndGenerateWorkflowStepNames function with additional coverage.
func TestCheckAndGenerateWorkflowStepNames_Coverage(t *testing.T) {
	tests := []struct {
		name          string
		input         *schema.WorkflowDefinition
		expectedNames []string
	}{
		{
			name: "nil steps",
			input: &schema.WorkflowDefinition{
				Steps: nil,
			},
			expectedNames: nil,
		},
		{
			name: "empty steps",
			input: &schema.WorkflowDefinition{
				Steps: []schema.WorkflowStep{},
			},
			expectedNames: []string{},
		},
		{
			name: "steps with existing names",
			input: &schema.WorkflowDefinition{
				Steps: []schema.WorkflowStep{
					{Name: "my-step-1", Command: "echo 1"},
					{Name: "my-step-2", Command: "echo 2"},
				},
			},
			expectedNames: []string{"my-step-1", "my-step-2"},
		},
		{
			name: "steps without names",
			input: &schema.WorkflowDefinition{
				Steps: []schema.WorkflowStep{
					{Command: "echo 1"},
					{Command: "echo 2"},
					{Command: "echo 3"},
				},
			},
			expectedNames: []string{"step1", "step2", "step3"},
		},
		{
			name: "mixed steps",
			input: &schema.WorkflowDefinition{
				Steps: []schema.WorkflowStep{
					{Name: "custom", Command: "echo 1"},
					{Command: "echo 2"},
					{Name: "another-custom", Command: "echo 3"},
					{Command: "echo 4"},
				},
			},
			expectedNames: []string{"custom", "step2", "another-custom", "step4"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checkAndGenerateWorkflowStepNames(tt.input)

			if tt.expectedNames == nil {
				assert.Nil(t, tt.input.Steps)
			} else {
				assert.Len(t, tt.input.Steps, len(tt.expectedNames))
				for i, expectedName := range tt.expectedNames {
					assert.Equal(t, expectedName, tt.input.Steps[i].Name)
				}
			}
		})
	}
}

// TestPrepareStepEnvironment tests the prepareStepEnvironment function.
func TestPrepareStepEnvironment_NoIdentity(t *testing.T) {
	// When no identity is specified, should return empty slice.
	env, err := prepareStepEnvironment("", "step1", nil)

	assert.NoError(t, err)
	assert.Empty(t, env)
}

// TestPrepareStepEnvironment_NilAuthManager tests prepareStepEnvironment with nil auth manager.
func TestPrepareStepEnvironment_NilAuthManager(t *testing.T) {
	env, err := prepareStepEnvironment("some-identity", "step1", nil)

	assert.ErrorIs(t, err, errUtils.ErrAuthManager)
	assert.Nil(t, env)
}

// TestWorkflowMatch tests the WorkflowMatch struct.
func TestWorkflowMatch(t *testing.T) {
	match := WorkflowMatch{
		File:        "deploy.yaml",
		Name:        "deploy-all",
		Description: "Deploy all components",
	}

	assert.Equal(t, "deploy.yaml", match.File)
	assert.Equal(t, "deploy-all", match.Name)
	assert.Equal(t, "Deploy all components", match.Description)
}

// TestExecuteDescribeWorkflows_SkipsInvalidFiles tests that invalid workflow files are skipped.
func TestExecuteDescribeWorkflows_SkipsInvalidFiles(t *testing.T) {
	tmpDir := t.TempDir()
	workflowsDir := filepath.Join(tmpDir, "workflows")
	err := os.MkdirAll(workflowsDir, 0o755)
	require.NoError(t, err)

	// Create a valid workflow file.
	validContent := `workflows:
  valid-workflow:
    steps:
      - name: step1
        command: echo hello
`
	err = os.WriteFile(filepath.Join(workflowsDir, "valid.yaml"), []byte(validContent), 0o644)
	require.NoError(t, err)

	// Create an invalid YAML file.
	invalidContent := `not: valid: yaml: content`
	err = os.WriteFile(filepath.Join(workflowsDir, "invalid.yaml"), []byte(invalidContent), 0o644)
	require.NoError(t, err)

	// Create a file without workflows key.
	noWorkflowsContent := `something_else:
  key: value
`
	err = os.WriteFile(filepath.Join(workflowsDir, "no-workflows.yaml"), []byte(noWorkflowsContent), 0o644)
	require.NoError(t, err)

	atmosConfig := schema.AtmosConfiguration{
		Workflows: schema.Workflows{
			BasePath: workflowsDir,
		},
	}

	listResult, _, _, err := ExecuteDescribeWorkflows(atmosConfig)

	// Should succeed but only have the valid workflow.
	assert.NoError(t, err)
	assert.Len(t, listResult, 1)
	assert.Equal(t, "valid-workflow", listResult[0].Workflow)
}
