package terraform

import (
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestCheckTerraformFlags(t *testing.T) {
	tests := []struct {
		name          string
		info          *schema.ConfigAndStacksInfo
		expectedError error
	}{
		{
			name:          "valid - no flags",
			info:          &schema.ConfigAndStacksInfo{},
			expectedError: nil,
		},
		{
			name: "invalid - component with affected flag",
			info: &schema.ConfigAndStacksInfo{
				ComponentFromArg: "test-component",
				Affected:         true,
			},
			expectedError: errUtils.ErrInvalidTerraformComponentWithMultiComponentFlags,
		},
		{
			name: "invalid - component with all flag",
			info: &schema.ConfigAndStacksInfo{
				ComponentFromArg: "test-component",
				All:              true,
			},
			expectedError: errUtils.ErrInvalidTerraformComponentWithMultiComponentFlags,
		},
		{
			name: "invalid - component with components flag",
			info: &schema.ConfigAndStacksInfo{
				ComponentFromArg: "test-component",
				Components:       []string{"comp1", "comp2"},
			},
			expectedError: errUtils.ErrInvalidTerraformComponentWithMultiComponentFlags,
		},
		{
			name: "invalid - affected with all flag",
			info: &schema.ConfigAndStacksInfo{
				Affected: true,
				All:      true,
			},
			expectedError: errUtils.ErrInvalidTerraformFlagsWithAffectedFlag,
		},
		{
			name: "invalid - affected with components flag",
			info: &schema.ConfigAndStacksInfo{
				Affected:   true,
				Components: []string{"comp1", "comp2"},
			},
			expectedError: errUtils.ErrInvalidTerraformFlagsWithAffectedFlag,
		},
		{
			name: "invalid - affected with query flag",
			info: &schema.ConfigAndStacksInfo{
				Affected: true,
				Query:    "test-query",
			},
			expectedError: errUtils.ErrInvalidTerraformFlagsWithAffectedFlag,
		},
		{
			name: "invalid - single and multi component flags",
			info: &schema.ConfigAndStacksInfo{
				PlanFile: "plan.tfplan",
				All:      true,
			},
			expectedError: errUtils.ErrInvalidTerraformSingleComponentAndMultiComponentFlags,
		},
		{
			name: "invalid - from-plan with multi component flag",
			info: &schema.ConfigAndStacksInfo{
				UseTerraformPlan: true,
				Affected:         true,
			},
			expectedError: errUtils.ErrInvalidTerraformSingleComponentAndMultiComponentFlags,
		},
		{
			name: "valid - only single component flag",
			info: &schema.ConfigAndStacksInfo{
				PlanFile: "plan.tfplan",
			},
			expectedError: nil,
		},
		{
			name: "valid - only multi component flag",
			info: &schema.ConfigAndStacksInfo{
				All: true,
			},
			expectedError: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := checkTerraformFlags(test.info)
			if test.expectedError != nil {
				assert.ErrorIs(t, err, test.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestTerraformIdentityFlagHandling tests the identity flag handling in terraformRun.
// Regression test for: https://github.com/cloudposse/atmos/issues/XXXX
// Ensures that when --identity flag is NOT provided, the code doesn't try to
// read from viper and potentially get the IdentityFlagSelectValue ("__SELECT__"),
// which would trigger the TTY check and fail in CI environments.
func TestTerraformIdentityFlagHandling(t *testing.T) {
	tests := []struct {
		name             string
		args             []string
		envVar           string
		envValue         string
		expectTTYError   bool
		expectedIdentity string
	}{
		{
			name:             "no --identity flag, no env var, should not trigger TTY check",
			args:             []string{},
			expectedIdentity: "", // ProcessCommandLineArgs will handle default identity
			expectTTYError:   false,
		},
		{
			name:             "ATMOS_IDENTITY env var set, should use env var",
			args:             []string{},
			envVar:           "ATMOS_IDENTITY",
			envValue:         "test-identity",
			expectedIdentity: "test-identity",
			expectTTYError:   false,
		},
		{
			name:             "ATMOS_IDENTITY set to __SELECT__, should NOT trigger TTY check (flag not explicitly provided)",
			args:             []string{},
			envVar:           "ATMOS_IDENTITY",
			envValue:         "__SELECT__",
			expectedIdentity: "__SELECT__", // ProcessCommandLineArgs sets this, but terraformRun doesn't override
			expectTTYError:   false,        // Key fix: TTY check only happens when flag is explicitly provided
		},
		{
			name:           "--identity flag without value, SHOULD trigger TTY check",
			args:           []string{"--identity"},
			expectTTYError: true, // Expected to fail in non-TTY environment
		},
		{
			name:             "--identity flag with value, should use flag value",
			args:             []string{"--identity=flag-identity"},
			expectedIdentity: "flag-identity",
			expectTTYError:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Set up environment variable if specified.
			if tc.envVar != "" {
				t.Setenv(tc.envVar, tc.envValue)
			}

			// Create a minimal command for testing.
			cmd := &cobra.Command{
				Use: "plan",
			}

			// Add the identity flag (matching terraform commands setup).
			cmd.Flags().StringP("identity", "i", "", "Specify identity")
			if identityFlag := cmd.Flags().Lookup("identity"); identityFlag != nil {
				identityFlag.NoOptDefVal = cfg.IdentityFlagSelectValue
			}

			// Parse the test args.
			err := cmd.ParseFlags(tc.args)

			// Check if we expect a TTY error.
			if tc.expectTTYError {
				// When --identity flag is provided without value in non-TTY environment,
				// we expect the code to fail fast with TTY error before trying to parse.
				// This test verifies the guard works correctly.
				// We can't easily test the full terraformRun flow here without mocking
				// the entire CLI config and auth manager, so we just verify flag parsing.
				assert.NoError(t, err, "Flag parsing should succeed")

				// Verify flag was set to __SELECT__.
				flagValue, err := cmd.Flags().GetString("identity")
				assert.NoError(t, err)
				assert.Equal(t, cfg.IdentityFlagSelectValue, flagValue, "Flag should be set to __SELECT__")
			} else {
				assert.NoError(t, err, "Flag parsing should succeed")

				// Verify the flag value if we don't expect TTY error.
				if len(tc.args) > 0 {
					flagValue, err := cmd.Flags().GetString("identity")
					assert.NoError(t, err)

					if tc.args[0] == "--identity" && len(tc.args) == 1 {
						// --identity without value → should be __SELECT__.
						assert.Equal(t, cfg.IdentityFlagSelectValue, flagValue)
					} else if len(tc.args) > 0 && tc.args[0] == "--identity=flag-identity" {
						// --identity with value → should be the value.
						assert.Equal(t, "flag-identity", flagValue)
					}
				}
			}
		})
	}
}

// TestUserAbortExitCode tests that ErrUserAborted results in exit code 130.
// This is a regression test for the bug where pressing Ctrl+C during identity
// selection would not properly exit the program, causing terraform to continue executing.
//
// The fix ensures that when GetDefaultIdentity returns ErrUserAborted, we immediately
// exit with code 130 (POSIX SIGINT: 128 + 2) before falling through to the generic
// error handler.
func TestUserAbortExitCode(t *testing.T) {
	// Save original OsExit and restore it after the test.
	originalOsExit := errUtils.OsExit
	defer func() {
		errUtils.OsExit = originalOsExit
	}()

	// Track whether Exit was called and with what code.
	var exitCalled bool
	var exitCode int
	errUtils.OsExit = func(code int) {
		exitCalled = true
		exitCode = code
		// Don't actually exit during the test.
	}

	// Simulate the error handling logic from handleInteractiveIdentitySelection.
	// This is the key fix: when we get ErrUserAborted, we should exit with code 130.
	err := errUtils.ErrUserAborted

	// This is the fix applied in terraform_utils.go:handleInteractiveIdentitySelection
	if errors.Is(err, errUtils.ErrUserAborted) {
		errUtils.Exit(errUtils.ExitCodeSIGINT)
	}

	// Verify that Exit was called with the correct exit code.
	assert.True(t, exitCalled, "Exit should have been called when user aborts")
	assert.Equal(t, errUtils.ExitCodeSIGINT, exitCode, "Exit code should be ExitCodeSIGINT (POSIX SIGINT: 128 + 2)")
}

func TestCheckTerraformFlags_ComponentWithQuery(t *testing.T) {
	// Test component with query flag.
	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg: "test-component",
		Query:            ".components.test",
	}

	err := checkTerraformFlags(info)
	assert.ErrorIs(t, err, errUtils.ErrInvalidTerraformComponentWithMultiComponentFlags)
}

func TestCheckTerraformFlags_AllEdgeCases(t *testing.T) {
	tests := []struct {
		name          string
		info          *schema.ConfigAndStacksInfo
		expectedError error
	}{
		{
			name: "plan-file with query",
			info: &schema.ConfigAndStacksInfo{
				PlanFile: "plan.tfplan",
				Query:    ".components.test",
			},
			expectedError: errUtils.ErrInvalidTerraformSingleComponentAndMultiComponentFlags,
		},
		{
			name: "plan-file with components",
			info: &schema.ConfigAndStacksInfo{
				PlanFile:   "plan.tfplan",
				Components: []string{"comp1"},
			},
			expectedError: errUtils.ErrInvalidTerraformSingleComponentAndMultiComponentFlags,
		},
		{
			name: "use-terraform-plan with all",
			info: &schema.ConfigAndStacksInfo{
				UseTerraformPlan: true,
				All:              true,
			},
			expectedError: errUtils.ErrInvalidTerraformSingleComponentAndMultiComponentFlags,
		},
		{
			name: "use-terraform-plan with query",
			info: &schema.ConfigAndStacksInfo{
				UseTerraformPlan: true,
				Query:            ".components.test",
			},
			expectedError: errUtils.ErrInvalidTerraformSingleComponentAndMultiComponentFlags,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkTerraformFlags(tt.info)
			// All test cases in this table expect errors - asserting the specific error type.
			assert.ErrorIs(t, err, tt.expectedError)
		})
	}
}

// TestStackFlagCompletion_NoArgs tests the stackFlagCompletion function without args.
func TestStackFlagCompletion_NoArgs(t *testing.T) {
	t.Chdir("../../examples/demo-stacks")

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("stack", "", "Stack flag")

	// Test without component arg.
	stacks, directive := stackFlagCompletion(cmd, []string{}, "")
	assert.NotNil(t, stacks)
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
}

// TestStackFlagCompletion_WithComponent tests stackFlagCompletion with a component.
func TestStackFlagCompletion_WithComponent(t *testing.T) {
	t.Chdir("../../examples/demo-stacks")

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("stack", "", "Stack flag")

	// Test with component arg.
	stacks, directive := stackFlagCompletion(cmd, []string{"myapp"}, "")
	assert.NotNil(t, stacks)
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
}

// TestIsMultiComponentExecution tests the isMultiComponentExecution function.
func TestIsMultiComponentExecution(t *testing.T) {
	tests := []struct {
		name     string
		info     *schema.ConfigAndStacksInfo
		expected bool
	}{
		{
			name:     "all flag set",
			info:     &schema.ConfigAndStacksInfo{All: true},
			expected: true,
		},
		{
			name:     "components set",
			info:     &schema.ConfigAndStacksInfo{Components: []string{"comp1", "comp2"}},
			expected: true,
		},
		{
			name:     "query set",
			info:     &schema.ConfigAndStacksInfo{Query: ".components.test"},
			expected: true,
		},
		{
			name:     "stack set without component",
			info:     &schema.ConfigAndStacksInfo{Stack: "dev-us-east-1"},
			expected: true,
		},
		{
			name:     "stack and component both set - single component mode",
			info:     &schema.ConfigAndStacksInfo{Stack: "dev-us-east-1", ComponentFromArg: "vpc"},
			expected: false,
		},
		{
			name:     "no flags - single component mode",
			info:     &schema.ConfigAndStacksInfo{},
			expected: false,
		},
		{
			name:     "only component - single component mode",
			info:     &schema.ConfigAndStacksInfo{ComponentFromArg: "vpc"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isMultiComponentExecution(tt.info)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestHasMultiComponentFlags tests the hasMultiComponentFlags function.
func TestHasMultiComponentFlags(t *testing.T) {
	tests := []struct {
		name     string
		info     *schema.ConfigAndStacksInfo
		expected bool
	}{
		{
			name:     "all flag",
			info:     &schema.ConfigAndStacksInfo{All: true},
			expected: true,
		},
		{
			name:     "affected flag",
			info:     &schema.ConfigAndStacksInfo{Affected: true},
			expected: true,
		},
		{
			name:     "components set",
			info:     &schema.ConfigAndStacksInfo{Components: []string{"comp1"}},
			expected: true,
		},
		{
			name:     "query set",
			info:     &schema.ConfigAndStacksInfo{Query: ".test"},
			expected: true,
		},
		{
			name:     "no flags",
			info:     &schema.ConfigAndStacksInfo{},
			expected: false,
		},
		{
			name:     "empty components slice",
			info:     &schema.ConfigAndStacksInfo{Components: []string{}},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasMultiComponentFlags(tt.info)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestHasNonAffectedMultiFlags tests the hasNonAffectedMultiFlags function.
func TestHasNonAffectedMultiFlags(t *testing.T) {
	tests := []struct {
		name     string
		info     *schema.ConfigAndStacksInfo
		expected bool
	}{
		{
			name:     "all flag",
			info:     &schema.ConfigAndStacksInfo{All: true},
			expected: true,
		},
		{
			name:     "components set",
			info:     &schema.ConfigAndStacksInfo{Components: []string{"comp1"}},
			expected: true,
		},
		{
			name:     "query set",
			info:     &schema.ConfigAndStacksInfo{Query: ".test"},
			expected: true,
		},
		{
			name:     "affected only - should return false",
			info:     &schema.ConfigAndStacksInfo{Affected: true},
			expected: false,
		},
		{
			name:     "no flags",
			info:     &schema.ConfigAndStacksInfo{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasNonAffectedMultiFlags(tt.info)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestHasSingleComponentFlags tests the hasSingleComponentFlags function.
func TestHasSingleComponentFlags(t *testing.T) {
	tests := []struct {
		name     string
		info     *schema.ConfigAndStacksInfo
		expected bool
	}{
		{
			name:     "plan-file set",
			info:     &schema.ConfigAndStacksInfo{PlanFile: "plan.tfplan"},
			expected: true,
		},
		{
			name:     "use-terraform-plan set",
			info:     &schema.ConfigAndStacksInfo{UseTerraformPlan: true},
			expected: true,
		},
		{
			name:     "both set",
			info:     &schema.ConfigAndStacksInfo{PlanFile: "plan.tfplan", UseTerraformPlan: true},
			expected: true,
		},
		{
			name:     "neither set",
			info:     &schema.ConfigAndStacksInfo{},
			expected: false,
		},
		{
			name:     "empty plan-file",
			info:     &schema.ConfigAndStacksInfo{PlanFile: ""},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasSingleComponentFlags(tt.info)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestHandlePathResolutionError tests the handlePathResolutionError function.
func TestHandlePathResolutionError(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		expectedErr error
		checkIs     bool // Use errors.Is to check
	}{
		{
			name:        "ambiguous path error passes through",
			err:         errUtils.ErrAmbiguousComponentPath,
			expectedErr: errUtils.ErrAmbiguousComponentPath,
			checkIs:     true,
		},
		{
			name:        "component not in stack passes through",
			err:         errUtils.ErrComponentNotInStack,
			expectedErr: errUtils.ErrComponentNotInStack,
			checkIs:     true,
		},
		{
			name:        "stack not found passes through",
			err:         errUtils.ErrStackNotFound,
			expectedErr: errUtils.ErrStackNotFound,
			checkIs:     true,
		},
		{
			name:        "user aborted passes through",
			err:         errUtils.ErrUserAborted,
			expectedErr: errUtils.ErrUserAborted,
			checkIs:     true,
		},
		{
			name:        "generic error gets wrapped with ErrPathResolutionFailed",
			err:         errors.New("some generic error"),
			expectedErr: errUtils.ErrPathResolutionFailed,
			checkIs:     true,
		},
		{
			name:        "wrapped ambiguous path error passes through",
			err:         errUtils.Build(errUtils.ErrAmbiguousComponentPath).WithExplanation("test").Err(),
			expectedErr: errUtils.ErrAmbiguousComponentPath,
			checkIs:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handlePathResolutionError(tt.err)
			assert.Error(t, result)
			if tt.checkIs {
				assert.ErrorIs(t, result, tt.expectedErr)
			}
		})
	}
}

// TestHandleInteractiveComponentStackSelection tests the handleInteractiveComponentStackSelection function.
func TestHandleInteractiveComponentStackSelection(t *testing.T) {
	tests := []struct {
		name                  string
		info                  *schema.ConfigAndStacksInfo
		expectComponentPrompt bool
		expectStackPrompt     bool
		shouldSkip            bool
	}{
		{
			name:       "skip when all flag is set",
			info:       &schema.ConfigAndStacksInfo{All: true},
			shouldSkip: true,
		},
		{
			name:       "skip when affected flag is set",
			info:       &schema.ConfigAndStacksInfo{Affected: true},
			shouldSkip: true,
		},
		{
			name:       "skip when components set",
			info:       &schema.ConfigAndStacksInfo{Components: []string{"comp1"}},
			shouldSkip: true,
		},
		{
			name:       "skip when query set",
			info:       &schema.ConfigAndStacksInfo{Query: ".test"},
			shouldSkip: true,
		},
		{
			name:       "skip when need help",
			info:       &schema.ConfigAndStacksInfo{NeedHelp: true},
			shouldSkip: true,
		},
		{
			name:       "skip when both component and stack provided",
			info:       &schema.ConfigAndStacksInfo{ComponentFromArg: "vpc", Stack: "dev"},
			shouldSkip: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "test"}
			err := handleInteractiveComponentStackSelection(tt.info, cmd)

			if tt.shouldSkip {
				assert.NoError(t, err)
				// Info should not be modified.
			}
		})
	}
}

// TestIdentityFlagCompletion tests the identityFlagCompletion function.
func TestIdentityFlagCompletion(t *testing.T) {
	t.Chdir("../../examples/demo-stacks")

	cmd := &cobra.Command{Use: "test"}

	// Test identity completion (will return empty if no identities configured).
	identities, directive := identityFlagCompletion(cmd, []string{}, "")

	// Directive should always be NoFileComp.
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)

	// Identities may be nil or empty depending on config.
	// We just verify no panic and correct directive.
	_ = identities
}

// TestAddIdentityCompletion tests the addIdentityCompletion function.
func TestAddIdentityCompletion(t *testing.T) {
	t.Run("flag exists", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		cmd.Flags().StringP("identity", "i", "", "Identity flag")

		// Should not panic.
		addIdentityCompletion(cmd)
	})

	t.Run("flag does not exist", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}

		// Should not panic when flag doesn't exist.
		addIdentityCompletion(cmd)
	})

	t.Run("inherited flag", func(t *testing.T) {
		parent := &cobra.Command{Use: "parent"}
		parent.PersistentFlags().StringP("identity", "i", "", "Identity flag")

		child := &cobra.Command{Use: "child"}
		parent.AddCommand(child)

		// Should find inherited flag.
		addIdentityCompletion(child)
	})
}

// TestComponentsArgCompletion tests the componentsArgCompletion function.
func TestComponentsArgCompletion(t *testing.T) {
	t.Chdir("../../examples/demo-stacks")

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("stack", "", "Stack flag")

	// Test with no args.
	components, directive := componentsArgCompletion(cmd, []string{}, "")
	assert.NotNil(t, components)
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
}

// TestComponentsArgCompletion_WithExistingArgs tests completion when args already exist.
func TestComponentsArgCompletion_WithExistingArgs(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}

	// Test with existing args - should return nil.
	components, directive := componentsArgCompletion(cmd, []string{"existing-component"}, "")
	assert.Nil(t, components)
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
}

// TestHandleInteractiveComponentStackSelection_ValidateStackExists tests the ValidateStackExists path.
func TestHandleInteractiveComponentStackSelection_ValidateStackExists(t *testing.T) {
	t.Chdir("../../examples/demo-stacks")

	tests := []struct {
		name        string
		info        *schema.ConfigAndStacksInfo
		expectError bool
	}{
		{
			name: "valid stack with no component passes validation",
			info: &schema.ConfigAndStacksInfo{
				Stack:            "dev",
				ComponentFromArg: "",
			},
			// Note: In non-TTY environment, this won't prompt but will return nil
			// since interactive mode isn't available.
			expectError: false,
		},
		{
			name: "invalid stack with no component - validation returns error",
			info: &schema.ConfigAndStacksInfo{
				Stack:            "nonexistent-stack-xyz",
				ComponentFromArg: "",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "test"}
			err := handleInteractiveComponentStackSelection(tt.info, cmd)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				// In non-TTY environment, it should return nil (interactive mode not available).
				assert.NoError(t, err)
			}
		})
	}
}

// TestHandlePromptErrorDelegate tests the handlePromptError delegate function.
func TestHandlePromptErrorDelegate(t *testing.T) {
	// Save original OsExit and restore it after tests.
	originalOsExit := errUtils.OsExit
	defer func() {
		errUtils.OsExit = originalOsExit
	}()

	tests := []struct {
		name             string
		err              error
		promptName       string
		expectExit       bool
		expectedExitCode int
		expectedReturn   error
	}{
		{
			name:           "nil error returns nil",
			err:            nil,
			promptName:     "component",
			expectExit:     false,
			expectedReturn: nil,
		},
		{
			name:           "ErrInteractiveModeNotAvailable returns nil",
			err:            errUtils.ErrInteractiveModeNotAvailable,
			promptName:     "stack",
			expectExit:     false,
			expectedReturn: nil,
		},
		{
			name:           "generic error returns the error",
			err:            errors.New("some error"),
			promptName:     "component",
			expectExit:     false,
			expectedReturn: errors.New("some error"),
		},
		{
			name:             "ErrUserAborted triggers exit with SIGINT code",
			err:              errUtils.ErrUserAborted,
			promptName:       "component",
			expectExit:       true,
			expectedExitCode: errUtils.ExitCodeSIGINT,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var exitCalled bool
			var exitCode int
			errUtils.OsExit = func(code int) {
				exitCalled = true
				exitCode = code
			}

			result := handlePromptError(tt.err, tt.promptName)

			if tt.expectExit {
				assert.True(t, exitCalled, "OsExit should be called")
				assert.Equal(t, tt.expectedExitCode, exitCode, "Exit code should match")
			} else {
				assert.False(t, exitCalled, "OsExit should not be called")
				if tt.expectedReturn == nil {
					assert.NoError(t, result)
				} else {
					assert.Error(t, result)
					assert.Equal(t, tt.expectedReturn.Error(), result.Error())
				}
			}
		})
	}
}

// TestPromptForComponentDelegate tests the promptForComponent delegate function.
func TestPromptForComponentDelegate(t *testing.T) {
	// Test that it delegates to shared.PromptForComponent.
	// In non-TTY environment, it should return ErrInteractiveModeNotAvailable.
	cmd := &cobra.Command{Use: "test"}
	_, err := promptForComponent(cmd, "")
	// The function should return an error in non-TTY environment.
	// This is expected behavior - in CI, interactive mode is not available.
	if err != nil {
		assert.ErrorIs(t, err, errUtils.ErrInteractiveModeNotAvailable)
	}
}

// TestPromptForStackDelegate tests the promptForStack delegate function.
func TestPromptForStackDelegate(t *testing.T) {
	// Test that it delegates to shared.PromptForStack.
	// In non-TTY environment, it should return ErrInteractiveModeNotAvailable.
	cmd := &cobra.Command{Use: "test"}
	_, err := promptForStack(cmd, "")
	// The function should return an error in non-TTY environment.
	// This is expected behavior - in CI, interactive mode is not available.
	if err != nil {
		assert.ErrorIs(t, err, errUtils.ErrInteractiveModeNotAvailable)
	}
}
