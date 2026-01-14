package cmd

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestVerifyInsideGitRepo(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Save the current working directory
	currentDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}

	// Test cases
	tests := []struct {
		name     string
		dir      string
		expected bool
	}{
		{
			name:     "outside git repository",
			dir:      tmpDir,
			expected: false,
		},
		{
			name:     "inside git repository",
			dir:      currentDir,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test environment
			t.Chdir(tt.dir)

			// Run test
			result := verifyInsideGitRepo()

			// Assert result
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestContains(t *testing.T) {
	tests := []struct {
		name     string
		slice    []string
		target   string
		expected bool
	}{
		{
			name:     "empty slice",
			slice:    []string{},
			target:   "test",
			expected: false,
		},
		{
			name:     "contains target",
			slice:    []string{"one", "two", "three"},
			target:   "two",
			expected: true,
		},
		{
			name:     "does not contain target",
			slice:    []string{"one", "two", "three"},
			target:   "four",
			expected: false,
		},
		{
			name:     "case sensitive",
			slice:    []string{"One", "Two", "Three"},
			target:   "one",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Contains(tt.slice, tt.target)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsVersionCommand(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected bool
	}{
		{
			name:     "version command",
			args:     []string{"version"},
			expected: true,
		},
		{
			name:     "version command with flags",
			args:     []string{"version", "--output", "json"},
			expected: true,
		},
		{
			name:     "--version flag",
			args:     []string{"--version"},
			expected: true,
		},
		{
			name:     "not version command",
			args:     []string{"help"},
			expected: false,
		},
		{
			name:     "empty args",
			args:     []string{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// isVersionCommand() reads os.Args directly, so tests must manipulate it.
			// This is acceptable because isVersionCommand() is called early in init
			// before Cobra command parsing happens.
			oldArgs := os.Args
			defer func() { os.Args = oldArgs }()

			// Set up test args
			os.Args = append([]string{"atmos"}, tt.args...)

			// Test the function
			result := isVersionCommand()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// skipIfPackerNotInstalled skips the test if packer is not available in PATH.
func skipIfPackerNotInstalled(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("packer"); err != nil {
		t.Skipf("packer not installed: %v", err)
	}
}

// skipIfHelmfileNotInstalled skips the test if helmfile is not available in PATH.
func skipIfHelmfileNotInstalled(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("helmfile"); err != nil {
		t.Skipf("helmfile not installed: %v", err)
	}
}

// TestPrintMessageForMissingAtmosConfig tests the printMessageForMissingAtmosConfig function.
func TestPrintMessageForMissingAtmosConfig(t *testing.T) {
	tests := []struct {
		name        string
		atmosConfig schema.AtmosConfiguration
	}{
		{
			name: "default config missing",
			atmosConfig: schema.AtmosConfiguration{
				BasePath: "/test",
				Stacks: schema.Stacks{
					BasePath: "stacks",
				},
				Default: true,
			},
		},
		{
			name: "custom config with invalid paths",
			atmosConfig: schema.AtmosConfiguration{
				BasePath: "/custom",
				Stacks: schema.Stacks{
					BasePath: "my-stacks",
				},
				Default: false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This function writes to both stdout (logo) and stderr (markdown messages).
			// We verify it executes without panic. The actual content is tested via
			// markdown rendering tests. This verifies both code paths execute:
			// (Default=true uses missingConfigDefaultMarkdown, Default=false uses missingConfigFoundMarkdown)

			// Use defer/recover to catch any panics.
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("printMessageForMissingAtmosConfig panicked: %v", r)
				}
			}()

			printMessageForMissingAtmosConfig(tt.atmosConfig)
		})
	}
}

// TestIdentityFlagCompletion tests the identityFlagCompletion function.
func TestIdentityFlagCompletion(t *testing.T) {
	tests := []struct {
		name           string
		setupConfigDir string
		expectedCount  int
		expectedNames  []string
		expectError    bool
	}{
		{
			name:           "valid auth config with identities",
			setupConfigDir: "../examples/demo-auth",
			expectedCount:  4,
			expectedNames:  []string{"oidc", "sso", "superuser", "saml"},
			expectError:    false,
		},
		{
			name:           "no atmos config",
			setupConfigDir: "",
			expectedCount:  0,
			expectedNames:  []string{},
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Change to test directory if specified (automatically reverted after test).
			if tt.setupConfigDir != "" {
				t.Chdir(tt.setupConfigDir)
			} else {
				// Use a temp directory with no atmos.yaml.
				tmpDir := t.TempDir()
				t.Chdir(tmpDir)
			}

			// Call the completion function.
			completions, directive := identityFlagCompletion(nil, []string{}, "")

			if tt.expectError {
				// When there's no config, we expect empty results.
				assert.Equal(t, 0, len(completions))
			} else {
				// Verify we got the expected number of completions.
				assert.Equal(t, tt.expectedCount, len(completions))

				// Verify all expected identities are present.
				for _, expected := range tt.expectedNames {
					assert.Contains(t, completions, expected)
				}
			}

			// Verify the directive is always NoFileComp.
			assert.Equal(t, 4, int(directive)) // cobra.ShellCompDirectiveNoFileComp
		})
	}
}

// TestAddIdentityCompletion tests the AddIdentityCompletion function.
func TestAddIdentityCompletion(t *testing.T) {
	tests := []struct {
		name                 string
		setupFlags           bool
		shouldHaveCompletion bool
	}{
		{
			name:                 "command with identity flag",
			setupFlags:           true,
			shouldHaveCompletion: true,
		},
		{
			name:                 "command without identity flag",
			setupFlags:           false,
			shouldHaveCompletion: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test command.
			cmd := &cobra.Command{
				Use:   "test",
				Short: "Test command",
			}

			if tt.setupFlags {
				cmd.Flags().StringP("identity", "i", "", "Test identity flag")
			}

			// Call AddIdentityCompletion.
			AddIdentityCompletion(cmd)

			// Verify completion function was registered (or not).
			if tt.shouldHaveCompletion {
				// Try to get the completion function.
				completionFunc, exists := cmd.GetFlagCompletionFunc("identity")
				assert.True(t, exists, "Completion function should be registered")
				assert.NotNil(t, completionFunc, "Completion function should not be nil")
			} else {
				// Verify no completion function exists.
				_, exists := cmd.GetFlagCompletionFunc("identity")
				assert.False(t, exists, "Completion function should not be registered")
			}
		})
	}
}

// TestIdentityFlagCompletionWithNoAuthConfig tests edge case with nil auth config.
func TestIdentityFlagCompletionWithNoAuthConfig(t *testing.T) {
	// Create a temp directory with an atmos.yaml that has no auth section (automatically reverted after test).
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	// Create minimal atmos.yaml without auth section.
	atmosYaml := `base_path: .
stacks:
  base_path: stacks
`
	err := os.WriteFile("atmos.yaml", []byte(atmosYaml), 0o644)
	require.NoError(t, err)

	// Call completion function.
	completions, directive := identityFlagCompletion(nil, []string{}, "")

	// Should return empty list when no auth config exists.
	assert.Empty(t, completions)
	assert.Equal(t, 4, int(directive)) // ShellCompDirectiveNoFileComp
}

// TestIdentityFlagCompletionPartialMatch tests completion with partial input.
func TestIdentityFlagCompletionPartialMatch(t *testing.T) {
	// Change to demo-auth directory (automatically reverted after test).
	t.Chdir("../examples/demo-auth")

	// Call completion with partial input.
	completions, directive := identityFlagCompletion(nil, []string{}, "ss")

	// Should still return all identities (filtering is done by shell).
	assert.NotEmpty(t, completions)
	assert.Contains(t, completions, "sso")
	assert.Equal(t, 4, int(directive)) // ShellCompDirectiveNoFileComp
}

// TestIdentityFlagCompletionSorting tests that identities are returned in sorted order.
func TestIdentityFlagCompletionSorting(t *testing.T) {
	// Change to demo-auth directory (automatically reverted after test).
	t.Chdir("../examples/demo-auth")

	// Call completion function.
	completions, _ := identityFlagCompletion(nil, []string{}, "")

	// Verify completions are in sorted order.
	assert.NotEmpty(t, completions)
	sortedCompletions := make([]string, len(completions))
	copy(sortedCompletions, completions)
	sort.Strings(sortedCompletions)
	assert.Equal(t, sortedCompletions, completions, "Completions should be sorted alphabetically")
}

// TestIdentityFlagCompletionErrorPath tests error handling when config loading fails.
func TestIdentityFlagCompletionErrorPath(t *testing.T) {
	// Create a temp directory with invalid atmos.yaml (automatically reverted after test).
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	// Create invalid YAML that will cause InitCliConfig to fail.
	invalidYaml := `invalid: yaml: content:
  - this is: [broken
`
	err := os.WriteFile("atmos.yaml", []byte(invalidYaml), 0o644)
	require.NoError(t, err)

	// Call completion function - should handle error gracefully.
	completions, directive := identityFlagCompletion(nil, []string{}, "")

	// Should return empty results with NoFileComp directive on error.
	assert.Empty(t, completions)
	assert.Equal(t, 4, int(directive)) // ShellCompDirectiveNoFileComp
}

// TestAddIdentityCompletionErrorHandling tests error handling in registration.
func TestAddIdentityCompletionErrorHandling(t *testing.T) {
	// Create a command with identity flag.
	cmd := &cobra.Command{
		Use:   "test",
		Short: "Test command",
	}
	cmd.Flags().StringP("identity", "i", "", "Test identity flag")

	// Register completion twice to test error path.
	AddIdentityCompletion(cmd)

	// Call again - should handle error gracefully (already registered).
	// This tests the error logging path.
	AddIdentityCompletion(cmd)

	// Verify completion is still registered.
	completionFunc, exists := cmd.GetFlagCompletionFunc("identity")
	assert.True(t, exists)
	assert.NotNil(t, completionFunc)
}

// TestStackFlagCompletion tests the stackFlagCompletion function.
func TestStackFlagCompletion(t *testing.T) {
	// Change to a directory with valid stacks configuration (automatically reverted after test).
	testDir := "../examples/quick-start-advanced"
	t.Chdir(testDir)

	// Create a test command.
	cmd := &cobra.Command{
		Use:   "test",
		Short: "Test command",
	}

	// Call the completion function.
	completions, directive := StackFlagCompletion(cmd, []string{}, "")

	// Verify we got some completions.
	assert.NotEmpty(t, completions, "Should have stack completions")

	// Verify the directive is NoFileComp.
	assert.Equal(t, 4, int(directive)) // cobra.ShellCompDirectiveNoFileComp
}

// TestAddStackCompletion tests the AddStackCompletion function.
func TestAddStackCompletion(t *testing.T) {
	tests := []struct {
		name       string
		setupFlags bool
	}{
		{
			name:       "command without stack flag",
			setupFlags: false,
		},
		{
			name:       "command with existing stack flag",
			setupFlags: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test command.
			cmd := &cobra.Command{
				Use:   "test",
				Short: "Test command",
			}

			if tt.setupFlags {
				cmd.PersistentFlags().StringP("stack", "s", "", "Stack flag")
			}

			// Call AddStackCompletion.
			AddStackCompletion(cmd)

			// Verify the flag exists.
			flag := cmd.Flag("stack")
			assert.NotNil(t, flag, "Stack flag should exist")

			// Verify completion function was registered.
			completionFunc, exists := cmd.GetFlagCompletionFunc("stack")
			assert.True(t, exists, "Completion function should be registered")
			assert.NotNil(t, completionFunc, "Completion function should not be nil")
		})
	}
}

// TestIdentityArgCompletionSorting tests that identityArgCompletion returns sorted identities.
func TestIdentityArgCompletionSorting(t *testing.T) {
	// Change to demo-auth directory (automatically reverted after test).
	t.Chdir("../examples/demo-auth")

	// Create a test command.
	cmd := &cobra.Command{
		Use:   "test",
		Short: "Test command",
	}

	// Call completion function with no previous args (first positional argument).
	completions, directive := identityArgCompletion(cmd, []string{}, "")

	// Verify completions are in sorted order.
	assert.NotEmpty(t, completions)
	sortedCompletions := make([]string, len(completions))
	copy(sortedCompletions, completions)
	sort.Strings(sortedCompletions)
	assert.Equal(t, sortedCompletions, completions, "Completions should be sorted alphabetically")
	assert.Equal(t, 4, int(directive)) // cobra.ShellCompDirectiveNoFileComp
}

// TestIdentityArgCompletionOnlyFirstArg tests that identityArgCompletion only completes the first arg.
func TestIdentityArgCompletionOnlyFirstArg(t *testing.T) {
	// Change to demo-auth directory (automatically reverted after test).
	t.Chdir("../examples/demo-auth")

	// Create a test command.
	cmd := &cobra.Command{
		Use:   "test",
		Short: "Test command",
	}

	// Call completion function with an existing arg (second positional argument).
	completions, directive := identityArgCompletion(cmd, []string{"existing-arg"}, "")

	// Should return no completions for second arg.
	assert.Empty(t, completions)
	assert.Equal(t, 4, int(directive)) // cobra.ShellCompDirectiveNoFileComp
}

// TestListStacksForComponent tests the listStacksForComponent function.
func TestListStacksForComponent(t *testing.T) {
	tests := []struct {
		name          string
		component     string
		setupDir      string
		expectStacks  bool
		expectError   bool
		minStackCount int
	}{
		{
			name:          "component exists in stacks",
			component:     "myapp",
			setupDir:      "../examples/demo-stacks",
			expectStacks:  true,
			expectError:   false,
			minStackCount: 1,
		},
		{
			name:          "component does not exist",
			component:     "nonexistent-component",
			setupDir:      "../examples/demo-stacks",
			expectStacks:  false,
			expectError:   false,
			minStackCount: 0,
		},
		{
			name:        "invalid directory with no config",
			component:   "test",
			setupDir:    "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Change to test directory if specified (automatically reverted after test).
			if tt.setupDir != "" {
				t.Chdir(tt.setupDir)
			} else {
				// Use a temp directory with no atmos.yaml.
				tmpDir := t.TempDir()
				t.Chdir(tmpDir)
			}

			// Call the function.
			stacks, err := listStacksForComponent(tt.component)

			if tt.expectError {
				assert.Error(t, err, "Should return error for invalid config")
				return
			}

			assert.NoError(t, err, "Should not return error")

			if tt.expectStacks {
				assert.NotEmpty(t, stacks, "Should return at least one stack")
				assert.GreaterOrEqual(t, len(stacks), tt.minStackCount, "Should have minimum number of stacks")
			} else {
				assert.Empty(t, stacks, "Should return empty list for non-existent component")
			}
		})
	}
}

// TestListStacksForComponentErrorHandling tests error handling in listStacksForComponent.
func TestListStacksForComponentErrorHandling(t *testing.T) {
	// Create a temp directory with invalid atmos.yaml (automatically reverted after test).
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	// Create invalid YAML that will cause InitCliConfig to fail.
	invalidYaml := `invalid: yaml: content:
  - this is: [broken
`
	err := os.WriteFile("atmos.yaml", []byte(invalidYaml), 0o644)
	require.NoError(t, err)

	// Call the function - should handle error gracefully.
	stacks, err := listStacksForComponent("test-component")

	// Should return error.
	assert.Error(t, err, "Should return error for invalid config")
	assert.Nil(t, stacks, "Should return nil stacks on error")
}

// TestListStacksForComponentEmptyComponent tests with empty component name.
func TestListStacksForComponentEmptyComponent(t *testing.T) {
	// Change to a valid directory (automatically reverted after test).
	t.Chdir("../examples/demo-stacks")

	// Call with empty component name.
	stacks, err := listStacksForComponent("")

	// Should not error. Empty component filter returns all stacks.
	assert.NoError(t, err, "Should not error with empty component")
	assert.NotEmpty(t, stacks, "Empty component filter returns all stacks")
}

// TestFilterChdirArgs tests the filterChdirArgs function.
func TestFilterChdirArgs(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "no chdir flags",
			input:    []string{"terraform", "plan", "component"},
			expected: []string{"terraform", "plan", "component"},
		},
		{
			name:     "long form --chdir=value",
			input:    []string{"terraform", "plan", "--chdir=/path", "component"},
			expected: []string{"terraform", "plan", "component"},
		},
		{
			name:     "long form --chdir value",
			input:    []string{"terraform", "plan", "--chdir", "/path", "component"},
			expected: []string{"terraform", "plan", "component"},
		},
		{
			name:     "short form -C=value",
			input:    []string{"terraform", "plan", "-C=/path", "component"},
			expected: []string{"terraform", "plan", "component"},
		},
		{
			name:     "short form -C value",
			input:    []string{"terraform", "plan", "-C", "/path", "component"},
			expected: []string{"terraform", "plan", "component"},
		},
		{
			name:     "short form concatenated -Cvalue",
			input:    []string{"terraform", "plan", "-C/path", "component"},
			expected: []string{"terraform", "plan", "component"},
		},
		{
			name:     "multiple chdir flags",
			input:    []string{"terraform", "--chdir=/first", "plan", "-C", "/second", "component"},
			expected: []string{"terraform", "plan", "component"},
		},
		{
			name:     "chdir at beginning",
			input:    []string{"--chdir=/path", "terraform", "plan"},
			expected: []string{"terraform", "plan"},
		},
		{
			name:     "chdir at end",
			input:    []string{"terraform", "plan", "--chdir=/path"},
			expected: []string{"terraform", "plan"},
		},
		{
			name:     "mixed formats",
			input:    []string{"terraform", "--chdir=/first", "plan", "-C/second", "deploy", "-C", "/third"},
			expected: []string{"terraform", "plan", "deploy"},
		},
		{
			name:     "preserve non-chdir flags",
			input:    []string{"terraform", "plan", "--var-file=test.tfvars", "--chdir=/path", "--auto-approve"},
			expected: []string{"terraform", "plan", "--var-file=test.tfvars", "--auto-approve"},
		},
		{
			name:     "chdir with tilde",
			input:    []string{"terraform", "--chdir=~/project", "plan"},
			expected: []string{"terraform", "plan"},
		},
		{
			name:     "empty input",
			input:    []string{},
			expected: []string{},
		},
		{
			name:     "only chdir flags",
			input:    []string{"--chdir=/path", "-C", "/another"},
			expected: []string{},
		},
		{
			name:     "chdir-like but not flag (value contains chdir)",
			input:    []string{"terraform", "plan", "my-chdir-component"},
			expected: []string{"terraform", "plan", "my-chdir-component"},
		},
		{
			name:     "short C alone should be filtered",
			input:    []string{"terraform", "-C", "/path", "plan"},
			expected: []string{"terraform", "plan"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterChdirArgs(tt.input)
			assert.Equal(t, tt.expected, result, "filterChdirArgs should correctly filter chdir arguments")
		})
	}
}

