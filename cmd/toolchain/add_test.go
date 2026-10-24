package toolchain

import (
	"os"
	"path/filepath"
	"testing"

	cockroachdberrors "github.com/cockroachdb/errors"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain"
)

// TestAddCommand_Structure tests the add command structure.
func TestAddCommand_Structure(t *testing.T) {
	t.Run("command has correct use string with variadic args", func(t *testing.T) {
		assert.Contains(t, addCmd.Use, "add")
		assert.Contains(t, addCmd.Use, "<tool[@version]>")
		assert.Contains(t, addCmd.Use, "...")
	})

	t.Run("command has short description", func(t *testing.T) {
		assert.NotEmpty(t, addCmd.Short)
		assert.Contains(t, addCmd.Short, "Add")
	})

	t.Run("command has long description mentioning latest default", func(t *testing.T) {
		assert.NotEmpty(t, addCmd.Long)
		assert.Contains(t, addCmd.Long, "latest")
	})

	t.Run("command has RunE function", func(t *testing.T) {
		assert.NotNil(t, addCmd.RunE)
	})
}

// TestAddCommand_Args tests argument handling for the add command.
func TestAddCommand_Args(t *testing.T) {
	t.Run("command accepts minimum 1 argument", func(t *testing.T) {
		// The Args function should be cobra.MinimumNArgs(1).
		require.NotNil(t, addCmd.Args)

		// Test that it fails with 0 args.
		err := addCmd.Args(addCmd, []string{})
		assert.Error(t, err)
	})

	t.Run("command accepts 1 argument", func(t *testing.T) {
		err := addCmd.Args(addCmd, []string{"terraform@1.5.0"})
		assert.NoError(t, err)
	})

	t.Run("command accepts multiple arguments", func(t *testing.T) {
		err := addCmd.Args(addCmd, []string{"terraform@1.5.0", "kubectl@1.28.0", "opentofu@1.7.0"})
		assert.NoError(t, err)
	})
}

// TestAddCommandProvider_Extended tests additional AddCommandProvider functionality.
func TestAddCommandProvider_Extended(t *testing.T) {
	provider := &AddCommandProvider{}

	t.Run("command has correct Use string", func(t *testing.T) {
		cmd := provider.GetCommand()
		require.NotNil(t, cmd)
		assert.Contains(t, cmd.Use, "add")
		assert.Contains(t, cmd.Use, "<tool[@version]>")
	})

	t.Run("provider returns correct command type", func(t *testing.T) {
		cmd := provider.GetCommand()
		require.NotNil(t, cmd)
		assert.IsType(t, &cobra.Command{}, cmd)
	})
}

// TestAddCommand_MultiPackageSupport tests that the command supports multiple packages.
func TestAddCommand_MultiPackageSupport(t *testing.T) {
	t.Run("Use string indicates variadic arguments", func(t *testing.T) {
		// The ... suffix indicates variadic arguments.
		assert.Contains(t, addCmd.Use, "...")
	})

	t.Run("Args validator allows multiple arguments", func(t *testing.T) {
		// Test with various numbers of arguments.
		testCases := []struct {
			name    string
			args    []string
			wantErr bool
		}{
			{"zero args", []string{}, true},
			{"one arg", []string{"terraform@1.5.0"}, false},
			{"two args", []string{"terraform@1.5.0", "kubectl@1.28.0"}, false},
			{"three args", []string{"a@1", "b@2", "c@3"}, false},
			{"five args", []string{"a@1", "b@2", "c@3", "d@4", "e@5"}, false},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				err := addCmd.Args(addCmd, tc.args)
				if tc.wantErr {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			})
		}
	})
}

// TestAddCommand_LatestVersionDefault tests that the Long description documents the latest default.
func TestAddCommand_LatestVersionDefault(t *testing.T) {
	t.Run("Long description mentions version defaults to latest", func(t *testing.T) {
		assert.Contains(t, addCmd.Long, "latest")
		assert.Contains(t, addCmd.Long, "omitted")
	})

	t.Run("Use string shows version is optional with brackets", func(t *testing.T) {
		// The [@version] syntax indicates version is optional.
		assert.Contains(t, addCmd.Use, "[@version]")
	})
}

