package cmd

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
	"github.com/cloudposse/atmos/pkg/telemetry"
)

func TestDescribeComponentCmd_Error(t *testing.T) {
	tk := NewTestKit(t)

	// This test verifies that calling the command with no arguments returns an error.
	// The command requires exactly one argument (the component name).
	err := describeComponentCmd.RunE(describeComponentCmd, []string{})
	assert.Error(tk, err, "describe component command should return an error when called with no parameters")
}

// TestDescribeComponentCmd_StackNotCobraRequired is a regression test for the
// missing-stack interactive prompt never firing: describe_component.go used to call
// describeComponentCmd.MarkPersistentFlagRequired("stack"), which is Cobra's own
// required-flag validation and runs BEFORE RunE -- unconditionally producing
// Cobra's generic "required flag(s) \"stack\" not set" error and never giving
// resolveDescribeComponentStack's interactive prompt a chance to run, even in a
// real terminal. This asserts the "stack" flag no longer carries Cobra's
// required-flag annotation.
func TestDescribeComponentCmd_StackNotCobraRequired(t *testing.T) {
	stackFlag := describeComponentCmd.PersistentFlags().Lookup("stack")
	require.NotNil(t, stackFlag, "stack flag should be registered")

	required, ok := stackFlag.Annotations[cobra.BashCompOneRequiredFlag]
	if ok {
		assert.NotEqual(t, []string{"true"}, required,
			"stack flag must not be marked Cobra-required, or the interactive "+
				"missing-stack prompt never gets a chance to run before Cobra's own "+
				"required-flag validation rejects the command")
	}
}

// TestGetRunnableDescribeComponentCmd_MissingStackTriggersPrompt is a regression
// test proving `atmos describe component <name>` with no --stack, in a
// mocked-interactive context, attempts the interactive "Choose a stack" prompt
// path (resolveDescribeComponentStack -> flags.PromptForMissingRequired ->
// describeComponentStackCompletion) instead of failing before ever reaching RunE.
// It also proves that when the prompt yields no selection (e.g. no matching
// stacks), the command still fails clearly via errUtils.ErrMissingStack rather
// than silently proceeding with an empty stack.
func TestGetRunnableDescribeComponentCmd_MissingStackTriggersPrompt(t *testing.T) {
	tk := NewTestKit(t)
	viper.Reset()

	preserved := telemetry.PreserveCIEnvVars()
	defer telemetry.RestoreCIEnvVars(preserved)
	tk.Setenv("ATMOS_FORCE_TTY", "true")
	viper.Set("interactive", true)

	testCmd := &cobra.Command{Use: "component"}
	testCmd.Flags().String("stack", "", "")
	testCmd.Flags().String("format", "yaml", "")
	testCmd.Flags().String("file", "", "")
	testCmd.Flags().Bool("process-templates", true, "")
	testCmd.Flags().Bool("process-functions", true, "")
	testCmd.Flags().Bool("use-mocks", false, "")
	testCmd.Flags().String("query", "", "")
	testCmd.Flags().StringSlice("skip", nil, "")
	testCmd.Flags().Bool("provenance", false, "")

	var capturedFlagName string
	var capturedArgs []string
	originalCompletion := describeComponentStackCompletion
	describeComponentStackCompletion = func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		capturedFlagName = "stack"
		capturedArgs = append([]string{}, args...)
		// No matching stacks -- forces the graceful "let the caller validate"
		// fallback instead of needing a real TTY form to complete a selection.
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	defer func() { describeComponentStackCompletion = originalCompletion }()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockExec := exec.NewMockDescribeComponentCmdExec(ctrl)
	mockExec.EXPECT().ExecuteDescribeComponentCmd(gomock.Any()).Times(0)

	run := getRunnableDescribeComponentCmd(getRunnableDescribeComponentCmdProps{
		checkAtmosConfigE: func(opts ...AtmosValidateOption) error { return nil },
		initCliConfig: func(info schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error) {
			return schema.AtmosConfiguration{}, nil
		},
		isExplicitComponentPath: func(component string) bool { return false },
		resolveComponentFromPath: func(atmosConfig *schema.AtmosConfiguration, component, stack string) (string, error) {
			return component, nil
		},
		executeDescribeComponent: func(params *exec.ExecuteDescribeComponentParams) (map[string]any, error) {
			return nil, nil
		},
		newDescribeComponentExec: mockExec,
	})

	err := run(testCmd, []string{"vpc"})

	assert.Equal(tk, "stack", capturedFlagName, "the missing-stack prompt path must be attempted (completion function invoked)")
	assert.Equal(tk, []string{"vpc"}, capturedArgs, "the completion function must receive the component positional arg for filtering")
	require.ErrorIs(tk, err, errUtils.ErrMissingStack,
		"a still-missing stack after the prompt attempt must fail with the standard ErrMissingStack, not a Cobra required-flag error or a silent empty-stack execution")
}