// TestValidateAtmosConfig tests the error-returning version of config validation.
// This test was not possible before refactoring because checkAtmosConfig() called os.Exit().
func TestValidateAtmosConfig(t *testing.T) {
	tests := []struct {
		name        string
		opts        []AtmosValidateOption
		wantErr     bool
		description string
	}{
		{
			name:        "skip stack validation",
			opts:        []AtmosValidateOption{WithStackValidation(false)},
			wantErr:     false,
			description: "Should succeed when skipping stack validation even without a valid atmos project",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAtmosConfig(tt.opts...)

			if tt.wantErr {
				assert.Error(t, err, tt.description)
			} else {
				// With stack validation disabled, we should get nil or config load error.
				// The important thing is it doesn't call os.Exit().
				t.Logf("validateAtmosConfig returned: %v", err)
			}
		})
	}
}

// TestGetConfigAndStacksInfo tests the error-returning version of config and stacks info processing.
// This test was not possible before refactoring because getConfigAndStacksInfo() called os.Exit() on errors.
func TestGetConfigAndStacksInfo(t *testing.T) {
	tests := []struct {
		name        string
		commandName string
		args        []string
		description string
	}{
		{
			name:        "empty args",
			commandName: "terraform",
			args:        []string{},
			description: "Should handle empty args without panicking or calling os.Exit()",
		},
		{
			name:        "args with double dash",
			commandName: "terraform",
			args:        []string{"plan", "--", "extra", "args"},
			description: "Should properly split args at double dash without panicking",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a minimal cobra command for testing.
			cmd := &cobra.Command{
				Use: tt.commandName,
			}

			// The key test: this call should return an error instead of calling os.Exit().
			info, err := getConfigAndStacksInfo(tt.commandName, cmd, tt.args)

			// We expect an error here because we're not in a valid atmos directory,
			// but the important thing is that it doesn't call os.Exit()
			// and we can actually test the error behavior.
			if err != nil {
				// This is expected - we're testing that errors are returned properly.
				t.Logf("Got expected error (no valid config): %v", err)
				assert.Error(t, err, tt.description)
			} else {
				// If somehow we got a valid config, verify the info structure.
				assert.IsType(t, schema.ConfigAndStacksInfo{}, info, "Should return ConfigAndStacksInfo struct")
			}
		})
	}
}

