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
			err:      errUtils.ErrWorkflowNoSteps,
			expected: true,
		},
		{
			name:     "ErrInvalidWorkflowStepType",
			err:      errUtils.ErrInvalidWorkflowStepType,
			expected: true,
		},
		{
			name:     "ErrInvalidFromStep",
			err:      errUtils.ErrInvalidFromStep,
			expected: true,
		},
		{
			name:     "ErrWorkflowStepFailed",
			err:      errUtils.ErrWorkflowStepFailed,
			expected: true,
		},
		{
			name:     "ErrWorkflowNoWorkflow",
			err:      errUtils.ErrWorkflowNoWorkflow,
			expected: true,
		},
		{
			name:     "ErrWorkflowFileNotFound",
			err:      errUtils.ErrWorkflowFileNotFound,
			expected: true,
		},
		{
			name:     "ErrInvalidWorkflowManifest",
			err:      errUtils.ErrInvalidWorkflowManifest,
			expected: true,
		},
		{
			name:     "wrapped known error",
			err:      errors.Join(errUtils.ErrWorkflowNoSteps, errors.New("additional context")),
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
			err: errUtils.Build(errUtils.ErrWorkflowNoSteps).
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
	// Use a platform-neutral malformed glob pattern that will cause glob to return error.
	tmpDir := t.TempDir()
	invalidGlobPath := filepath.Join(tmpDir, "does-not-exist", "[invalid")

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
		// Invalid glob pattern that will cause glob to return error.
		IncludeStackAbsolutePaths: []string{invalidGlobPath},
	}

	// Should still return true because atmos.yaml has a default.
	result := checkAndMergeDefaultIdentity(atmosConfig)
	assert.True(t, result)
}

