package validation

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			return "untracked.yaml\x00", nil
		default:
			return "", errors.New("unexpected git invocation")
		}
	}

	paths, err := AffectedFiles("main")
	require.NoError(t, err)
	assert.Equal(t, []string{"committed.yaml", "duplicate.yaml", "working.yaml", "untracked.yaml"}, paths)
	assert.Equal(t, []string{"merge-base", "HEAD", "main"}, calls[0])
	assert.Equal(t, []string{"diff", "--name-only", "-z", "--diff-filter=ACMRD", "base-sha...HEAD"}, calls[1])
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
