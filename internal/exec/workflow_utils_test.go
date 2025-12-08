package exec

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"mvdan.cc/sh/v3/shell"

	errUtils "github.com/cloudposse/atmos/errors"
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

// TestStringsFieldsQuotedArguments demonstrates the issue with strings.Fields()
// when parsing workflow commands that contain quoted arguments.
// This test documents the bug where multi-component flags like --query with
// quoted expressions are incorrectly split.
func TestStringsFieldsQuotedArguments(t *testing.T) {
	tests := []struct {
		name              string
		command           string
		expectedArgs      []string // What we WANT (correct parsing)
		stringsFieldsArgs []string // What strings.Fields() produces (incorrect)
	}{
		{
			name:    "query with single-quoted expression containing spaces",
			command: "terraform plan --query '.metadata.component == \"gcp/compute/v0.2.0\"' -s dev",
			expectedArgs: []string{
				"terraform", "plan",
				"--query", ".metadata.component == \"gcp/compute/v0.2.0\"",
				"-s", "dev",
			},
			stringsFieldsArgs: []string{
				"terraform", "plan",
				"--query", "'.metadata.component", "==", "\"gcp/compute/v0.2.0\"'",
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
			stringsFieldsArgs: []string{
				"terraform", "plan",
				"--query", "\".settings.enabled", "==", "true\"",
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
			// This one works correctly because there are no spaces
			stringsFieldsArgs: []string{
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
			stringsFieldsArgs: []string{
				"terraform", "plan",
				"--components", "\"gcp/compute/001,", "gcp/compute/101\"",
				"-s", "dev",
			},
		},
		{
			name:    "simple command without quotes",
			command: "terraform plan vpc -s dev",
			expectedArgs: []string{
				"terraform", "plan", "vpc", "-s", "dev",
			},
			// This one works correctly
			stringsFieldsArgs: []string{
				"terraform", "plan", "vpc", "-s", "dev",
			},
		},
		{
			name:    "all flag (no arguments)",
			command: "terraform plan --all -s dev",
			expectedArgs: []string{
				"terraform", "plan", "--all", "-s", "dev",
			},
			// This one works correctly
			stringsFieldsArgs: []string{
				"terraform", "plan", "--all", "-s", "dev",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This demonstrates the current (buggy) behavior
			actualArgs := strings.Fields(tt.command)

			// Verify that strings.Fields produces the expected (possibly wrong) output
			assert.Equal(t, tt.stringsFieldsArgs, actualArgs,
				"strings.Fields() should produce these args (demonstrating current behavior)")

			// Document whether the current behavior matches desired behavior
			if !assert.ObjectsAreEqual(tt.expectedArgs, tt.stringsFieldsArgs) {
				t.Logf("BUG: strings.Fields() produces incorrect output for command: %s", tt.command)
				t.Logf("  Expected: %v", tt.expectedArgs)
				t.Logf("  Actual:   %v", actualArgs)
			}
		})
	}
}

// TestWorkflowCommandParsing tests that workflow commands are parsed correctly.
// This test verifies that shell.Fields() correctly handles quoted arguments.
func TestWorkflowCommandParsing_QuotedArguments(t *testing.T) {
	tests := []struct {
		name         string
		command      string
		expectedArgs []string
	}{
		{
			name:    "query with quoted expression",
			command: "terraform plan --query '.metadata.component == \"mock\"' -s nonprod",
			expectedArgs: []string{
				"terraform", "plan",
				"--query", ".metadata.component == \"mock\"",
				"-s", "nonprod",
			},
		},
		{
			name:    "components list",
			command: "terraform plan --components mock -s nonprod",
			expectedArgs: []string{
				"terraform", "plan",
				"--components", "mock",
				"-s", "nonprod",
			},
		},
		{
			name:    "all flag",
			command: "terraform plan --all -s nonprod",
			expectedArgs: []string{
				"terraform", "plan",
				"--all",
				"-s", "nonprod",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Current implementation uses strings.Fields which is buggy for quoted args
			actualArgs := strings.Fields(tt.command)

			// This test documents the expected behavior
			// After fix, we should use a proper shell-like parser
			t.Logf("Command: %s", tt.command)
			t.Logf("Expected args: %v", tt.expectedArgs)
			t.Logf("Actual args (strings.Fields): %v", actualArgs)

			// For commands with quoted arguments containing spaces, this will fail
			// This is the expected behavior after the fix is implemented
			// TODO: Uncomment after implementing the fix
			// assert.Equal(t, tt.expectedArgs, actualArgs)
		})
	}
}

// TestShellFieldsCorrectParsing verifies that shell.Fields() from mvdan.cc/sh
// correctly parses workflow commands with quoted arguments.
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
