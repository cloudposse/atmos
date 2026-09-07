package cmd

import (
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Edition-pin precedence/source detection (flag > env > config) is now owned by
// pkg/config.EditionPinSource — see TestEditionPinSource in
// pkg/config/load_edition_test.go for that coverage in isolation.
// TestDescribeEditionCmdOutputFormats below still asserts the real `source:`
// field end-to-end through this command's wiring (not just pkg/config's own
// unit tests), since that wiring — passing atmosConfig.Edition into
// cfg.EditionPinSource — is exactly where a regression would hide otherwise.

// TestDescribeEditionCmd_FormatFlagWrongType covers the genuinely-forceable return-err
// branch on cmd.Flags().GetString("format"): registering "format" as a Bool flag (instead
// of the real String flag) and marking it Changed reproduces a type mismatch without
// needing to touch any config-loading code path. Mirrors
// describe_dependents_test.go's TestSetFlagsForDescribeDependentsCmd_ErrorModeWrongType.
func TestDescribeEditionCmd_FormatFlagWrongType(t *testing.T) {
	testCmd := &cobra.Command{Use: "edition"}
	testCmd.Flags().Bool("format", false, "")
	require.NoError(t, testCmd.Flags().Set("format", "true"))

	err := describeEditionCmd.RunE(testCmd, nil)
	require.Error(t, err, "GetString on a Bool-typed flag must return an error")
}

// TestDescribeEditionCmd_ConfigLoadError covers the cfg.InitCliConfig error-propagation
// branch. `describe edition` calls InitCliConfig with processStacks=false, which loads
// atmos.yaml leniently (falling back to defaults for most config problems, per this repo's
// "defaults work without atmos.yaml" contract — an unresolvable --config-path or a merely
// missing atmos.yaml does not error here). An unparseable `edition:` pin does fail
// unconditionally (config.applyEditionDefaults validates it before the pin can ever reach
// DescribePin), giving a real, reachable reproducer for this branch.
func TestDescribeEditionCmd_ConfigLoadError(t *testing.T) {
	tk := NewTestKit(t)
	tk.Chdir(t.TempDir())
	require.NoError(t, os.WriteFile("atmos.yaml", []byte("edition: \"not-a-date\"\n"), 0o600))

	testCmd := &cobra.Command{Use: "edition"}
	testCmd.Flags().String("format", "yaml", "")

	err := describeEditionCmd.RunE(testCmd, nil)
	require.Error(t, err, "an invalid edition pin should surface as a config load error")
	assert.Contains(t, err.Error(), "invalid edition")
}

func TestDescribeEditionCmdOutputFormats(t *testing.T) {
	_ = NewTestKit(t)
	t.Setenv("ATMOS_EDITION", "")
	t.Chdir(t.TempDir())
	require.NoError(t, os.WriteFile("atmos.yaml", []byte("edition: \"2025-09\"\n"), 0o600))

	for _, tt := range []struct {
		name   string
		format string
		want   string
	}{
		{name: "yaml", format: "yaml", want: "pinned: true"},
		{name: "json", format: "json", want: "\"pinned\": true"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			testCmd := &cobra.Command{Use: "edition"}
			testCmd.Flags().String("format", tt.format, "")
			stdout, _ := captureStdoutStderr(t, func() {
				require.NoError(t, describeEditionCmd.RunE(testCmd, nil))
			})
			assert.Contains(t, stdout, tt.want)
		})
	}
}

// TestDescribeEditionCmdSourceIsConfig guards against a regression where
// cfg.EditionPinSource was called with the wrong Viper instance (the global
// singleton, which never has the config file merged into it) and silently
// dropped the `source` field instead of reporting "config" for a pin that
// came from atmos.yaml — see cfg.EditionPinSource's doc comment.
func TestDescribeEditionCmdSourceIsConfig(t *testing.T) {
	_ = NewTestKit(t)
	t.Setenv("ATMOS_EDITION", "")
	t.Chdir(t.TempDir())
	require.NoError(t, os.WriteFile("atmos.yaml", []byte("edition: \"2026-06\"\n"), 0o600))

	testCmd := &cobra.Command{Use: "edition"}
	testCmd.Flags().String("format", "yaml", "")
	stdout, _ := captureStdoutStderr(t, func() {
		require.NoError(t, describeEditionCmd.RunE(testCmd, nil))
	})
	assert.Contains(t, stdout, "source: config")
}
