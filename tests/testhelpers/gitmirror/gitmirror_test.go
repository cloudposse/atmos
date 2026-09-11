package gitmirror

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestBuild verifies Build produces a bare mirror at <root>/cloudposse/atmos.git on
// branch main whose tree contains examples/ (the only subtree any fixture vendors
// from cloudposse/atmos), and that it's actually clonable over file://.
func TestBuild(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, Build(root))

	bareDir := filepath.Join(root, Owner, Repo+".git")
	info, err := os.Stat(bareDir)
	require.NoError(t, err)
	require.True(t, info.IsDir())

	// A bare repo has HEAD, refs/, objects/ directly under it (no .git subdir).
	_, err = os.Stat(filepath.Join(bareDir, "HEAD"))
	require.NoError(t, err, "expected a bare git repo at %s", bareDir)

	clone := t.TempDir()
	cloneDir := filepath.Join(clone, "checkout")
	cloneCmd := exec.Command("git", "clone", "--quiet", "--branch", "main", bareDir, cloneDir)
	cloneCmd.Env = gitEnv() // Verify with the same scrubbed env Build uses.
	out, err := cloneCmd.CombinedOutput()
	require.NoError(t, err, "git clone failed: %s", out)

	// The mirror's root must directly contain examples/, mirroring the same relative
	// layout a real "github.com/cloudposse/atmos.git//examples/..." fetch would see.
	entries, err := os.ReadDir(filepath.Join(cloneDir, "examples"))
	require.NoError(t, err)
	require.NotEmpty(t, entries, "examples/ must not be empty in the mirror")

	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	require.Contains(t, names, "demo-library")
	require.Contains(t, names, "demo-stacks")
	require.Contains(t, names, "demo-helmfile")

	// A representative leaf file used by tests/test-cases/vendor-test.yaml.
	weatherMain := filepath.Join(cloneDir, "examples", "demo-library", "weather", "main.tf")
	_, err = os.Stat(weatherMain)
	require.NoError(t, err, "expected %s to exist in the mirror", weatherMain)

	// The mirror must not carry a nested .git directory under examples/ (would confuse
	// git operations on the outer repo).
	_, err = os.Stat(filepath.Join(cloneDir, "examples", ".git"))
	require.True(t, os.IsNotExist(err))
}

// TestFileURI verifies FileURI produces a well-formed file:// URI, including the
// Windows volume-name case (file:///C:/...).
func TestFileURI(t *testing.T) {
	if runtime.GOOS == "windows" {
		uri := FileURI(`C:\repo\mirror`)
		require.Equal(t, "file:///C:/repo/mirror", uri)
		return
	}

	uri := FileURI("/tmp/mirror/cloudposse")
	require.Equal(t, "file:///tmp/mirror/cloudposse", uri)
}