// setupTestEnvironment creates a temp directory with an atmos config and tool-versions file.
// Returns cleanup function and the temp directory path.
func setupTestEnvironment(t *testing.T) (cleanup func(), tempDir string) {
	t.Helper()

	tempDir = t.TempDir()

	// Create a minimal tool-versions file.
	tvPath := filepath.Join(tempDir, ".tool-versions")
	err := os.WriteFile(tvPath, []byte(""), 0o644)
	require.NoError(t, err)

	// Save original config and set test config.
	originalConfig := toolchain.GetAtmosConfig()

	testConfig := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			VersionsFile: tvPath,
			InstallPath:  tempDir,
		},
	}
	toolchain.SetAtmosConfig(testConfig)

	cleanup = func() {
		toolchain.SetAtmosConfig(originalConfig)
	}

	return cleanup, tempDir
}

// TestAddCommand_RunE tests the RunE function execution paths.
func TestAddCommand_RunE(t *testing.T) {
	t.Run("RunE with valid tool format processes argument", func(t *testing.T) {
		cleanup, tempDir := setupTestEnvironment(t)
		defer cleanup()

		// Call RunE with a valid tool@version format.
		// This exercises:
		// - Line 21: for loop entry
		// - Line 22: ParseToolVersionArg call (success)
		// - Line 27-28: version check (skipped since version provided)
		// - Line 30: AddToolVersion call
		// The tool may be found in Aqua registry, so this may succeed.
		err := addCmd.RunE(addCmd, []string{"hashicorp/terraform@1.5.0"})

		// Either succeeds (tool found in registry) or fails with proper error.
		if err != nil {
			assert.ErrorIs(t, err, errUtils.ErrToolVersionsFileOperation)
		} else {
			// Success - verify tool was added to version file.
			tvPath := filepath.Join(tempDir, ".tool-versions")
			content, readErr := os.ReadFile(tvPath)
			require.NoError(t, readErr)
			assert.Contains(t, string(content), "terraform")
		}
	})

	t.Run("RunE with version omitted defaults to latest", func(t *testing.T) {
		cleanup, tempDir := setupTestEnvironment(t)
		defer cleanup()

		// Call RunE without version to test default to "latest".
		// This exercises:
		// - Line 22: ParseToolVersionArg call
		// - Line 27-28: version empty check, defaults to "latest"
		// - Line 30: AddToolVersion call with "latest"
		err := addCmd.RunE(addCmd, []string{"hashicorp/terraform"})

		// Either succeeds or fails with proper error.
		if err != nil {
			assert.ErrorIs(t, err, errUtils.ErrToolVersionsFileOperation)
		} else {
			// Success - verify tool was added with "latest".
			tvPath := filepath.Join(tempDir, ".tool-versions")
			content, readErr := os.ReadFile(tvPath)
			require.NoError(t, readErr)
			assert.Contains(t, string(content), "terraform")
			assert.Contains(t, string(content), "latest")
		}
	})

	t.Run("RunE with multiple valid arguments processes all", func(t *testing.T) {
		cleanup, tempDir := setupTestEnvironment(t)
		defer cleanup()

		// Process multiple tools.
		err := addCmd.RunE(addCmd, []string{"hashicorp/terraform@1.5.0", "kubernetes/kubectl@1.28.0"})

		// Either both succeed or first failure returns error.
		if err != nil {
			assert.ErrorIs(t, err, errUtils.ErrToolVersionsFileOperation)
		} else {
			// Success - verify tools were added.
			tvPath := filepath.Join(tempDir, ".tool-versions")
			content, readErr := os.ReadFile(tvPath)
			require.NoError(t, readErr)
			assert.Contains(t, string(content), "terraform")
		}
	})

	t.Run("RunE with short alias format processes argument", func(t *testing.T) {
		cleanup, _ := setupTestEnvironment(t)
		defer cleanup()

		// Test with short tool name that gets resolved via aliases/registry.
		err := addCmd.RunE(addCmd, []string{"terraform@1.5.0"})
		// Either succeeds (tool found) or fails with proper error wrapping.
		if err != nil {
			assert.ErrorIs(t, err, errUtils.ErrToolVersionsFileOperation)
		}
	})
}