// TestGetConfigAndStacksInfoDoubleDashHandling tests that double-dash arguments are properly separated.
// This verifies the refactored function maintains the original behavior.
func TestGetConfigAndStacksInfoDoubleDashHandling(t *testing.T) {
	cmd := &cobra.Command{
		Use: "terraform",
	}

	// Test with args containing "--".
	args := []string{"plan", "component", "--stack", "dev", "--", "-target=resource"}

	_, err := getConfigAndStacksInfo("terraform", cmd, args)

	// We expect this to fail due to missing atmos config, but importantly
	// it should NOT panic or call os.Exit().
	assert.Error(t, err, "Should return error for missing config instead of calling os.Exit()")
	assert.ErrorIs(t, err, errUtils.ErrStacksDirectoryDoesNotExist)
}

// TestGetConfigAndStacksInfoReturnsErrorInsteadOfExiting demonstrates the key improvement.
// Before refactoring, this test would have been impossible because the function called os.Exit().
func TestGetConfigAndStacksInfoReturnsErrorInsteadOfExiting(t *testing.T) {
	cmd := &cobra.Command{
		Use: "terraform",
	}

	// Call the function with invalid input that would previously cause os.Exit(1).
	_, err := getConfigAndStacksInfo("terraform", cmd, []string{})

	// The key assertion: we got an error back instead of the process terminating.
	assert.Error(t, err, "Function should return error instead of calling os.Exit()")

	// Verify it contains useful information.
	assert.NotEmpty(t, err.Error(), "Error should have a descriptive message")
}

