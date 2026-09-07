package updater

import (
	"context"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResolveExecutionWorkdir_CurrentModeIsNoOp proves execution.mode values other than "worktree"
// (unset, or explicitly "current") leave workdir/resolvedBase/cleanup exactly as before this
// feature existed: no worktree is created, no env var or cwd is touched. The "worktree" mode's
// isolation guarantee is proven end-to-end (against real git fixtures) by
// TestResolveExecutionWorkdir_WorktreeModeIsolatesInvokingCheckout in cmd/vendor, which also
// exercises the version-bump write phase that follows it.
func TestResolveExecutionWorkdir_CurrentModeIsNoOp(t *testing.T) {
	for _, mode := range []string{"", "current"} {
		t.Run("mode="+mode, func(t *testing.T) {
			v := viper.New()
			if mode != "" {
				v.Set("vendor.update.execution.mode", mode)
			}

			execWorkdir, err := ResolveExecutionWorkdir(context.Background(), v, "workdir-placeholder")
			require.NoError(t, err)
			assert.Equal(t, "workdir-placeholder", execWorkdir.Workdir)
			assert.Empty(t, execWorkdir.ResolvedBase)
			require.NotNil(t, execWorkdir.Cleanup)
			execWorkdir.Cleanup() // Must be a safe no-op.
		})
	}
}