// TestAddCommand_RunE_ErrorPaths tests error handling in RunE.
func TestAddCommand_RunE_ErrorPaths(t *testing.T) {
	t.Run("error wraps with ErrToolVersionsFileOperation", func(t *testing.T) {
		cleanup, _ := setupTestEnvironment(t)
		defer cleanup()

		// Any failure should be wrapped with ErrToolVersionsFileOperation.
		err := addCmd.RunE(addCmd, []string{"nonexistent-tool@1.0.0"})

		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrToolVersionsFileOperation)
	})

	t.Run("error message contains failed argument", func(t *testing.T) {
		cleanup, _ := setupTestEnvironment(t)
		defer cleanup()

		err := addCmd.RunE(addCmd, []string{"my-special-tool@1.0.0"})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "my-special-tool")
	})

	// TestAddCommand_RunE_ErrorPaths/range_syntax_error_preserves_hints_and_explanation
	// reproduces a field-test finding: runAdd wraps every error with
	// fmt.Errorf("%w: failed to add '%s': %w", ErrToolVersionsFileOperation, arg, err) -- two
	// %w verbs in one call. That produces a Go 1.20 multi-wrap error that
	// github.com/cockroachdb/errors' GetAllHints/GetAllDetails cannot traverse, so the helpful,
	// hand-written hint ValidateVersionSpec attaches for range/constraint syntax (e.g.
	// "^1.7.0") is completely invisible to `atmos toolchain add` users, even though
	// `atmos toolchain install`/`atmos toolchain set` (which don't double-wrap) show it
	// correctly. errors.Is still matches ErrVersionFormatInvalid -- only the human-facing
	// hint/explanation content is lost.
	t.Run("range syntax error preserves hints and explanation", func(t *testing.T) {
		cleanup, _ := setupTestEnvironment(t)
		defer cleanup()

		err := addCmd.RunE(addCmd, []string{"hashicorp/terraform@~>1.0"})

		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrVersionFormatInvalid)
		assert.NotEmpty(t, cockroachdberrors.GetAllHints(err),
			"the range/constraint hint (use dependencies.tools or atmos version track) must survive error wrapping")
		assert.NotEmpty(t, cockroachdberrors.GetAllDetails(err),
			"the explanation of why the version was rejected must survive error wrapping")
	})
}

// TestAddCommand_RunE_DefaultFlagIgnoredForMultipleTools reproduces the runAdd guard that
// resets setAsDefault to false (and warns) when --default is combined with more than one
// tool argument. AddVersionToTool's asDefault=true path *replaces* a tool's whole version
// list with just the new version, while asDefault=false *appends* alongside any existing
// entry. Pre-seeding an existing "hashicorp/terraform 1.0.0" entry and then running
// `add --default hashicorp/terraform@1.5.0 kubernetes/kubectl@1.28.0` makes the two
// behaviors observably different: if the guard didn't fire, the pre-existing 1.0.0 entry
// would be wiped; since it fires, 1.5.0 is appended and 1.0.0 survives.
func TestAddCommand_RunE_DefaultFlagIgnoredForMultipleTools(t *testing.T) {
	cleanup, tempDir := setupTestEnvironment(t)
	defer cleanup()

	tvPath := filepath.Join(tempDir, ".tool-versions")
	require.NoError(t, os.WriteFile(tvPath, []byte("hashicorp/terraform 1.0.0\n"), 0o644))

	t.Cleanup(func() {
		require.NoError(t, addCmd.Flags().Set("default", "false"))
	})
	require.NoError(t, addCmd.Flags().Set("default", "true"))

	err := addCmd.RunE(addCmd, []string{"hashicorp/terraform@1.5.0", "kubernetes/kubectl@1.28.0"})
	require.NoError(t, err)

	content, readErr := os.ReadFile(tvPath)
	require.NoError(t, readErr)

	// If --default had NOT been ignored, this line would read only "hashicorp/terraform
	// 1.5.0" (the 1.0.0 entry replaced). Since multi-tool --default is ignored, both
	// versions must be present, appended.
	assert.Contains(t, string(content), "1.0.0")
	assert.Contains(t, string(content), "1.5.0")
}

// TestAddCommandProvider_AllMethods tests all CommandProvider interface methods.
func TestAddCommandProvider_AllMethods(t *testing.T) {
	provider := &AddCommandProvider{}

	t.Run("GetName returns add", func(t *testing.T) {
		assert.Equal(t, "add", provider.GetName())
	})

	t.Run("GetGroup returns Toolchain Commands", func(t *testing.T) {
		assert.Equal(t, "Toolchain Commands", provider.GetGroup())
	})

	t.Run("GetFlagsBuilder returns non-nil parser", func(t *testing.T) {
		assert.NotNil(t, provider.GetFlagsBuilder())
	})

	t.Run("GetPositionalArgsBuilder returns nil", func(t *testing.T) {
		assert.Nil(t, provider.GetPositionalArgsBuilder())
	})

	t.Run("GetCompatibilityFlags returns nil", func(t *testing.T) {
		assert.Nil(t, provider.GetCompatibilityFlags())
	})
}
