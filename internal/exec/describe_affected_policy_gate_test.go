package exec

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestAffectedPolicyGateDiscovery(t *testing.T) {
	t.Setenv("ATMOS_POLICY_DIR", "")
	t.Chdir(t.TempDir())
	// No applicable rules must not require repositories or load configuration.
	require.NoError(t, enforceAffectedPolicies(nil, nil, nil, "", ""))
	require.NoError(t, os.WriteFile("policy.d", []byte("not a directory"), 0o600))
	require.ErrorIs(t, enforceAffectedPolicies(nil, nil, nil, "", ""), errUtils.ErrPolicyInvalid)
	t.Setenv("ATMOS_POLICY_DIR", t.TempDir())
	require.ErrorIs(t, enforceAffectedPolicies(nil, nil, nil, "", ""), errUtils.ErrPolicyInvalid)
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
