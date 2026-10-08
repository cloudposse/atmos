package git

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsureGitSafeDirectory(t *testing.T) {
	t.Run("skips when not in GitHub Actions", func(t *testing.T) {
		t.Setenv("GITHUB_ACTIONS", "")
		err := EnsureGitSafeDirectory()
		require.NoError(t, err)
	})

	t.Run("skips when GITHUB_WORKSPACE is empty", func(t *testing.T) {
		t.Setenv("GITHUB_ACTIONS", "true")
		t.Setenv("GITHUB_WORKSPACE", "")
		err := EnsureGitSafeDirectory()
		require.NoError(t, err)
	})

	t.Run("adds safe directory in GitHub Actions", func(t *testing.T) {
		isolateGlobalGitConfig(t)
		workspace := filepath.Join(t.TempDir(), "test-workspace")
		t.Setenv("GITHUB_ACTIONS", "true")
		t.Setenv("GITHUB_WORKSPACE", workspace)

		err := EnsureGitSafeDirectory()
		require.NoError(t, err)

		// Verify git config was set.
		// filepath.Clean normalizes the path per-OS, so match against that.
		out, err := exec.Command("git", "config", "--global", "--get-all", "safe.directory").Output()
		require.NoError(t, err)
		assert.Contains(t, string(out), filepath.Clean(workspace))
	})

	t.Run("repeated calls add the directory once", func(t *testing.T) {
		isolateGlobalGitConfig(t)
		workspace := filepath.Join(t.TempDir(), "repeat-workspace")
		t.Setenv("GITHUB_ACTIONS", "true")
		t.Setenv("GITHUB_WORKSPACE", workspace)

		for range 3 {
			require.NoError(t, EnsureGitSafeDirectory())
		}

		out, err := exec.Command("git", "config", "--global", "--get-all", "safe.directory").Output()
		require.NoError(t, err)
		assert.Equal(t, 1, strings.Count(string(out), filepath.Clean(workspace)), "duplicates accumulate in the global git config")
	})

	t.Run("keeps other safe directories and adds a new one", func(t *testing.T) {
		isolateGlobalGitConfig(t)
		first := filepath.Join(t.TempDir(), "first")
		second := filepath.Join(t.TempDir(), "second")
		t.Setenv("GITHUB_ACTIONS", "true")

		t.Setenv("GITHUB_WORKSPACE", first)
		require.NoError(t, EnsureGitSafeDirectory())
		t.Setenv("GITHUB_WORKSPACE", second)
		require.NoError(t, EnsureGitSafeDirectory())
		require.NoError(t, EnsureGitSafeDirectory())

		out, err := exec.Command("git", "config", "--global", "--get-all", "safe.directory").Output()
		require.NoError(t, err)
		assert.Equal(t, 1, strings.Count(string(out), filepath.Clean(first)))
		assert.Equal(t, 1, strings.Count(string(out), filepath.Clean(second)))
	})
}

// isolateGlobalGitConfig points git's global config at a temporary file so tests never touch the real one.
func isolateGlobalGitConfig(t *testing.T) {
	t.Helper()

	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "gitconfig"))
}
