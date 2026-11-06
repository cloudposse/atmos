package exec

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/policy"
)

const (
	affectedDenyEffect     = "      action: deny\n      reason: Requires review.\n"
	affectedApprovalEffect = "      action: require_approval\n      approvals:\n        - id: platform\n          provider: github-actions\n          spec: {reviewers: [alice], minimum: 1}\n"
)

func affectedGateRules(t *testing.T, expression, effect string) []policy.Rule {
	t.Helper()
	directory := t.TempDir()
	document := "apiVersion: atmos/v1\nkind: Policy\nmetadata: {name: guard}\nspec:\n  policies:\n    guard:\n" +
		"      on: atmos.affected\n      rule: \"" + expression + "\"\n" + effect
	require.NoError(t, os.WriteFile(filepath.Join(directory, "guard.yaml"), []byte(document), 0o600))
	bundle, err := policy.LoadDirectory(directory)
	require.NoError(t, err)
	rules := bundle.RulesFor(policy.AtmosAffected)
	require.Len(t, rules, 1)
	return rules
}

// affectedGateRequest builds a well-formed affected input that carries a sensitive value.
func affectedGateRequest(t *testing.T) *policy.Request {
	t.Helper()
	snapshot := func(password string) policy.InventorySnapshot {
		return policy.InventorySnapshot{Stacks: map[string]map[string]map[string]json.RawMessage{
			"prod": {"helm": {"app": json.RawMessage(`{"vars":{"password":"` + password + `"}}`)}},
		}}
	}
	input, err := policy.BuildAffectedInput(snapshot("hunter1"), snapshot("hunter2"), nil, nil)
	require.NoError(t, err)
	data, err := json.Marshal(input)
	require.NoError(t, err)
	require.Contains(t, string(data), "hunter2", "the fixture must carry a sensitive inventory value")
	return &policy.Request{Kind: policy.AtmosAffected, Input: data}
}

func TestEvaluateAffectedPoliciesWithholdsInputOnEvaluationFailure(t *testing.T) {
	request := affectedGateRequest(t)

	err := evaluateAffectedPolicies(request, affectedGateRules(t, "input.missing.field == 'trigger'", affectedDenyEffect))
	require.ErrorIs(t, err, errUtils.ErrPolicyEvaluation)
	require.NotContains(t, err.Error(), "hunter2", "inventory values may be sensitive")

	// A rule that evaluates cleanly reports a decision, not an evaluation failure.
	err = evaluateAffectedPolicies(request, affectedGateRules(t, "true", affectedDenyEffect))
	require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
	require.NotErrorIs(t, err, errUtils.ErrPolicyEvaluation)
}

func TestEvaluateAffectedPoliciesFailsClosedWithoutApprovalProvider(t *testing.T) {
	restore := ci.SwapRegistryForTest()
	defer restore()
	request := affectedGateRequest(t)

	// No CI provider can verify reviews, so a pending approval is never treated as granted.
	err := evaluateAffectedPolicies(request, affectedGateRules(t, "true", affectedApprovalEffect))
	require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
	require.NotErrorIs(t, err, errUtils.ErrPolicyDenied)

	// A non-matching approval rule never consults the provider.
	require.NoError(t, evaluateAffectedPolicies(request, affectedGateRules(t, "false", affectedApprovalEffect)))
}

func TestAffectedPolicyGateDiscovery(t *testing.T) {
	t.Setenv("ATMOS_POLICY_DIR", "")
	t.Chdir(t.TempDir())
	// No applicable rules must not require repositories or load configuration.
	require.NoError(t, enforceAffectedPolicies(nil, nil, nil, "", ""))
	require.NoError(t, os.WriteFile("policy.d", []byte("not a directory"), 0o600))
	require.ErrorIs(t, enforceAffectedPolicies(nil, nil, nil, "", ""), errUtils.ErrPolicyInvalid)
	t.Setenv("ATMOS_POLICY_DIR", t.TempDir())
	require.ErrorIs(t, enforceAffectedPolicies(nil, nil, nil, "", ""), errUtils.ErrNoPoliciesConfigured)
	directory := t.TempDir()
	t.Setenv("ATMOS_POLICY_DIR", directory)
	writeAffectedPolicyGateFixture(t, directory, "operation")
	require.NoError(t, enforceAffectedPolicies(nil, nil, nil, "", ""))
}