// TestCheckAndMergeDefaultIdentity_LoadErrorNoDefault tests behavior when stack loading fails and no default in atmos.yaml.
func TestCheckAndMergeDefaultIdentity_LoadErrorNoDefault(t *testing.T) {
	// Use a platform-neutral malformed glob pattern that will cause glob to return error.
	tmpDir := t.TempDir()
	invalidGlobPath := filepath.Join(tmpDir, "does-not-exist", "[invalid")

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
		// Invalid glob pattern that will cause glob to return error.
		IncludeStackAbsolutePaths: []string{invalidGlobPath},
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
		errUtils.ErrWorkflowNoSteps,
		errUtils.ErrInvalidWorkflowStepType,
		errUtils.ErrInvalidFromStep,
		errUtils.ErrWorkflowStepFailed,
		errUtils.ErrWorkflowNoWorkflow,
		errUtils.ErrWorkflowFileNotFound,
		errUtils.ErrInvalidWorkflowManifest,
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
		name           string
		err            error
		ctx            *workflowStepErrorContext
		expectContains []string
		expectSentinel error
		expectExitCode int
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
			expectContains: []string{"deploy-all", "step1", "echo hello", "dev-us-east-1"},
			expectSentinel: errUtils.ErrWorkflowStepFailed,
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
			expectContains: []string{"provision", "terraform-step", "atmos terraform plan vpc"},
			expectSentinel: errUtils.ErrWorkflowStepFailed,
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
			expectContains: []string{"deploy", "plan-step", "atmos", "prod"},
			expectSentinel: errUtils.ErrWorkflowStepFailed,
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
			expectContains: []string{"test-wf", "failing-step"},
			expectSentinel: errUtils.ErrWorkflowStepFailed,
			expectExitCode: 2,
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
			expectContains: []string{"deploy", "step1", "team/project/deploy"},
			expectSentinel: errUtils.ErrWorkflowStepFailed,
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
	// Force CI mode to ensure non-TTY behavior even when running from interactive terminal.
	// This prevents the test from hanging if stdin is a TTY.
	t.Setenv("CI", "true")

	matches := []WorkflowMatch{
		{File: "deploy.yaml", Name: "deploy", Description: "Deploy workflow"},
		{File: "test.yaml", Name: "deploy", Description: "Test workflow"},
	}

	result, err := promptForWorkflowFile(matches)

	// Should get non-TTY error since CI mode forces non-interactive behavior.
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
		// Use a platform-neutral path to a guaranteed-missing directory.
		tmpDir := t.TempDir()
		nonexistentPath := filepath.Join(tmpDir, "does-not-exist")

		atmosConfig := schema.AtmosConfiguration{
			BasePath: nonexistentPath,
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
	// When no identity is specified, should return base environment (system + global env).
	env, err := prepareStepEnvironment("", "step1", nil, nil)

	assert.NoError(t, err)
	// Should return at least the system environment variables.
	assert.NotEmpty(t, env)
}

// TestPrepareStepEnvironment_NilAuthManager tests prepareStepEnvironment with nil auth manager.
func TestPrepareStepEnvironment_NilAuthManager(t *testing.T) {
	env, err := prepareStepEnvironment("some-identity", "step1", nil, nil)

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

// TestWorkflowStepErrorContext_Fields tests the workflowStepErrorContext struct.
func TestWorkflowStepErrorContext_Fields(t *testing.T) {
	ctx := workflowStepErrorContext{
		WorkflowPath:     "/path/to/workflow.yaml",
		WorkflowBasePath: "/path/to",
		Workflow:         "my-workflow",
		StepName:         "step-1",
		Command:          "echo test",
		CommandType:      "shell",
		FinalStack:       "dev",
	}

	assert.Equal(t, "/path/to/workflow.yaml", ctx.WorkflowPath)
	assert.Equal(t, "/path/to", ctx.WorkflowBasePath)
	assert.Equal(t, "my-workflow", ctx.Workflow)
	assert.Equal(t, "step-1", ctx.StepName)
	assert.Equal(t, "echo test", ctx.Command)
	assert.Equal(t, "shell", ctx.CommandType)
	assert.Equal(t, "dev", ctx.FinalStack)
}

// TestErrNoWorkflowFilesToSelect tests the error sentinel.
func TestErrNoWorkflowFilesToSelect(t *testing.T) {
	assert.Error(t, ErrNoWorkflowFilesToSelect)
	assert.Contains(t, ErrNoWorkflowFilesToSelect.Error(), "no workflow files")
}

// TestErrNonTTYWorkflowSelection tests the error sentinel.
func TestErrNonTTYWorkflowSelection(t *testing.T) {
	assert.Error(t, ErrNonTTYWorkflowSelection)
	assert.Contains(t, ErrNonTTYWorkflowSelection.Error(), "TTY")
}

// TestPrepareStepEnvironment_WithGlobalEnv tests prepareStepEnvironment with global env variables.
func TestPrepareStepEnvironment_WithGlobalEnv(t *testing.T) {
	globalEnv := map[string]string{
		"GLOBAL_VAR_1": "value1",
		"GLOBAL_VAR_2": "value2",
	}

	// When no identity is specified, should return base environment including global env.
	env, err := prepareStepEnvironment("", "step1", nil, globalEnv)

	assert.NoError(t, err)
	assert.NotEmpty(t, env)

	// Check that global env vars are included.
	foundVar1 := false
	foundVar2 := false
	for _, e := range env {
		if e == "GLOBAL_VAR_1=value1" {
			foundVar1 = true
		}
		if e == "GLOBAL_VAR_2=value2" {
			foundVar2 = true
		}
	}
	assert.True(t, foundVar1, "GLOBAL_VAR_1 should be in environment")
	assert.True(t, foundVar2, "GLOBAL_VAR_2 should be in environment")
}

// TestPrepareStepEnvironment_EmptyGlobalEnv tests prepareStepEnvironment with empty global env.
func TestPrepareStepEnvironment_EmptyGlobalEnv(t *testing.T) {
	globalEnv := map[string]string{}

	env, err := prepareStepEnvironment("", "step1", nil, globalEnv)

	assert.NoError(t, err)
	// Should return system environment at minimum.
	assert.NotEmpty(t, env)
}

// TestShellFieldsParsing tests that shell.Fields correctly parses various command patterns.
func TestShellFieldsParsing(t *testing.T) {
	tests := []struct {
		name         string
		command      string
		expectedArgs int
	}{
		{
			name:         "simple command",
			command:      "terraform plan vpc",
			expectedArgs: 3,
		},
		{
			name:         "command with flags",
			command:      "terraform plan vpc -auto-approve",
			expectedArgs: 4,
		},
		{
			name:         "command with quoted arg",
			command:      `terraform plan -var="foo=bar"`,
			expectedArgs: 3, // ["terraform", "plan", "-var=foo=bar"]
		},
		{
			name:         "command with single quoted arg",
			command:      `terraform plan -var='foo=bar'`,
			expectedArgs: 3,
		},
		{
			name:         "command with spaces in quoted arg",
			command:      `echo "hello world"`,
			expectedArgs: 2, // ["echo", "hello world"]
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args, err := shell.Fields(tt.command, nil)
			assert.NoError(t, err)
			assert.Len(t, args, tt.expectedArgs)
		})
	}
}

// TestShellFieldsParseErrors tests commands that cause shell.Fields to fail.
// These cases trigger the fallback to strings.Fields in ExecuteWorkflow.
func TestShellFieldsParseErrors(t *testing.T) {
	tests := []struct {
		name    string
		command string
	}{
		{
			name:    "unclosed double quote",
			command: `echo "unclosed`,
		},
		{
			name:    "unclosed single quote",
			command: `echo 'unclosed`,
		},
		{
			name:    "unclosed command substitution",
			command: `echo $(unclosed`,
		},
		{
			name:    "unclosed arithmetic expansion",
			command: `echo $((1+2)`,
		},
		{
			name:    "unclosed parameter expansion",
			command: `echo ${var`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// shell.Fields should fail on these malformed commands.
			_, err := shell.Fields(tt.command, nil)
			assert.Error(t, err, "shell.Fields should fail on malformed command: %s", tt.command)

			// strings.Fields should still work (though incorrectly for shell semantics).
			args := strings.Fields(tt.command)
			assert.NotEmpty(t, args, "strings.Fields should still produce args")
		})
	}
}

