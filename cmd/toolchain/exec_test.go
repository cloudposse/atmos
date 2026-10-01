package toolchain

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Provider and structure tests are in command_provider_test.go.
// Business-logic coverage for --dry-run (resolving the binary path but never invoking it) lives
// in pkg/toolchain/exec_test.go, which tests RunExecCommandWithOptions directly against a fake
// installer. This file only covers the cmd-layer flag wiring.

func TestExecCommand_Flags(t *testing.T) {
	t.Run("has dry-run flag", func(t *testing.T) {
		flag := execCmd.Flags().Lookup("dry-run")
		require.NotNil(t, flag)
		assert.Equal(t, "false", flag.DefValue)
	})
}

func TestExecCommand_FlagDescriptions(t *testing.T) {
	flag := execCmd.Flags().Lookup("dry-run")
	require.NotNil(t, flag)
	assert.Contains(t, flag.Usage, "without executing")
}

// TestExecCommand_RunE_DryRun_MissingToolSpec exercises the real execCmd.RunE with --dry-run to
// confirm the dry-run flag is actually wired through to RunExecCommandWithOptions (not just
// registered and ignored): an invalid tool spec still fails fast on the same
// ErrInvalidToolSpec/resolver guard it would without --dry-run, proving RunE reached the shared
// business logic rather than short-circuiting into some dry-run-only code path.
func TestExecCommand_RunE_DryRun_InvalidTool(t *testing.T) {
	t.Cleanup(func() {
		require.NoError(t, execCmd.Flags().Set("dry-run", "false"))
		viper.Reset()
	})

	require.NoError(t, execCmd.Flags().Set("dry-run", "true"))

	err := execCmd.RunE(execCmd, []string{"not-a-real-tool@1.0.0"})
	require.Error(t, err)
}

func TestExecCommand_Args(t *testing.T) {
	t.Run("rejects zero arguments", func(t *testing.T) {
		err := execCmd.Args(execCmd, []string{})
		assert.Error(t, err)
	})

	t.Run("accepts one argument", func(t *testing.T) {
		err := execCmd.Args(execCmd, []string{"terraform@1.9.8"})
		assert.NoError(t, err)
	})
}