// TestResolveDescribeComponentStack_PromptSelectsStack drives resolveDescribeComponentStack's
// real success path: PromptForMissingRequired -> flags.PromptForValue -> the actual Huh form
// body (title, options), run in accessible mode via flags.SetFormRunnerForTest so it doesn't
// need a live TTY. This covers the `return stackFlag.Value.Set(selected)` line -- proving a
// selection made through the real prompt is written back onto the command's own "stack" flag.
func TestResolveDescribeComponentStack_PromptSelectsStack(t *testing.T) {
	originalInteractive := viper.GetBool("interactive")
	defer viper.Set("interactive", originalInteractive)

	preserved := telemetry.PreserveCIEnvVars()
	defer telemetry.RestoreCIEnvVars(preserved)
	t.Setenv("ATMOS_FORCE_TTY", "true")
	viper.Set("interactive", true)
	require.True(t, flags.IsInteractive(), "test setup must actually reach the interactive branch")

	restoreRunner := flags.SetFormRunnerForTest(func(f *huh.Form) error {
		// "1\n" selects the first listed option ("dev") via Huh's accessible-mode
		// numbered prompt -- this exercises the real title/options rendering and
		// selection logic, not a stub.
		return f.WithAccessible(true).WithInput(strings.NewReader("1\n")).WithOutput(io.Discard).Run()
	})
	defer restoreRunner()

	originalCompletion := describeComponentStackCompletion
	var capturedArgs []string
	describeComponentStackCompletion = func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		capturedArgs = append([]string{}, args...)
		return []string{"dev", "staging"}, cobra.ShellCompDirectiveNoFileComp
	}
	defer func() { describeComponentStackCompletion = originalCompletion }()

	cmd := &cobra.Command{Use: "component"}
	cmd.Flags().String("stack", "", "")

	err := resolveDescribeComponentStack(cmd, []string{"vpc"})
	require.NoError(t, err)
	assert.Equal(t, []string{"vpc"}, capturedArgs, "completion function must receive the component positional arg")
	assert.Equal(t, "dev", cmd.Flags().Lookup("stack").Value.String(),
		"the selection made through the real prompt must be written back onto the stack flag")
}

// TestResolveDescribeComponentStack_PromptFormError covers resolveDescribeComponentStack's
// error-wrapping branch: when the underlying prompt fails (e.g. the Huh form itself errors),
// resolveDescribeComponentStack must wrap it with "prompt for --stack" context rather than
// silently discarding it or panicking.
func TestResolveDescribeComponentStack_PromptFormError(t *testing.T) {
	originalInteractive := viper.GetBool("interactive")
	defer viper.Set("interactive", originalInteractive)

	preserved := telemetry.PreserveCIEnvVars()
	defer telemetry.RestoreCIEnvVars(preserved)
	t.Setenv("ATMOS_FORCE_TTY", "true")
	viper.Set("interactive", true)
	require.True(t, flags.IsInteractive(), "test setup must actually reach the interactive branch")

	boom := errors.New("form boom")
	restoreRunner := flags.SetFormRunnerForTest(func(*huh.Form) error { return boom })
	defer restoreRunner()

	originalCompletion := describeComponentStackCompletion
	describeComponentStackCompletion = func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"dev", "staging"}, cobra.ShellCompDirectiveNoFileComp
	}
	defer func() { describeComponentStackCompletion = originalCompletion }()

	cmd := &cobra.Command{Use: "component"}
	cmd.Flags().String("stack", "", "")

	err := resolveDescribeComponentStack(cmd, []string{"vpc"})
	require.Error(t, err)
	assert.ErrorIs(t, err, boom)
	assert.Contains(t, err.Error(), "prompt for --stack", "error must be wrapped with the --stack prompt context")
}

