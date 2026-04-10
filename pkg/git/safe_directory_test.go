package git

import (
	"os/exec"
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
		t.Setenv("GITHUB_ACTIONS", "true")
		t.Setenv("GITHUB_WORKSPACE", "/tmp/test-workspace")

		err := EnsureGitSafeDirectory()
		require.NoError(t, err)

		// Verify git config was set.
		out, err := exec.Command("git", "config", "--global", "--get-all", "safe.directory").Output()
		require.NoError(t, err)
		assert.Contains(t, string(out), "/tmp/test-workspace")
	})
}
