package admission

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/ci/providers/github"
	"github.com/cloudposse/atmos/pkg/policy"
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

// writePolicy installs one policy document and points the loader at it.
func writePolicy(t *testing.T, document string) {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "policy.yaml"), []byte(document), 0o600))
	t.Setenv("ATMOS_POLICY_DIR", dir)
}

func TestCheckWithoutRulesForTheRequestKind(t *testing.T) {
	writePolicy(t, "apiVersion: atmos/v1\nkind: Policy\nmetadata: {name: safety}\nspec:\n  policies:\n    exec:\n      on: function\n      action: deny\n      rule: 'true'\n")
	var events []policy.Explanation
	cleanup := policy.BeginExplanation(func(e policy.Explanation) { events = append(events, e) }, nil)
	defer cleanup()

	err := Check(context.Background(), &policy.Request{Kind: policy.Source, Input: []byte(`{}`)})

	require.NoError(t, err, "rules for other capabilities never apply")
	require.Equal(t, []policy.Explanation{{Kind: policy.Source, Status: "gate reached; no configured rules"}}, events)
}

func TestCheckFailsClosedWhenCISnapshotCannotBeCollected(t *testing.T) {
	writePolicy(t, "apiVersion: atmos/v1\nkind: Policy\nmetadata: {name: safety}\nspec:\n  policies:\n    exec:\n      on: function\n      action: deny\n      rule: 'false'\n")
	restore := ci.SwapRegistryForTest()
	defer restore()
	ci.Register(github.NewProvider())
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_EVENT_PATH", "")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := Check(ctx, &policy.Request{Kind: policy.Function, Input: []byte(`{"name":"exec"}`)})

	require.ErrorIs(t, err, context.Canceled)
}

func TestCheckFailsClosedWhenApprovalsCannotBeVerified(t *testing.T) {
	writePolicy(t, `apiVersion: atmos/v1
kind: Policy
metadata: {name: review}
spec:
  policies:
    exec:
      on: function
      action: require_approval
      rule: "true"
      approvals:
        - id: platform
          provider: github-actions
          spec: {reviewers: [alice], minimum: 1}
`)
	restore := ci.SwapRegistryForTest()
	defer restore()

	err := Check(context.Background(), &policy.Request{Kind: policy.Function, Input: []byte(`{"name":"exec"}`)})

	require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable, "no CI provider can verify the review obligation")
}

func TestCheckWithholdsEvaluationDetails(t *testing.T) {
	writePolicy(t, "apiVersion: atmos/v1\nkind: Policy\nmetadata: {name: safety}\nspec:\n  policies:\n    exec:\n      on: function\n      action: deny\n      rule: 'false'\n")
	// A live CI provider would collect its snapshot with the cancelled context and return that error first.
	restore := ci.SwapRegistryForTest()
	defer restore()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := Check(ctx, &policy.Request{Kind: policy.Function, Input: []byte(`{"name":"secret-argument"}`)})

	require.ErrorIs(t, err, errUtils.ErrPolicyEvaluation)
	require.NotContains(t, err.Error(), "secret-argument")
}