// TestGetRunnableDescribeComponentCmd_StackPromptErrorPropagates covers the early-return
// branch in getRunnableDescribeComponentCmd's RunE closure: when resolveDescribeComponentStack
// itself fails (as opposed to yielding no selection), the command must return that error
// immediately rather than continuing on to parse flags or execute the describe.
func TestGetRunnableDescribeComponentCmd_StackPromptErrorPropagates(t *testing.T) {
	tk := NewTestKit(t)
	viper.Reset()

	preserved := telemetry.PreserveCIEnvVars()
	defer telemetry.RestoreCIEnvVars(preserved)
	tk.Setenv("ATMOS_FORCE_TTY", "true")
	viper.Set("interactive", true)

	testCmd := &cobra.Command{Use: "component"}
	testCmd.Flags().String("stack", "", "")
	testCmd.Flags().String("format", "yaml", "")
	testCmd.Flags().String("file", "", "")
	testCmd.Flags().Bool("process-templates", true, "")
	testCmd.Flags().Bool("process-functions", true, "")
	testCmd.Flags().Bool("use-mocks", false, "")
	testCmd.Flags().String("query", "", "")
	testCmd.Flags().StringSlice("skip", nil, "")
	testCmd.Flags().Bool("provenance", false, "")

	boom := errors.New("form boom")
	restoreRunner := flags.SetFormRunnerForTest(func(*huh.Form) error { return boom })
	defer restoreRunner()

	originalCompletion := describeComponentStackCompletion
	describeComponentStackCompletion = func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"dev", "staging"}, cobra.ShellCompDirectiveNoFileComp
	}
	defer func() { describeComponentStackCompletion = originalCompletion }()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockExec := exec.NewMockDescribeComponentCmdExec(ctrl)
	mockExec.EXPECT().ExecuteDescribeComponentCmd(gomock.Any()).Times(0)

	run := getRunnableDescribeComponentCmd(getRunnableDescribeComponentCmdProps{
		checkAtmosConfigE: func(opts ...AtmosValidateOption) error { return nil },
		initCliConfig: func(info schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error) {
			return schema.AtmosConfiguration{}, nil
		},
		isExplicitComponentPath: func(component string) bool { return false },
		resolveComponentFromPath: func(atmosConfig *schema.AtmosConfiguration, component, stack string) (string, error) {
			return component, nil
		},
		executeDescribeComponent: func(params *exec.ExecuteDescribeComponentParams) (map[string]any, error) {
			return nil, nil
		},
		newDescribeComponentExec: mockExec,
	})

	err := run(testCmd, []string{"vpc"})

	require.Error(tk, err)
	assert.ErrorIs(tk, err, boom, "the underlying form error must propagate unwrapped through errors.Is")
	assert.Contains(tk, err.Error(), "prompt for --stack",
		"a failing stack prompt must short-circuit RunE with the prompt's own wrapped error, "+
			"not continue on to flag parsing or execution")
}

func TestDescribeComponentCmd_ProvenanceFlag(t *testing.T) {
	// Test that the --provenance flag is properly registered
	// Use PersistentFlags() since that's where the flag is registered
	provenanceFlag := describeComponentCmd.PersistentFlags().Lookup("provenance")
	require.NotNil(t, provenanceFlag, "provenance flag should be registered")
	assert.Equal(t, "bool", provenanceFlag.Value.Type(), "provenance flag should be a boolean")
	assert.Equal(t, "false", provenanceFlag.DefValue, "provenance flag should default to false")
}

func TestHasIdentityBackedStore(t *testing.T) {
	ctrl := gomock.NewController(t)
	identityAware := store.NewMockIdentityAwareStore(ctrl)
	plain := store.NewMockStore(ctrl)

	tests := []struct {
		name        string
		atmosConfig *schema.AtmosConfiguration
		want        bool
	}{
		{name: "nil configuration", atmosConfig: nil, want: false},
		{
			name: "plain store with identity",
			atmosConfig: &schema.AtmosConfiguration{
				StoresConfig: store.StoresConfig{"plain": {Identity: "platform"}},
				Stores:       store.StoreRegistry{"plain": plain},
			},
			want: false,
		},
		{
			name: "identity-aware store without identity",
			atmosConfig: &schema.AtmosConfiguration{
				StoresConfig: store.StoresConfig{"cloud": {}},
				Stores:       store.StoreRegistry{"cloud": identityAware},
			},
			want: false,
		},
		{
			name: "identity-aware store with identity",
			atmosConfig: &schema.AtmosConfiguration{
				StoresConfig: store.StoresConfig{"cloud": {Identity: "platform"}},
				Stores:       store.StoreRegistry{"cloud": identityAware},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, hasIdentityBackedStore(tt.atmosConfig))
		})
	}
}

