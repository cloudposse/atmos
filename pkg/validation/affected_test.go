package validation

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	cockroachErrors "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestAffectedFilesIncludesCommittedWorkingTreeAndUntrackedFiles(t *testing.T) {
	originalRunGit := runGit
	t.Cleanup(func() { runGit = originalRunGit })

	var calls [][]string
	runGit = func(args ...string) (string, error) {
		calls = append(calls, args)
		switch len(calls) {
		case 1:
			return "base-sha\n", nil
		case 2:
			return "committed.yaml\x00duplicate.yaml\x00", nil
		case 3:
			return "working.yaml\x00duplicate.yaml\x00", nil
		case 4:
			return "staged.yaml\x00duplicate.yaml\x00", nil
		case 5:
			return "untracked.yaml\x00", nil
		default:
			return "", errors.New("unexpected git invocation")
		}
	}

	paths, err := AffectedFiles("main")
	require.NoError(t, err)
	assert.Equal(t, []string{"committed.yaml", "duplicate.yaml", "working.yaml", "staged.yaml", "untracked.yaml"}, paths)
	assert.Equal(t, []string{"merge-base", "HEAD", "main"}, calls[0])
	assert.Equal(t, []string{"diff", "--name-only", "-z", "--diff-filter=ACMRD", "base-sha...HEAD"}, calls[1])
	assert.Equal(t, []string{"diff", "--cached", "--name-only", "-z", "--diff-filter=ACMRD"}, calls[3])
	assert.Equal(t, []string{"ls-files", "--others", "--exclude-standard", "--full-name", "-z", "--", ":/"}, calls[4])
}

func TestExcludePaths(t *testing.T) {
	paths := []string{
		"atmos.yaml",
		"tests/fixtures/scenarios/invalid/stack.yaml",
		"tests/fixtures/valid.yaml",
		"stacks/dev.yaml",
	}

	filtered, err := ExcludePaths(paths, []string{"tests/fixtures/**"})
	require.NoError(t, err)
	assert.Equal(t, []string{"atmos.yaml", "stacks/dev.yaml"}, filtered)

	filtered, err = ExcludePaths([]string{"tests\\fixtures\\invalid.yaml", "stacks/dev.yaml"}, []string{"tests/fixtures/**"})
	require.NoError(t, err)
	assert.Equal(t, []string{"stacks/dev.yaml"}, filtered)

	_, err = ExcludePaths(paths, []string{"["})
	assert.Error(t, err)
	_, err = ExcludePaths(paths, []string{""})
	assert.Error(t, err)
}

// TestValidationRepositoryPath_CrossVolumeFallsBackToAbsolute guards against a
// regression where GitHub Actions Windows runners check out the repo on one
// drive letter while t.TempDir() and other temp paths live on another, and
// filepath.Rel errors on cross-volume paths on Windows. That used to fail the
// whole ExcludePaths call for any absolute path outside the repo, as hit for
// real by TestFilterValidationSchemaFiles in internal/exec. The fallback here
// keeps the absolute path instead of erroring.
func TestValidationRepositoryPath_CrossVolumeFallsBackToAbsolute(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("cross-volume filepath.Rel failures are Windows-specific")
	}

	cwd, err := os.Getwd()
	require.NoError(t, err)
	otherVolume := "E:"
	if filepath.VolumeName(cwd) == otherVolume {
		otherVolume = "F:"
	}

	got, err := validationRepositoryPath(otherVolume + `\temp\first.yaml`)
	require.NoError(t, err)
	assert.Equal(t, otherVolume+"/temp/first.yaml", got)
}

func TestResolveAffectedBase(t *testing.T) {
	// GITHUB_EVENT_PATH is set by real GitHub Actions runners and takes precedence
	// over GITHUB_BASE_REF in resolveAffectedBase; clear it so this test is not at
	// the mercy of the CI environment it happens to run in.
	t.Setenv("GITHUB_EVENT_PATH", "")
	t.Setenv("GITHUB_BASE_REF", "main")

	base, explicit := resolveAffectedBase("abc123")
	assert.Equal(t, "abc123", base)
	assert.True(t, explicit)

	base, explicit = resolveAffectedBase("")
	assert.Equal(t, "origin/main", base)
	assert.False(t, explicit)
}