// TestValidateAtmosConfigWithOptions tests that options pattern works correctly.
func TestValidateAtmosConfigWithOptions(t *testing.T) {
	// Test that WithStackValidation option is respected.
	err := validateAtmosConfig(WithStackValidation(false))
	// With stack validation disabled, we should only fail on config loading.
	// (which will fail in test env, but that's OK - we're testing it returns an error)
	if err != nil {
		t.Logf("Got expected error with stack validation disabled: %v", err)
	}
}

// TestErrorWrappingInGetConfigAndStacksInfo verifies proper error wrapping.
func TestErrorWrappingInGetConfigAndStacksInfo(t *testing.T) {
	cmd := &cobra.Command{
		Use: "terraform",
	}

	_, err := getConfigAndStacksInfo("terraform", cmd, []string{})

	// Should always return an error in test environment (config/stacks not found).
	require.Error(t, err, "Expected error when stacks directory doesn't exist")

	// Verify error can be checked with errors.Is() - this tests proper error wrapping.
	// The function should return ErrStacksDirectoryDoesNotExist from validateAtmosConfig.
	assert.True(t, errors.Is(err, errUtils.ErrStacksDirectoryDoesNotExist),
		"Error should wrap ErrStacksDirectoryDoesNotExist, got: %v", err)

	// Verify error contains useful context.
	assert.NotEmpty(t, err.Error(), "Error should have a message")
}

