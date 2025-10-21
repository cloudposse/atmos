package cmd

import (
	"os"
	"os/exec"
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

func TestExtractTrailingArgs(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		osArgs         []string
		expectedArgs   []string
		expectedString string
	}{
		{
			name:           "no trailing args",
			args:           []string{"arg1", "arg2"},
			osArgs:         []string{"program", "arg1", "arg2"},
			expectedArgs:   []string{"arg1", "arg2"},
			expectedString: "",
		},
		{
			name:           "with trailing args",
			args:           []string{"arg1", "--", "trail1", "trail2"},
			osArgs:         []string{"program", "arg1", "--", "trail1", "trail2"},
			expectedArgs:   []string{"arg1"},
			expectedString: "trail1 trail2",
		},
		{
			name:           "double dash at end",
			args:           []string{"arg1", "--"},
			osArgs:         []string{"program", "arg1", "--"},
			expectedArgs:   []string{"arg1"},
			expectedString: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args, str := extractTrailingArgs(tt.args, tt.osArgs)
			assert.Equal(t, tt.expectedArgs, args)
			assert.Equal(t, tt.expectedString, str)
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

// TestBuildMissingAtmosConfigError tests the buildMissingAtmosConfigError function.
// This function builds rich error messages when the Atmos stacks directory doesn't exist.
// The error includes context, hints, and proper exit codes.
func TestBuildMissingAtmosConfigError(t *testing.T) {
	tests := []struct {
		name        string
		atmosConfig schema.AtmosConfiguration
	}{
		{
			name: "default config missing",
			atmosConfig: schema.AtmosConfiguration{
				BasePath:               "/test",
				StacksBaseAbsolutePath: "/test/stacks",
				CliConfigPath:          "/test/atmos.yaml",
				Stacks: schema.Stacks{
					BasePath: "stacks",
				},
				Default: true, // Tests the code path when atmos.yaml was not found and default config is used.
			},
		},
		{
			name: "custom config with invalid paths",
			atmosConfig: schema.AtmosConfiguration{
				BasePath:               "/custom",
				StacksBaseAbsolutePath: "/custom/my-stacks",
				CliConfigPath:          "/custom/atmos.yaml",
				Stacks: schema.Stacks{
					BasePath: "my-stacks",
				},
				Default: false, // Tests the code path when atmos.yaml was found but has invalid paths.
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := buildMissingAtmosConfigError(&tt.atmosConfig)
			if !assert.Error(t, err) {
				return
			}

			// Verify exit code is 2 (usage error).
			exitCode := errUtils.GetExitCode(err)
			assert.Equal(t, 2, exitCode, "Exit code should be 2 for usage errors")

			// Verify it's the right base error so callers can use errors.Is() checks.
			assert.ErrorIs(t, err, errUtils.ErrStacksDirectoryDoesNotExist)
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
	completions, directive := stackFlagCompletion(cmd, []string{}, "")

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
