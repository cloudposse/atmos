package function

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/stretchr/testify/require"
)

func TestRegistryFunctionAdmission(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ATMOS_POLICY_DIR", dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "guard.yaml"), []byte("apiVersion: atmos/v1\nkind: Policy\nmetadata: {name: registry}\nspec:\n  policies:\n    deny:\n      on: function\n      action: deny\n      rule: input.name == 'exec'\n"), 0o600))
	fn, err := DefaultRegistry().Get("exec")
	require.NoError(t, err)
	_, err = fn.Execute(context.Background(), "never-start", nil)
	require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
	_, err = NewExecFunction().Execute(context.Background(), "never-start", nil)
	require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
}