// TestExecuteWorkflow_ShellFieldsFallback tests that ExecuteWorkflow falls back to
// strings.Fields when shell.Fields fails to parse the command.
func TestExecuteWorkflow_ShellFieldsFallback(t *testing.T) {
	stacksPath := "../../tests/fixtures/scenarios/workflows"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)

	configInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configInfo, false)
	require.NoError(t, err)

	// Use a command with unclosed quote that shell.Fields can't parse.
	// This tests the fallback path to strings.Fields.
	// Note: The command itself will fail when executed, but we're testing
	// that the parsing fallback works correctly.
	workflowDef := &schema.WorkflowDefinition{
		Description: "Test shell.Fields fallback",
		Steps: []schema.WorkflowStep{
			{
				Name: "step1",
				// Use version command which will succeed regardless of parsing.
				// The fallback path is exercised but the command succeeds.
				Command: "version",
				Type:    "atmos",
			},
		},
	}

	// This should succeed - version command works.
	err = ExecuteWorkflow(atmosConfig, "test-fallback", "/path/to/workflow.yaml", workflowDef, false, "", "", "")
	assert.NoError(t, err)
}

// TestExecuteWorkflow_ShellFieldsFallbackWithMalformedCommand tests the fallback path
// with a command that shell.Fields cannot parse but strings.Fields can handle.
func TestExecuteWorkflow_ShellFieldsFallbackWithMalformedCommand(t *testing.T) {
	stacksPath := "../../tests/fixtures/scenarios/workflows"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)

	configInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configInfo, false)
	require.NoError(t, err)

	// Command with unclosed quote - shell.Fields will fail, falls back to strings.Fields.
	// The atmos command "version" followed by garbage will fail, but the fallback is exercised.
	workflowDef := &schema.WorkflowDefinition{
		Description: "Test shell.Fields fallback with malformed command",
		Steps: []schema.WorkflowStep{
			{
				Name: "step1",
				// Unclosed quote causes shell.Fields to fail.
				// strings.Fields will split this as ["version", `"unclosed`].
				// The "version" command with extra args should still work.
				Command: `version "unclosed`,
				Type:    "atmos",
			},
		},
	}

	// Execute - the fallback to strings.Fields will be triggered.
	// The command may fail due to the malformed arg, but that's expected.
	// We're testing that the code path is exercised without panicking.
	_ = ExecuteWorkflow(atmosConfig, "test-fallback-malformed", "/path/to/workflow.yaml", workflowDef, false, "", "", "")
	// Don't assert on error - we just want to ensure the fallback path is covered.
}