// TestGetRunnableDescribeComponentCmd_InvalidErrorMode covers the dispatch call site
// inside getRunnableDescribeComponentCmd that rejects a resolved --error-mode value that
// isn't one of "strict", "warn", or "silent" once resolved against atmos.yaml's
// describe.error_mode: an invalid resolved value must short-circuit before the describe
// component executor ever runs. Mirrors describe_stacks_test.go's and
// describe_dependents_test.go's InvalidErrorMode tests for the same shared --error-mode
// flag resolution path (cmd/describe_error_mode_flag.go).
//
// Unlike those siblings, the value is set via ParseFlags rather than by reaching into the
// registered flag's Value directly, since describeComponentCmd's --error-mode is a
// PersistentFlag, and cobra only merges persistent flags into the command's own flag set
// on the first ParseFlags/Execute call, not on registration. Its siblings happen to get
// that merge for free from an unrelated earlier test's real dispatch call, but
// describeComponentCmd does not, so looking up the flag directly would return nil here
// depending on test order. ParseFlags both triggers the merge and sets the value in one
// deterministic step.
func TestGetRunnableDescribeComponentCmd_InvalidErrorMode(t *testing.T) {
	tk := NewTestKit(t)

	viper.Reset()
	tk.Setenv("ATMOS_IDENTITY", "")
	tk.Setenv("IDENTITY", "")

	errorModeFlag := describeComponentCmd.PersistentFlags().Lookup(describeErrorModeFlagName)
	require.NotNil(t, errorModeFlag, "error-mode flag must be registered on describeComponentCmd")
	origValue := errorModeFlag.Value.String()
	origChanged := errorModeFlag.Changed
	t.Cleanup(func() {
		_ = errorModeFlag.Value.Set(origValue)
		errorModeFlag.Changed = origChanged
	})
	require.NoError(t, describeComponentCmd.ParseFlags([]string{"--error-mode=bogus"}))

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockExec := exec.NewMockDescribeComponentCmdExec(ctrl)
	mockExec.EXPECT().ExecuteDescribeComponentCmd(gomock.Any()).Times(0)

	run := getRunnableDescribeComponentCmd(getRunnableDescribeComponentCmdProps{
		checkAtmosConfigE: func(opts ...AtmosValidateOption) error { return nil },
		initCliConfig: func(info schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error) {
			return schema.AtmosConfiguration{}, nil
		},
		isExplicitComponentPath: func(component string) bool { return false },
		resolveComponentFromPath: func(atmosConfig *schema.AtmosConfiguration, component, stack string) (string, error) {
			return component, nil
		},
		executeDescribeComponent: func(params *exec.ExecuteDescribeComponentParams) (map[string]any, error) {
			return nil, nil
		},
		newDescribeComponentExec: mockExec,
	})

	err := run(describeComponentCmd, []string{"vpc"})

	require.ErrorIs(t, err, exec.ErrInvalidErrorMode, "invalid error-mode should be rejected before executing")
}