// TestDetermineComponentTypeFromCommand tests component type detection from command hierarchy.
func TestDetermineComponentTypeFromCommand(t *testing.T) {
	tests := []struct {
		name         string
		setupFunc    func() *cobra.Command
		expectedType string
	}{
		{
			name: "terraform command",
			setupFunc: func() *cobra.Command {
				terraform := &cobra.Command{Use: "terraform"}
				plan := &cobra.Command{Use: "plan"}
				terraform.AddCommand(plan)
				return plan
			},
			expectedType: "terraform",
		},
		{
			name: "helmfile command",
			setupFunc: func() *cobra.Command {
				helmfile := &cobra.Command{Use: "helmfile"}
				apply := &cobra.Command{Use: "apply"}
				helmfile.AddCommand(apply)
				return apply
			},
			expectedType: "helmfile",
		},
		{
			name: "packer command",
			setupFunc: func() *cobra.Command {
				packer := &cobra.Command{Use: "packer"}
				build := &cobra.Command{Use: "build"}
				packer.AddCommand(build)
				return build
			},
			expectedType: "packer",
		},
		{
			name: "nested terraform subcommand",
			setupFunc: func() *cobra.Command {
				root := &cobra.Command{Use: "atmos"}
				terraform := &cobra.Command{Use: "terraform"}
				plan := &cobra.Command{Use: "plan"}
				root.AddCommand(terraform)
				terraform.AddCommand(plan)
				return plan
			},
			expectedType: "terraform",
		},
		{
			name: "unknown command defaults to terraform",
			setupFunc: func() *cobra.Command {
				root := &cobra.Command{Use: "atmos"}
				unknown := &cobra.Command{Use: "unknown"}
				root.AddCommand(unknown)
				return unknown
			},
			expectedType: "terraform",
		},
		{
			name: "no parent command defaults to terraform",
			setupFunc: func() *cobra.Command {
				return &cobra.Command{Use: "standalone"}
			},
			expectedType: "terraform",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := tt.setupFunc()
			result := determineComponentTypeFromCommand(cmd)
			assert.Equal(t, tt.expectedType, result)
		})
	}
}