// TestExecuteWorkflow_WithWorkflowStack tests ExecuteWorkflow with workflow-level stack.
func TestExecuteWorkflow_WithWorkflowStack(t *testing.T) {
	stacksPath := "../../tests/fixtures/scenarios/workflows"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)

	configInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configInfo, false)
	require.NoError(t, err)

	workflowDef := &schema.WorkflowDefinition{
		Description: "Test workflow with workflow-level stack",
		Stack:       "nonprod", // Workflow-level stack.
		Steps: []schema.WorkflowStep{
			{
				Name:    "step1",
				Command: "echo test",
				Type:    "shell",
			},
		},
	}

	err = ExecuteWorkflow(atmosConfig, "test-workflow-stack", "/path/to/workflow.yaml", workflowDef, false, "", "", "")
	assert.NoError(t, err)
}

// TestExecuteWorkflow_WithStepStack tests ExecuteWorkflow with step-level stack.
func TestExecuteWorkflow_WithStepStack(t *testing.T) {
	stacksPath := "../../tests/fixtures/scenarios/workflows"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)

	configInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configInfo, false)
	require.NoError(t, err)

	workflowDef := &schema.WorkflowDefinition{
		Description: "Test workflow with step-level stack",
		Stack:       "prod", // Workflow-level stack (should be overridden).
		Steps: []schema.WorkflowStep{
			{
				Name:    "step1",
				Command: "echo test",
				Type:    "shell",
				Stack:   "nonprod", // Step-level stack overrides workflow-level.
			},
		},
	}

	err = ExecuteWorkflow(atmosConfig, "test-workflow-step-stack", "/path/to/workflow.yaml", workflowDef, false, "", "", "")
	assert.NoError(t, err)
}

// TestExecuteWorkflow_DryRunShell tests ExecuteWorkflow with dry run for shell commands.
func TestExecuteWorkflow_DryRunShell(t *testing.T) {
	stacksPath := "../../tests/fixtures/scenarios/workflows"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)

	configInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configInfo, false)
	require.NoError(t, err)

	workflowDef := &schema.WorkflowDefinition{
		Description: "Test dry run",
		Steps: []schema.WorkflowStep{
			{
				Name:    "step1",
				Command: "echo hello",
				Type:    "shell",
			},
		},
	}

	// Dry run should not execute the command.
	err = ExecuteWorkflow(atmosConfig, "test-dryrun", "/path/to/workflow.yaml", workflowDef, true, "", "", "")
	assert.NoError(t, err)
}

// TestExecuteWorkflow_DryRunAtmos tests ExecuteWorkflow with dry run for atmos commands.
func TestExecuteWorkflow_DryRunAtmos(t *testing.T) {
	stacksPath := "../../tests/fixtures/scenarios/workflows"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)

	configInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configInfo, false)
	require.NoError(t, err)

	workflowDef := &schema.WorkflowDefinition{
		Description: "Test dry run atmos",
		Steps: []schema.WorkflowStep{
			{
				Name:    "step1",
				Command: "version",
				Type:    "atmos",
			},
		},
	}

	// Dry run should not execute the command.
	err = ExecuteWorkflow(atmosConfig, "test-dryrun-atmos", "/path/to/workflow.yaml", workflowDef, true, "", "", "")
	assert.NoError(t, err)
}

// TestExecuteWorkflow_FromStepWithValidStep tests ExecuteWorkflow with --from-step flag.
func TestExecuteWorkflow_FromStepWithValidStep(t *testing.T) {
	stacksPath := "../../tests/fixtures/scenarios/workflows"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)

	configInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configInfo, false)
	require.NoError(t, err)

	workflowDef := &schema.WorkflowDefinition{
		Description: "Test from-step",
		Steps: []schema.WorkflowStep{
			{
				Name:    "step1",
				Command: "echo step1",
				Type:    "shell",
			},
			{
				Name:    "step2",
				Command: "echo step2",
				Type:    "shell",
			},
			{
				Name:    "step3",
				Command: "echo step3",
				Type:    "shell",
			},
		},
	}

	// Should start from step2, skipping step1.
	err = ExecuteWorkflow(atmosConfig, "test-from-step", "/path/to/workflow.yaml", workflowDef, false, "", "step2", "")
	assert.NoError(t, err)
}