// TestGetRunnableDescribeComponentCmd_ErrorModeWrongType covers the genuinely-forceable
// return-err branch on cmd.Flags().GetString(describeErrorModeFlagName) inside
// getRunnableDescribeComponentCmd: registering "error-mode" as a Bool flag (instead of the
// real String flag) reproduces a type mismatch without needing to touch any
// BindFlagsToViper-adjacent code path. Mirrors describe_dependents_test.go's
// TestSetFlagsForDescribeDependentsCmd_ErrorModeWrongType and
// describe_edition_test.go's TestDescribeEditionCmd_FormatFlagWrongType.
//
// Note: resolveDescribeErrorModeFlag itself still succeeds here (binding a Bool pflag to
// Viper doesn't error, and Viper's GetString on the bound Bool value round-trips to
// "false", which cmd.Flags().Set("error-mode", "false") happily accepts on a Bool flag)
// -- it's the subsequent cmd.Flags().GetString call that fails, because the flag is
// genuinely a Bool.
func TestGetRunnableDescribeComponentCmd_ErrorModeWrongType(t *testing.T) {
	tk := NewTestKit(t)
	viper.Reset()

	testCmd := &cobra.Command{Use: "component"}
	testCmd.Flags().String("stack", "", "")
	testCmd.Flags().String("format", "yaml", "")
	testCmd.Flags().String("file", "", "")
	testCmd.Flags().Bool("process-templates", true, "")
	testCmd.Flags().Bool("process-functions", true, "")
	testCmd.Flags().String("query", "", "")
	testCmd.Flags().StringSlice("skip", nil, "")
	testCmd.Flags().Bool("provenance", false, "")
	testCmd.Flags().Bool("error-mode", false, "")
	require.NoError(t, testCmd.Flags().Set("error-mode", "true"))

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockExec := exec.NewMockDescribeComponentCmdExec(ctrl)
	mockExec.EXPECT().ExecuteDescribeComponentCmd(gomock.Any()).Times(0)

	run := getRunnableDescribeComponentCmd(getRunnableDescribeComponentCmdProps{
		checkAtmosConfigE: func(opts ...AtmosValidateOption) error { return nil },
		initCliConfig: func(info schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error) {
			return schema.AtmosConfiguration{}, nil
		},
		isExplicitComponentPath: func(component string) bool { return false },
		resolveComponentFromPath: func(atmosConfig *schema.AtmosConfiguration, component, stack string) (string, error) {
			return component, nil
		},
		executeDescribeComponent: func(params *exec.ExecuteDescribeComponentParams) (map[string]any, error) {
			return nil, nil
		},
		newDescribeComponentExec: mockExec,
	})

	err := run(testCmd, []string{"vpc"})

	require.Error(tk, err, "GetString on a Bool-typed error-mode flag must return an error")
	assert.NotErrorIs(tk, err, exec.ErrInvalidErrorMode, "the failure must come from GetString, not error-mode validation")
}

// TestDescribeComponentCmd_ProvenanceWithFormatJSON tests that provenance and format flags
// are correctly parsed and accepted. This is a flag parsing test, not a functional test.
func TestDescribeComponentCmd_ProvenanceWithFormatJSON(t *testing.T) {
	tk := NewTestKit(t)

	stacksPath := "examples/quick-start-advanced"

	// Skip if examples directory doesn't exist.
	if _, err := os.Stat(stacksPath); os.IsNotExist(err) {
		tk.Skipf("Skipping test: %s directory not found", stacksPath)
	}

	tk.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	tk.Setenv("ATMOS_BASE_PATH", stacksPath)

	// Set flags for this test.
	require.NoError(tk, describeComponentCmd.PersistentFlags().Set("stack", "plat-ue2-dev"))
	require.NoError(tk, describeComponentCmd.PersistentFlags().Set("format", "json"))
	require.NoError(tk, describeComponentCmd.PersistentFlags().Set("provenance", "true"))

	// Execute command - may fail due to missing files in test environment.
	// We're testing that flag parsing succeeds, not the full command execution.
	err := describeComponentCmd.RunE(describeComponentCmd, []string{"vpc"})
	if err != nil {
		// Verify the error is not due to flag parsing issues.
		errStr := err.Error()
		assert.NotContains(tk, errStr, "unknown flag", "Flag parsing should succeed")
		assert.NotContains(tk, errStr, "invalid flag", "Flag validation should succeed")
	}
}

// TestDescribeComponentCmd_ProvenanceWithFileOutput tests that provenance and file flags
// are correctly parsed and accepted. This is a flag parsing test, not a functional test.
func TestDescribeComponentCmd_ProvenanceWithFileOutput(t *testing.T) {
	tk := NewTestKit(t)

	stacksPath := "examples/quick-start-advanced"

	// Skip if examples directory doesn't exist.
	if _, err := os.Stat(stacksPath); os.IsNotExist(err) {
		tk.Skipf("Skipping test: %s directory not found", stacksPath)
	}

	tk.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	tk.Setenv("ATMOS_BASE_PATH", stacksPath)

	// Create a temporary file for output.
	tmpFile := filepath.Join(os.TempDir(), "test-provenance-output.yaml")
	defer os.Remove(tmpFile)

	// Set flags for this test.
	require.NoError(tk, describeComponentCmd.PersistentFlags().Set("stack", "plat-ue2-dev"))
	require.NoError(tk, describeComponentCmd.PersistentFlags().Set("file", tmpFile))
	require.NoError(tk, describeComponentCmd.PersistentFlags().Set("provenance", "true"))

	// Execute command - may fail due to missing files in test environment.
	// We're testing that flag parsing succeeds, not the full command execution.
	err := describeComponentCmd.RunE(describeComponentCmd, []string{"vpc"})
	if err != nil {
		// Verify the error is not due to flag parsing issues.
		errStr := err.Error()
		assert.NotContains(tk, errStr, "unknown flag", "Flag parsing should succeed")
		assert.NotContains(tk, errStr, "invalid flag", "Flag validation should succeed")
	}
}