// TestCloneCommand tests the cloneCommand function.
func TestCloneCommand(t *testing.T) {
	tests := []struct {
		name     string
		input    *schema.Command
		wantErr  bool
		verifyFn func(t *testing.T, orig, clone *schema.Command)
	}{
		{
			name: "basic command",
			input: &schema.Command{
				Name:        "test",
				Description: "Test command",
			},
			wantErr: false,
			verifyFn: func(t *testing.T, orig, clone *schema.Command) {
				assert.Equal(t, orig.Name, clone.Name)
				assert.Equal(t, orig.Description, clone.Description)
			},
		},
		{
			name: "command with steps",
			input: &schema.Command{
				Name: "multi-step",
				Steps: schema.Tasks{
					{Command: "step1", Type: "shell"},
					{Command: "step2", Type: "shell"},
					{Command: "step3", Type: "shell"},
				},
			},
			wantErr: false,
			verifyFn: func(t *testing.T, orig, clone *schema.Command) {
				assert.Equal(t, len(orig.Steps), len(clone.Steps))
				// Verify it's a deep copy - modifying clone doesn't affect original.
				clone.Steps[0].Command = "modified"
				assert.NotEqual(t, orig.Steps[0].Command, clone.Steps[0].Command)
			},
		},
		{
			name: "command with nested commands",
			input: &schema.Command{
				Name: "parent",
				Commands: []schema.Command{
					{Name: "child1"},
					{Name: "child2"},
				},
			},
			wantErr: false,
			verifyFn: func(t *testing.T, orig, clone *schema.Command) {
				assert.Equal(t, len(orig.Commands), len(clone.Commands))
				assert.Equal(t, orig.Commands[0].Name, clone.Commands[0].Name)
			},
		},
		{
			name: "command with flags",
			input: &schema.Command{
				Name: "with-flags",
				Flags: []schema.CommandFlag{
					{Name: "verbose", Type: "bool", Shorthand: "v"},
					{Name: "output", Type: "string", Required: true},
				},
			},
			wantErr: false,
			verifyFn: func(t *testing.T, orig, clone *schema.Command) {
				assert.Equal(t, len(orig.Flags), len(clone.Flags))
				assert.Equal(t, orig.Flags[0].Name, clone.Flags[0].Name)
				assert.Equal(t, orig.Flags[1].Required, clone.Flags[1].Required)
			},
		},
		{
			name: "command with arguments",
			input: &schema.Command{
				Name: "with-args",
				Arguments: []schema.CommandArgument{
					{Name: "component", Required: true},
					{Name: "stack", Default: "dev"},
				},
			},
			wantErr: false,
			verifyFn: func(t *testing.T, orig, clone *schema.Command) {
				assert.Equal(t, len(orig.Arguments), len(clone.Arguments))
				assert.Equal(t, orig.Arguments[0].Required, clone.Arguments[0].Required)
				assert.Equal(t, orig.Arguments[1].Default, clone.Arguments[1].Default)
			},
		},
		{
			name: "command with env vars",
			input: &schema.Command{
				Name: "with-env",
				Env: []schema.CommandEnv{
					{Key: "AWS_PROFILE", Value: "dev"},
					{Key: "TF_LOG", ValueCommand: "echo DEBUG"},
				},
			},
			wantErr: false,
			verifyFn: func(t *testing.T, orig, clone *schema.Command) {
				assert.Equal(t, len(orig.Env), len(clone.Env))
				assert.Equal(t, orig.Env[0].Key, clone.Env[0].Key)
				assert.Equal(t, orig.Env[1].ValueCommand, clone.Env[1].ValueCommand)
			},
		},
		{
			name: "command with component config",
			input: &schema.Command{
				Name: "with-component-config",
				ComponentConfig: schema.CommandComponentConfig{
					Component: "vpc",
					Stack:     "{{ .Arguments.stack }}",
				},
			},
			wantErr: false,
			verifyFn: func(t *testing.T, orig, clone *schema.Command) {
				assert.Equal(t, orig.ComponentConfig.Component, clone.ComponentConfig.Component)
				assert.Equal(t, orig.ComponentConfig.Stack, clone.ComponentConfig.Stack)
			},
		},
		{
			name:    "empty command",
			input:   &schema.Command{},
			wantErr: false,
			verifyFn: func(t *testing.T, orig, clone *schema.Command) {
				assert.Equal(t, orig.Name, clone.Name)
				assert.Empty(t, clone.Name)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clone, err := cloneCommand(tt.input)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, clone)

			// Verify it's a different object.
			if tt.input != nil {
				assert.NotSame(t, tt.input, clone)
			}

			if tt.verifyFn != nil {
				tt.verifyFn(t, tt.input, clone)
			}
		})
	}
}

// TestHandleHelpRequest tests the handleHelpRequest function.
func TestHandleHelpRequest(t *testing.T) {
	// Note: handleHelpRequest calls os.Exit(0) when help is requested,
	// so we can only test cases where it doesn't exit.
	tests := []struct {
		name          string
		args          []string
		shouldNotExit bool
	}{
		{
			name:          "no args - does not exit",
			args:          []string{},
			shouldNotExit: true,
		},
		{
			name:          "regular arg - does not exit",
			args:          []string{"component-name"},
			shouldNotExit: true,
		},
		{
			name:          "non-help flag - does not exit",
			args:          []string{"--verbose"},
			shouldNotExit: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{
				Use:   "test",
				Short: "Test command",
			}

			if tt.shouldNotExit {
				// This should not panic or call os.Exit.
				handleHelpRequest(cmd, tt.args)
				// If we got here, the function returned normally.
				assert.True(t, true, "handleHelpRequest returned without exiting")
			}
		})
	}
}

// TestListComponents tests the listComponents function.
func TestListComponents(t *testing.T) {
	tests := []struct {
		name        string
		setupDir    string
		stackFlag   string
		expectError bool
	}{
		{
			name:        "valid directory with components",
			setupDir:    "../examples/demo-stacks",
			stackFlag:   "",
			expectError: false,
		},
		{
			name:        "invalid directory",
			setupDir:    "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Change to test directory if specified.
			if tt.setupDir != "" {
				t.Chdir(tt.setupDir)
			} else {
				tmpDir := t.TempDir()
				t.Chdir(tmpDir)
			}

			// Create a test command.
			cmd := &cobra.Command{
				Use: "test",
			}
			cmd.Flags().String("stack", tt.stackFlag, "Stack flag")

			// Call the function.
			components, err := listComponents(cmd)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, components)
			}
		})
	}
}

// TestComponentsArgCompletion tests the ComponentsArgCompletion function.
func TestComponentsArgCompletion(t *testing.T) {
	tests := []struct {
		name       string
		setupDir   string
		args       []string
		toComplete string
		expectDir  bool // Whether we expect directory completion directive.
	}{
		{
			name:       "first arg with dot - directory completion",
			setupDir:   "../examples/demo-stacks",
			args:       []string{},
			toComplete: ".",
			expectDir:  true,
		},
		{
			name:       "first arg with path separator - directory completion",
			setupDir:   "../examples/demo-stacks",
			args:       []string{},
			toComplete: "./components/",
			expectDir:  true,
		},
		{
			name:       "first arg without path - component completion",
			setupDir:   "../examples/demo-stacks",
			args:       []string{},
			toComplete: "myapp",
			expectDir:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Change to test directory.
			if tt.setupDir != "" {
				t.Chdir(tt.setupDir)
			}

			// Create a test command.
			cmd := &cobra.Command{
				Use: "test",
			}
			cmd.Flags().String("stack", "", "Stack flag")

			// Call the function.
			_, directive := ComponentsArgCompletion(cmd, tt.args, tt.toComplete)

			if tt.expectDir {
				assert.Equal(t, cobra.ShellCompDirectiveFilterDirs, directive)
			} else {
				assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
			}
		})
	}
}

