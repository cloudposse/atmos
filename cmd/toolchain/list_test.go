package toolchain

import (
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain"
)

// Provider and structure tests are in command_provider_test.go.
// This file contains list-specific tests.

func TestListCommand_Flags(t *testing.T) {
	t.Run("has format flag", func(t *testing.T) {
		flag := listCmd.Flags().Lookup("format")
		require.NotNil(t, flag)
		assert.Equal(t, "table", flag.DefValue)
	})

	t.Run("has installed-only flag", func(t *testing.T) {
		flag := listCmd.Flags().Lookup("installed-only")
		require.NotNil(t, flag)
		assert.Equal(t, "false", flag.DefValue)
	})

	t.Run("has pending-only flag", func(t *testing.T) {
		flag := listCmd.Flags().Lookup("pending-only")
		require.NotNil(t, flag)
		assert.Equal(t, "false", flag.DefValue)
	})
}

func TestListCommand_FlagDescriptions(t *testing.T) {
	tests := []struct {
		flagName string
		contains string
	}{
		{"format", "format"},
		{"installed-only", "installed"},
		{"pending-only", "pending"},
	}

	for _, tt := range tests {
		t.Run(tt.flagName+" has description", func(t *testing.T) {
			flag := listCmd.Flags().Lookup(tt.flagName)
			require.NotNil(t, flag)
			assert.Contains(t, flag.Usage, tt.contains)
		})
	}
}

func TestListCommand_SupportedFormats(t *testing.T) {
	assert.Equal(t, []string{"table", "plain", "json"}, supportedListFormats)
}

// TestListCommand_RunE exercises the real listCmd.RunE (not a re-implementation of its logic),
// covering the format-validation and --installed-only/--pending-only mutual-exclusion guards
// added to cmd/toolchain/list.go. It points VersionsFile at a non-existent path in an isolated
// temp dir so RunListWithOptions takes its "no .tool-versions file" graceful-exit branch instead
// of touching any real repo state.
func TestListCommand_RunE(t *testing.T) {
	tempDir := t.TempDir()
	toolchain.SetAtmosConfig(&schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			VersionsFile: filepath.Join(tempDir, "tool-versions-does-not-exist"),
		},
	})
	t.Cleanup(func() { toolchain.SetAtmosConfig(nil) })

	tests := []struct {
		name          string
		format        string
		installedOnly string
		pendingOnly   string
		wantErr       error
	}{
		{name: "table format is valid", format: "table"},
		{name: "plain format is valid", format: "plain"},
		{name: "json format is valid", format: "json"},
		{name: "invalid format is rejected", format: "xml", wantErr: errUtils.ErrInvalidFlagValue},
		{name: "empty format is rejected", format: "", wantErr: errUtils.ErrInvalidFlagValue},
		{
			name: "installed-only and pending-only are mutually exclusive", format: "table",
			installedOnly: "true", pendingOnly: "true", wantErr: errUtils.ErrMutuallyExclusiveFlags,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(func() {
				require.NoError(t, listCmd.Flags().Set("format", "table"))
				require.NoError(t, listCmd.Flags().Set("installed-only", "false"))
				require.NoError(t, listCmd.Flags().Set("pending-only", "false"))
				viper.Reset()
			})

			require.NoError(t, listCmd.Flags().Set("format", tt.format))
			if tt.installedOnly != "" {
				require.NoError(t, listCmd.Flags().Set("installed-only", tt.installedOnly))
			}
			if tt.pendingOnly != "" {
				require.NoError(t, listCmd.Flags().Set("pending-only", tt.pendingOnly))
			}

			err := listCmd.RunE(listCmd, []string{})

			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestListCommand_FormatFlagCompletion exercises the shell-completion function registered for
// --format in cmd/toolchain/list.go's init(). This isn't boilerplate: an off-by-one or stale
// list here would silently break `atmos toolchain list --format <TAB>` without any other test
// catching it, since RunE's own format validation reads the same supportedListFormats slice
// directly rather than through this registered function.
func TestListCommand_FormatFlagCompletion(t *testing.T) {
	completionFunc, ok := listCmd.GetFlagCompletionFunc("format")
	require.True(t, ok, "expected a completion function to be registered for --format")

	suggestions, directive := completionFunc(listCmd, []string{}, "")

	assert.Equal(t, supportedListFormats, suggestions)
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
}

func TestListCommand_Args(t *testing.T) {
	t.Run("accepts zero arguments", func(t *testing.T) {
		err := listCmd.Args(listCmd, []string{})
		assert.NoError(t, err)
	})

	t.Run("rejects one argument", func(t *testing.T) {
		err := listCmd.Args(listCmd, []string{"terraform"})
		assert.Error(t, err)
	})
}