func writeAffectedPolicyGateFixture(t *testing.T, directory, kind string) {
	t.Helper()
	document := `apiVersion: atmos/v1
kind: Policy
metadata: {name: guard}
spec:
  policies:
    deny:
      on: INPUT_KIND
      rule: "true"
      action: deny
      reason: Requires review.
`
	require.NoError(t, os.WriteFile(filepath.Join(directory, "guard.yaml"), []byte(strings.ReplaceAll(document, "INPUT_KIND", kind)), 0o600))
}

func TestAffectedPolicyGateRejectsDirtyComparison(t *testing.T) {
	root, repo := affectedPolicyRepo(t, map[string]string{"stacks/prod.yaml": affectedTestManifest})
	directory := t.TempDir()
	t.Setenv("ATMOS_POLICY_DIR", directory)
	writeAffectedPolicyGateFixture(t, directory, "atmos.affected")
	require.NoError(t, os.WriteFile(filepath.Join(root, "untracked"), []byte("dirty"), 0o600))
	err := enforceAffectedPolicies(affectedPolicyTestConfig(root), repo, repo, root, root)
	require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
	require.ErrorContains(t, err, "clean comparison worktrees")
	require.ErrorIs(t, verifyAffectedPolicyRevision(repo, "any"), errUtils.ErrPolicyInputUnavailable)
}

func TestAffectedPolicyCollectionRejectsInvalidBase(t *testing.T) {
	headPath, head := affectedPolicyRepo(t, map[string]string{"stacks/prod.yaml": affectedTestManifest})
	for name, files := range map[string]map[string]string{
		"missing base import": {"stacks/prod.yaml": "import: [missing]\n" + affectedTestManifest},
		"dirty base":          {"stacks/prod.yaml": affectedTestManifest},
	} {
		t.Run(name, func(t *testing.T) {
			basePath, base := affectedPolicyRepo(t, files)
			if name == "dirty base" {
				require.NoError(t, os.WriteFile(filepath.Join(basePath, "untracked"), []byte("dirty"), 0o600))
			}
			request, err := collectAffectedPolicyRequest(affectedPolicyTestConfig(headPath), head, base, headPath, basePath)
			require.Error(t, err)
			require.Nil(t, request)
		})
	}
	require.ErrorIs(t, verifyAffectedPolicyRevisions(head, head, "stale", "stale"), errUtils.ErrPolicyInputUnavailable)
}

func TestAffectedPolicyTreeRejectsUnavailableHistory(t *testing.T) {
	t.Run("bare repository", func(t *testing.T) {
		repo, err := git.PlainInit(t.TempDir(), true)
		require.NoError(t, err)
		_, _, err = affectedPolicyTree(repo)
		require.ErrorIs(t, err, git.ErrIsBareRepository)
	})
	t.Run("unborn head", func(t *testing.T) {
		repo, err := git.PlainInit(t.TempDir(), false)
		require.NoError(t, err)
		_, _, err = affectedPolicyTree(repo)
		require.ErrorIs(t, err, plumbing.ErrReferenceNotFound)
	})
	t.Run("missing commit", func(t *testing.T) {
		repo, err := git.PlainInit(t.TempDir(), false)
		require.NoError(t, err)
		require.NoError(t, repo.Storer.SetReference(plumbing.NewHashReference(plumbing.HEAD, plumbing.NewHash(strings.Repeat("a", 40)))))
		_, _, err = affectedPolicyTree(repo)
		require.ErrorIs(t, err, plumbing.ErrObjectNotFound)
	})
}

// go-git ignores core.excludesFile, so the clean-worktree check loads the user's global excludes itself.
func TestAffectedPolicyTreeHonorsGlobalExcludes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("global excludes load through a chrooted root filesystem that Windows paths do not map onto")
	}
	root, repo := affectedPolicyRepo(t, map[string]string{"stacks/prod.yaml": affectedTestManifest})
	require.NoError(t, os.WriteFile(filepath.Join(root, "build.log"), []byte("generated"), 0o600))

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	_, _, err := affectedPolicyTree(repo)
	require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable, "an untracked file blocks the analysis without global excludes")

	excludes := filepath.Join(home, "global-ignore")
	require.NoError(t, os.WriteFile(excludes, []byte("*.log\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(home, ".gitconfig"), []byte("[core]\n\texcludesfile = "+filepath.ToSlash(excludes)+"\n"), 0o600))
	tree, _, err := affectedPolicyTree(repo)
	require.NoError(t, err, "a file ignored only by the global excludes is not a dirty worktree")
	require.NotNil(t, tree)

	require.NoError(t, os.WriteFile(filepath.Join(root, "notes.txt"), []byte("not ignored"), 0o600))
	_, _, err = affectedPolicyTree(repo)
	require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable, "other untracked files still block it")
}