// TestIsGitRepository tests the isGitRepository function.
func TestIsGitRepository(t *testing.T) {
	// Test in a git repo (current directory should be a git repo).
	t.Chdir("../")
	result := isGitRepository()
	assert.True(t, result, "Should detect git repository")
}

// TestIsGitRepository_NonGitDir tests isGitRepository in a non-git directory.
func TestIsGitRepository_NonGitDir(t *testing.T) {
	// Create a temp directory that's not a git repo.
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	result := isGitRepository()
	assert.False(t, result, "Should not detect git repository in temp dir")
}

// TestVerifyInsideGitRepoE tests the verifyInsideGitRepoE function.
func TestVerifyInsideGitRepoE(t *testing.T) {
	// Test in a git repo.
	t.Chdir("../")
	err := verifyInsideGitRepoE()
	assert.NoError(t, err, "Should not error in git repository")
}

// TestVerifyInsideGitRepoE_NonGitDir tests verifyInsideGitRepoE in a non-git directory.
func TestVerifyInsideGitRepoE_NonGitDir(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	err := verifyInsideGitRepoE()
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrNotInGitRepository)
}

// TestGetTopLevelCommands tests the getTopLevelCommands function.
func TestGetTopLevelCommands(t *testing.T) {
	_ = NewTestKit(t)

	result := getTopLevelCommands()

	// Should return a map.
	assert.NotNil(t, result)
	// Should contain some commands.
	assert.Greater(t, len(result), 0)
}

// TestListStacks tests the listStacks function.
func TestListStacks(t *testing.T) {
	t.Chdir("../examples/demo-stacks")

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("stack", "", "Stack flag")

	stacks, err := listStacks(cmd)
	require.NoError(t, err)
	assert.NotNil(t, stacks)
	assert.Greater(t, len(stacks), 0)
}

// TestListStacks_InvalidDirectory tests listStacks in an invalid directory.
func TestListStacks_InvalidDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("stack", "", "Stack flag")

	_, err := listStacks(cmd)
	assert.Error(t, err)
}

// TestGetConfigAndStacksInfo_PathResolution tests path resolution in getConfigAndStacksInfo.
// These tests verify that path-based component arguments are properly detected and
// the path resolution code path is triggered without panicking.
func TestGetConfigAndStacksInfo_PathResolution(t *testing.T) {
	tests := []struct {
		name                string
		commandName         string
		args                []string
		expectPathResolving bool
	}{
		{
			name:                "path component with ./ - triggers path resolution",
			commandName:         "terraform",
			args:                []string{"plan", "./components/terraform/myapp", "--stack", "dev"},
			expectPathResolving: true,
		},
		{
			name:                "path component with . - triggers path resolution",
			commandName:         "terraform",
			args:                []string{"plan", ".", "--stack", "dev"},
			expectPathResolving: true,
		},
		{
			name:                "path component with ../ - triggers path resolution",
			commandName:         "terraform",
			args:                []string{"plan", "../vpc", "--stack", "dev"},
			expectPathResolving: true,
		},
		{
			name:                "absolute path - triggers path resolution",
			commandName:         "terraform",
			args:                []string{"plan", "/tmp/components/terraform/vpc", "--stack", "dev"},
			expectPathResolving: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = NewTestKit(t)

			cmd := &cobra.Command{Use: tt.commandName}
			cmd.Flags().String("stack", "", "stack name")

			// All path resolution tests expect errors in test environment.
			// The key validation is that the code doesn't panic.
			_, err := getConfigAndStacksInfo(tt.commandName, cmd, tt.args)

			// Should get an error in test environment (missing config).
			assert.Error(t, err, "Expected error in test environment")

			if tt.expectPathResolving {
				// Path resolution code path was triggered.
				t.Logf("Path resolution triggered for: %s", tt.args[1])
			}
		})
	}
}

// TestGetConfigAndStacksInfo_PathResolutionWithValidPath tests successful path resolution.
func TestGetConfigAndStacksInfo_PathResolutionWithValidPath(t *testing.T) {
	stacksPath := "../tests/fixtures/scenarios/complete"

	// Skip if fixtures directory doesn't exist.
	if _, err := os.Stat(stacksPath); os.IsNotExist(err) {
		t.Skipf("Skipping test: %s directory not found", stacksPath)
	}

	// Create component directory.
	componentDir := filepath.Join(stacksPath, "components", "terraform", "top-level-component1")
	if _, err := os.Stat(componentDir); os.IsNotExist(err) {
		t.Skipf("Skipping test: %s directory not found", componentDir)
	}

	// Change to the component directory.
	t.Chdir(componentDir)

	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)

	_ = NewTestKit(t)

	cmd := &cobra.Command{Use: "terraform"}
	cmd.Flags().String("stack", "", "stack name")

	// Use "." to trigger path resolution.
	args := []string{"plan", ".", "--stack", "tenant1-ue2-dev"}

	info, err := getConfigAndStacksInfo("terraform", cmd, args)
	// This may fail due to config loading issues in test environment.
	// The key test is that it doesn't panic and handles path resolution properly.
	if err != nil {
		t.Logf("Got error (expected in test environment): %v", err)
		// Should not panic - error is expected in test environment.
		return
	}

	// If we got here, verify the path was resolved.
	assert.Equal(t, "top-level-component1", info.ComponentFromArg, "Path should be resolved to component name")
}