// stubRunGit replaces runGit for one test and restores it afterwards.
func stubRunGit(t *testing.T, fn func(args ...string) (string, error)) {
	t.Helper()
	original := runGit
	t.Cleanup(func() { runGit = original })
	runGit = fn
}

func TestResolveAffectedMergeBaseClassifiesGitFailures(t *testing.T) {
	t.Setenv("GITHUB_EVENT_PATH", "")
	t.Setenv("GITHUB_BASE_REF", "")
	notRepository := &gitCommandError{err: errors.New("exit status 128"), stderr: "fatal: not a git repository (or any of the parent directories): .git"}
	unknownRef := &gitCommandError{err: errors.New("exit status 128"), stderr: "fatal: Not a valid object name origin/HEAD"}

	t.Run("outside a repository", func(t *testing.T) {
		stubRunGit(t, func(...string) (string, error) { return "", notRepository })
		for _, base := range []string{"", "main"} {
			_, err := resolveAffectedMergeBase(base)
			require.ErrorIs(t, err, errUtils.ErrNotInGitRepository)
			assert.Contains(t, err.Error(), "run inside a repository or drop --affected")
			assert.NotContains(t, err.Error(), "exit status")
			assert.Contains(t, strings.Join(cockroachErrors.GetAllHints(err), " "), "--affected compares against Git history; run inside a repository or drop --affected")
		}
	})

	t.Run("default base is unresolved", func(t *testing.T) {
		var calls [][]string
		stubRunGit(t, func(args ...string) (string, error) {
			calls = append(calls, args)
			return "", unknownRef
		})
		_, err := resolveAffectedMergeBase("")
		require.ErrorIs(t, err, errUtils.ErrAffectedBaseUnresolved)
		require.NotErrorIs(t, err, errUtils.ErrNotInGitRepository)
		assert.Contains(t, err.Error(), `"origin/HEAD"`)
		assert.Contains(t, err.Error(), "pass --base=<ref>")
		assert.Contains(t, err.Error(), "Not a valid object name origin/HEAD", "git's own explanation must survive")
		hints := strings.Join(cockroachErrors.GetAllHints(err), " ")
		assert.Contains(t, hints, "--base=")
		assert.Contains(t, hints, "git remote set-head origin --auto")
		require.Len(t, calls, 2, "the HEAD~1 fallback is still tried for the default base")
	})

	t.Run("explicit base keeps its name", func(t *testing.T) {
		stubRunGit(t, func(args ...string) (string, error) {
			return "", &gitCommandError{err: errors.New("exit status 128"), stderr: "fatal: Not a valid object name no-such-ref"}
		})
		_, err := resolveAffectedMergeBase("no-such-ref")
		require.ErrorIs(t, err, errUtils.ErrAffectedBaseUnresolved)
		assert.Contains(t, err.Error(), `"no-such-ref"`)
		assert.NotContains(t, err.Error(), "pass --base=<ref>", "the user already passed --base")
		assert.NotContains(t, strings.Join(cockroachErrors.GetAllHints(err), " "), "set-head")
	})

	t.Run("default base falls back to the previous commit", func(t *testing.T) {
		stubRunGit(t, func(args ...string) (string, error) {
			if args[len(args)-1] == "HEAD~1" {
				return "parent-sha\n", nil
			}
			return "", unknownRef
		})
		mergeBase, err := resolveAffectedMergeBase("")
		require.NoError(t, err)
		assert.Equal(t, "parent-sha\n", mergeBase)
	})
}

func TestRunGitCapturesStderr(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is unavailable")
	}
	dir := t.TempDir()
	t.Chdir(dir)
	// Keep git from discovering a repository in an ancestor of the temporary directory.
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))

	_, err := runGit("rev-parse", "--show-toplevel")
	require.Error(t, err)
	var gitErr *gitCommandError
	require.ErrorAs(t, err, &gitErr)
	assert.Contains(t, gitErr.stderr, "not a git repository")
	assert.True(t, isNotGitRepository(err))
}