// TestDescribeComponentCmd_PathResolution tests that component arguments with various formats
// are processed without panicking. This is a smoke test for the path resolution code path.
func TestDescribeComponentCmd_PathResolution(t *testing.T) {
	tk := NewTestKit(t)

	stacksPath := "examples/quick-start-advanced"

	// Skip if examples directory doesn't exist.
	if _, err := os.Stat(stacksPath); os.IsNotExist(err) {
		tk.Skipf("Skipping test: %s directory not found", stacksPath)
	}

	tk.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	tk.Setenv("ATMOS_BASE_PATH", stacksPath)

	tests := []struct {
		name      string
		component string
		stack     string
	}{
		{
			name:      "component name resolution",
			component: "vpc",
			stack:     "plat-ue2-dev",
		},
		{
			name:      "component name with slash",
			component: "vpc/security",
			stack:     "plat-ue2-dev",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tk := NewTestKit(t)

			// Set flags.
			require.NoError(tk, describeComponentCmd.PersistentFlags().Set("stack", tt.stack))

			// Execute command - may fail due to missing component in test environment.
			// We're testing that the code path executes without panicking.
			err := describeComponentCmd.RunE(describeComponentCmd, []string{tt.component})
			if err != nil {
				// Non-path components (without ./ or ../ prefix) should not trigger
				// path resolution logic, so any error should be about missing component.
				errStr := err.Error()
				assert.NotContains(tk, errStr, "path resolution", "Non-path component should bypass path resolution")
			}
		})
	}
}

// TestDescribeComponentCmd_ConfigLoadError tests that config load errors are properly handled
// for both regular component names and path-based component references.
func TestDescribeComponentCmd_ConfigLoadError(t *testing.T) {
	tests := []struct {
		name      string
		component string
	}{
		{
			name:      "non-path component with invalid config",
			component: "vpc",
		},
		{
			name:      "path component with invalid config",
			component: "./components/terraform/vpc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tk := NewTestKit(t)

			// Set invalid config path to trigger config load error.
			tk.Setenv("ATMOS_CLI_CONFIG_PATH", "/nonexistent/path")

			// Set flags.
			require.NoError(tk, describeComponentCmd.PersistentFlags().Set("stack", "test-stack"))

			// Run command - should fail due to config load error.
			err := describeComponentCmd.RunE(describeComponentCmd, []string{tt.component})
			assert.Error(tk, err, "Command should fail with invalid config path")
		})
	}
}

// TestDescribeComponentCmd_AuthManager tests that the auth manager code path is exercised
// without panicking. This is a smoke test for auth manager integration.
func TestDescribeComponentCmd_AuthManager(t *testing.T) {
	tk := NewTestKit(t)

	stacksPath := "examples/quick-start-advanced"

	// Skip if examples directory doesn't exist.
	if _, err := os.Stat(stacksPath); os.IsNotExist(err) {
		tk.Skipf("Skipping test: %s directory not found", stacksPath)
	}

	tk.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	tk.Setenv("ATMOS_BASE_PATH", stacksPath)

	// Set flags.
	require.NoError(tk, describeComponentCmd.PersistentFlags().Set("stack", "plat-ue2-dev"))

	// Execute command - may fail due to missing component in test environment.
	// We're testing that auth manager creation code path executes without panicking.
	// Actual auth validation is covered in dedicated auth tests.
	err := describeComponentCmd.RunE(describeComponentCmd, []string{"vpc"})
	if err != nil {
		// Verify error is not due to auth manager initialization issues.
		errStr := err.Error()
		assert.NotContains(tk, errStr, "auth manager creation failed", "Auth manager should initialize without errors")
	}
}
