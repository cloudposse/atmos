package admission

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/policy"
	"github.com/stretchr/testify/require"
)

func TestFunctionAdmissionPinsPolicies(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ATMOS_POLICY_DIR", dir)
	path := filepath.Join(dir, "deny.yaml")
	require.NoError(t, os.WriteFile(path, []byte("apiVersion: atmos/v1\nkind: Policy\nmetadata: {name: safety}\nspec:\n  policies:\n    exec:\n      on: function\n      action: deny\n      rule: input.name == 'exec'\n"), 0o600))
	restore, err := BeginInvocation()
	require.NoError(t, err)
	defer restore()
	require.NoError(t, os.Remove(path))
	t.Setenv("ATMOS_POLICY_DIR", t.TempDir())
	require.ErrorIs(t, Function(context.Background(), "!exec"), errUtils.ErrPolicyDenied)
	require.NoError(t, Function(context.Background(), "env"))
	enabled, err := HasRules(policy.Function)
	require.NoError(t, err)
	require.True(t, enabled)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.Error(t, Function(ctx, "exec"))
}

func TestAdmissionInvalidDirectoryFailsClosed(t *testing.T) {
	t.Setenv("ATMOS_POLICY_DIR", t.TempDir())
	require.ErrorIs(t, Function(context.Background(), "exec"), errUtils.ErrPolicyInvalid)
	_, err := BeginInvocation()
	require.Error(t, err)
}

func TestAdmissionMissingDefault(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("ATMOS_POLICY_DIR", "")
	require.NoError(t, Function(context.Background(), "repo-root"))
	require.NoError(t, Function(context.Background(), "git.ref"))
	require.ErrorIs(t, Check(context.Background(), nil), errUtils.ErrPolicyInputInvalid)
}