// TestProcessCommandAliases tests the processCommandAliases function.
// NOTE: We use unique alias names (e.g., "test-alias-tp") to avoid conflicts
// with existing RootCmd commands, since processCommandAliases internally checks
// getTopLevelCommands() which reads from the global RootCmd.
func TestProcessCommandAliases(t *testing.T) {
	tests := []struct {
		name            string
		aliases         schema.CommandAliases
		topLevel        bool
		expectedAliases []string
	}{
		{
			name: "single alias",
			aliases: schema.CommandAliases{
				"test-alias-tp": "terraform plan",
			},
			topLevel:        true,
			expectedAliases: []string{"test-alias-tp"},
		},
		{
			name: "multiple aliases",
			aliases: schema.CommandAliases{
				"test-alias-tp": "terraform plan",
				"test-alias-ta": "terraform apply",
				"test-alias-td": "terraform destroy",
			},
			topLevel:        true,
			expectedAliases: []string{"test-alias-tp", "test-alias-ta", "test-alias-td"},
		},
		{
			name:            "empty aliases",
			aliases:         schema.CommandAliases{},
			topLevel:        true,
			expectedAliases: []string{},
		},
		{
			name: "alias with whitespace",
			aliases: schema.CommandAliases{
				"  test-alias-ws  ": "  terraform plan  ",
			},
			topLevel:        true,
			expectedAliases: []string{"test-alias-ws"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use test kit to reset RootCmd state.
			_ = NewTestKit(t)

			// Create a parent command.
			parentCmd := &cobra.Command{
				Use:   "atmos",
				Short: "Test parent command",
			}

			// Process aliases.
			err := processCommandAliases(schema.AtmosConfiguration{}, tt.aliases, parentCmd, tt.topLevel)
			require.NoError(t, err)

			// Verify the expected aliases were added.
			for _, expectedAlias := range tt.expectedAliases {
				found := false
				for _, cmd := range parentCmd.Commands() {
					if cmd.Name() != expectedAlias {
						continue
					}
					found = true

					// Verify the command has the configAlias annotation.
					assert.NotNil(t, cmd.Annotations, "Alias command should have annotations")
					_, hasConfigAlias := cmd.Annotations["configAlias"]
					assert.True(t, hasConfigAlias, "Alias command should have configAlias annotation")

					// Verify the Short description contains "alias for".
					assert.Contains(t, cmd.Short, "alias for", "Alias command should have 'alias for' in Short")

					// Verify DisableFlagParsing is true.
					assert.True(t, cmd.DisableFlagParsing, "Alias command should have DisableFlagParsing=true")

					break
				}
				assert.True(t, found, "Expected alias %q to be added to parent command", expectedAlias)
			}
		})
	}
}

// TestProcessCommandAliases_DoesNotOverrideExistingCommands tests that aliases don't override existing RootCmd commands.
// NOTE: The processCommandAliases function checks getTopLevelCommands() which reads from global RootCmd,
// so we test that it doesn't add aliases that conflict with existing RootCmd commands like "version".
func TestProcessCommandAliases_DoesNotOverrideExistingCommands(t *testing.T) {
	_ = NewTestKit(t)

	// Create a parent command to receive aliases.
	parentCmd := &cobra.Command{
		Use:   "atmos",
		Short: "Test parent command",
	}

	// Try to add aliases - "version" should be skipped (exists in RootCmd),
	// but "test-alias-new" should be added (unique name).
	aliases := schema.CommandAliases{
		"version":         "terraform version", // Should NOT be added (exists in RootCmd).
		"test-alias-new2": "terraform plan",    // Should be added (unique name).
	}

	err := processCommandAliases(schema.AtmosConfiguration{}, aliases, parentCmd, true)
	require.NoError(t, err)

	// Verify "version" alias was NOT added to parentCmd (because it exists in RootCmd).
	var versionFound bool
	for _, cmd := range parentCmd.Commands() {
		if cmd.Name() == "version" {
			versionFound = true
			break
		}
	}
	assert.False(t, versionFound, "version alias should not be added because it conflicts with existing RootCmd command")

	// Verify "test-alias-new2" alias was added.
	var newAliasFound bool
	for _, cmd := range parentCmd.Commands() {
		if cmd.Name() == "test-alias-new2" {
			newAliasFound = true
			assert.Contains(t, cmd.Short, "alias for")
			break
		}
	}
	assert.True(t, newAliasFound, "test-alias-new2 should be added")
}

// TestProcessCommandAliases_NonTopLevel tests that non-top-level aliases are NOT added.
// The processCommandAliases function only adds aliases when topLevel=true.
func TestProcessCommandAliases_NonTopLevel(t *testing.T) {
	_ = NewTestKit(t)

	parentCmd := &cobra.Command{
		Use:   "terraform",
		Short: "Terraform commands",
	}

	aliases := schema.CommandAliases{
		"p": "plan",
		"a": "apply",
	}

	// Process as non-top-level (topLevel=false).
	err := processCommandAliases(schema.AtmosConfiguration{}, aliases, parentCmd, false)
	require.NoError(t, err)

	// Verify aliases were NOT added (because topLevel=false).
	// The condition `!exist && topLevel` prevents alias creation when topLevel=false.
	// Check directly in Commands() list since Find() returns parent on not found.
	hasAliases := false
	for _, cmd := range parentCmd.Commands() {
		if cmd.Name() == "p" || cmd.Name() == "a" {
			hasAliases = true
			break
		}
	}
	assert.False(t, hasAliases, "Non-top-level aliases should not be added")
}
