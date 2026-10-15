package utils

import (
	"os"
	"path/filepath"
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/stretchr/testify/require"
)

func TestExecPolicyBeforeShell(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ATMOS_POLICY_DIR", dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "deny.yaml"), []byte("apiVersion: atmos/v1\nkind: Policy\nmetadata: {name: safety}\nspec:\n  policies:\n    exec:\n      on: function\n      action: deny\n      rule: input.name == 'exec'\n"), 0o600))
	_, err := ProcessTagExec("!exec nonexistent-command-must-never-start secret-argument")
	require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
	require.NotContains(t, err.Error(), "secret-argument")
}