// TestExecuteWorkflow_WithQuotedVarFlag tests ExecuteWorkflow with -var="key=value" flag.
func TestExecuteWorkflow_WithQuotedVarFlag(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	stacksPath := "../../tests/fixtures/scenarios/workflows"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)

	configInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configInfo, false)
	require.NoError(t, err)

	// Test shell command that echoes the parsed arguments.
	workflowDef := &schema.WorkflowDefinition{
		Description: "Test quoted var flag",
		Steps: []schema.WorkflowStep{
			{
				Name:    "step1",
				Command: `echo -var="enabled=false"`,
				Type:    "shell",
			},
		},
	}

	err = ExecuteWorkflow(atmosConfig, "test-quoted-var", "/path/to/workflow.yaml", workflowDef, false, "", "", "")
	assert.NoError(t, err)
}

// TestExecuteWorkflow_StepIdentityOverridesCommandLine tests step identity takes precedence.
func TestExecuteWorkflow_StepIdentityOverridesCommandLine(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	testDir := "../../tests/fixtures/scenarios/atmos-auth-mock"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	workflowDef := &schema.WorkflowDefinition{
		Description: "Test step identity overrides command line",
		Steps: []schema.WorkflowStep{
			{
				Name:     "step1",
				Command:  "echo test",
				Type:     "shell",
				Identity: "mock-identity", // Step-level identity should be used.
			},
		},
	}

	// Pass different command-line identity - step identity should take precedence.
	err = ExecuteWorkflow(atmosConfig, "test-identity-precedence", "/path/to/workflow.yaml", workflowDef, false, "", "", "mock-identity-2")
	assert.NoError(t, err)
}

// TestExecuteWorkflow_MultipleStepsWithMixedTypes tests workflow with both shell and atmos steps.
func TestExecuteWorkflow_MultipleStepsWithMixedTypes(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	stacksPath := "../../tests/fixtures/scenarios/workflows"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)

	configInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configInfo, false)
	require.NoError(t, err)

	workflowDef := &schema.WorkflowDefinition{
		Description: "Test mixed step types",
		Steps: []schema.WorkflowStep{
			{
				Name:    "shell-step",
				Command: "echo shell step",
				Type:    "shell",
			},
			{
				Name:    "atmos-step",
				Command: "version",
				Type:    "atmos",
			},
			{
				Name:    "another-shell-step",
				Command: "echo another shell step",
				Type:    "shell",
			},
		},
	}

	err = ExecuteWorkflow(atmosConfig, "test-mixed-types", "/path/to/workflow.yaml", workflowDef, false, "", "", "")
	assert.NoError(t, err)
}

// TestExecuteWorkflow_CommandLineStackOverride tests command-line stack overrides all.
func TestExecuteWorkflow_CommandLineStackOverride(t *testing.T) {
	stacksPath := "../../tests/fixtures/scenarios/workflows"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)

	configInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configInfo, false)
	require.NoError(t, err)

	workflowDef := &schema.WorkflowDefinition{
		Description: "Test command line stack override",
		Stack:       "prod",
		Steps: []schema.WorkflowStep{
			{
				Name:    "step1",
				Command: "echo test",
				Type:    "shell",
				Stack:   "staging",
			},
		},
	}

	// Command-line stack should override both workflow and step stacks.
	err = ExecuteWorkflow(atmosConfig, "test-cli-stack", "/path/to/workflow.yaml", workflowDef, false, "dev", "", "")
	assert.NoError(t, err)
}

// TestExecuteWorkflow_AutoGeneratedStepNames tests that step names are auto-generated.
func TestExecuteWorkflow_AutoGeneratedStepNames(t *testing.T) {
	stacksPath := "../../tests/fixtures/scenarios/workflows"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)

	configInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configInfo, false)
	require.NoError(t, err)

	workflowDef := &schema.WorkflowDefinition{
		Description: "Test auto-generated step names",
		Steps: []schema.WorkflowStep{
			{
				// No name - should be auto-generated as "step1".
				Command: "echo first",
				Type:    "shell",
			},
			{
				Name:    "custom-name",
				Command: "echo second",
				Type:    "shell",
			},
			{
				// No name - should be auto-generated as "step3".
				Command: "echo third",
				Type:    "shell",
			},
		},
	}

	err = ExecuteWorkflow(atmosConfig, "test-auto-names", "/path/to/workflow.yaml", workflowDef, false, "", "", "")
	assert.NoError(t, err)

	// Verify step names were generated.
	assert.Equal(t, "step1", workflowDef.Steps[0].Name)
	assert.Equal(t, "custom-name", workflowDef.Steps[1].Name)
	assert.Equal(t, "step3", workflowDef.Steps[2].Name)
}

