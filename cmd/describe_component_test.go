package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
)

func TestDescribeComponentCmd_Error(t *testing.T) {
	tk := NewTestKit(t)

	// This test verifies that calling the command with no arguments returns an error.
	// The command requires exactly one argument (the component name).
	err := describeComponentCmd.RunE(describeComponentCmd, []string{})
	assert.Error(tk, err, "describe component command should return an error when called with no parameters")
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

	assert.False(t, hasIdentityBackedStore(nil))
	assert.False(t, hasIdentityBackedStore(&schema.AtmosConfiguration{
		StoresConfig: store.StoresConfig{"plain": {Identity: "platform"}},
		Stores:       store.StoreRegistry{"plain": plain},
	}))
	assert.False(t, hasIdentityBackedStore(&schema.AtmosConfiguration{
		StoresConfig: store.StoresConfig{"cloud": {}},
		Stores:       store.StoreRegistry{"cloud": identityAware},
	}))
	assert.True(t, hasIdentityBackedStore(&schema.AtmosConfiguration{
		StoresConfig: store.StoresConfig{"cloud": {Identity: "platform"}},
		Stores:       store.StoreRegistry{"cloud": identityAware},
	}))
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