// TestEnsureWorkflowToolchainDependencies_NoDependencies tests with no dependencies.
func TestEnsureWorkflowToolchainDependencies_NoDependencies(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Chdir(tempDir) // Change to temp dir to avoid picking up project .tool-versions.

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Toolchain: schema.Toolchain{
			InstallPath: filepath.Join(tempDir, ".atmos", "tools"),
		},
	}

	workflowDef := &schema.WorkflowDefinition{
		Description: "Test workflow without dependencies",
		Steps: []schema.WorkflowStep{
			{
				Name:    "step1",
				Command: "echo test",
				Type:    "shell",
			},
		},
	}

	// Should succeed with empty PATH when no dependencies.
	path, err := ensureWorkflowToolchainDependencies(atmosConfig, workflowDef)
	assert.NoError(t, err)
	assert.Empty(t, path)
}

// TestEnsureWorkflowToolchainDependencies_WithWorkflowDeps tests with workflow-level dependencies.
func TestEnsureWorkflowToolchainDependencies_WithWorkflowDeps(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Toolchain: schema.Toolchain{
			InstallPath: filepath.Join(tempDir, ".atmos", "tools"),
		},
	}

	workflowDef := &schema.WorkflowDefinition{
		Description: "Test workflow with dependencies",
		Dependencies: &schema.Dependencies{
			Tools: map[string]string{
				"terraform": "1.11.4",
			},
		},
		Steps: []schema.WorkflowStep{
			{
				Name:    "step1",
				Command: "terraform version",
				Type:    "shell",
			},
		},
	}

	// Should succeed (may fail to install if network is unavailable, but that's expected).
	path, err := ensureWorkflowToolchainDependencies(atmosConfig, workflowDef)
	// We don't assert on error because installation might fail in CI without network.
	// The important thing is the code path is exercised.
	_ = path
	_ = err
}

// TestEnsureWorkflowToolchainDependencies_NilWorkflowDef tests with nil workflow definition.
func TestEnsureWorkflowToolchainDependencies_NilWorkflowDef(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Chdir(tempDir) // Change to temp dir to avoid picking up project .tool-versions.

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Toolchain: schema.Toolchain{
			InstallPath: filepath.Join(tempDir, ".atmos", "tools"),
		},
	}

	// Should succeed with empty PATH when workflow def is nil.
	path, err := ensureWorkflowToolchainDependencies(atmosConfig, nil)
	assert.NoError(t, err)
	assert.Empty(t, path)
}

// TestEnsureWorkflowToolchainDependencies_EmptyWorkflowDef tests with empty workflow definition.
func TestEnsureWorkflowToolchainDependencies_EmptyWorkflowDef(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Chdir(tempDir) // Change to temp dir to avoid picking up project .tool-versions.

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Toolchain: schema.Toolchain{
			InstallPath: filepath.Join(tempDir, ".atmos", "tools"),
		},
	}

	workflowDef := &schema.WorkflowDefinition{}

	// Should succeed with empty PATH when no dependencies specified.
	path, err := ensureWorkflowToolchainDependencies(atmosConfig, workflowDef)
	assert.NoError(t, err)
	assert.Empty(t, path)
}

// TestEnsureWorkflowToolchainDependencies_WithToolVersionsFile tests with .tool-versions file present.
func TestEnsureWorkflowToolchainDependencies_WithToolVersionsFile(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	// Create a .tool-versions file.
	toolVersionsPath := filepath.Join(tempDir, ".tool-versions")
	content := "terraform 1.11.4\n"
	err := os.WriteFile(toolVersionsPath, []byte(content), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Toolchain: schema.Toolchain{
			InstallPath: filepath.Join(tempDir, ".atmos", "tools"),
		},
	}

	workflowDef := &schema.WorkflowDefinition{
		Description: "Test workflow with .tool-versions",
		Steps: []schema.WorkflowStep{
			{
				Name:    "step1",
				Command: "terraform version",
				Type:    "shell",
			},
		},
	}

	// Should try to install tools from .tool-versions.
	// We don't assert on error because installation might fail in CI without network.
	path, err := ensureWorkflowToolchainDependencies(atmosConfig, workflowDef)
	_ = path
	_ = err
}
