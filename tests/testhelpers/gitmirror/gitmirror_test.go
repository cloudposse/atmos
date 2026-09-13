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
	// Clone through FileURI (rather than passing bareDir as a plain local path) so this test
	// actually exercises the documented file:// flow, including FileURI's Windows volume-name
	// handling.
	cloneCmd := exec.Command("git", "clone", "--quiet", "--branch", "main", FileURI(bareDir), cloneDir)
	cloneCmd.Env = buildEnv() // Verify with the same fully-scrubbed env Build uses.
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

	// The mirror must not carry a .git entry anywhere under examples/, not just at its root
	// (would confuse git operations on the outer repo, or turn part of the copy into a
	// submodule-like gitlink).
	require.NoError(t, filepath.WalkDir(filepath.Join(cloneDir, "examples"), func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.Name() == ".git" {
			t.Errorf("unexpected .git entry in mirror: %s", path)
		}
		return nil
	}))
}

// TestBuild_RelativeRoot verifies Build resolves a relative root against the caller's current
// working directory, not against whatever directory a later step happens to run in -- the push
// into the bare mirror runs with cmd.Dir set to a scratch directory, so passing the relative root
// straight through to that push (rather than resolving it to absolute first) would silently
// create the bare repo under the scratch directory instead of the caller's intended location.
// Deliberately does not chdir the process: the relative path is built with enough ".."
// components (via filepath.Rel) to reach a t.TempDir() location from the current working
// directory, so a wrong resolution base is easy to detect (the mirror simply won't exist at the
// intended absolute location).
func TestBuild_RelativeRoot(t *testing.T) {
	target := t.TempDir()

	cwd, err := os.Getwd()
	require.NoError(t, err)
	relRoot, err := filepath.Rel(cwd, target)
	require.NoError(t, err)

	require.NoError(t, Build(relRoot))

	bareDir := filepath.Join(target, Owner, Repo+".git")
	_, err = os.Stat(filepath.Join(bareDir, "HEAD"))
	require.NoError(t, err, "expected the mirror to be built at the resolved absolute location %s, not elsewhere", target)
}

// TestFilterGitDirEnvCaseInsensitive verifies filterGitDirEnv drops GIT_DIR-family variables
// regardless of the casing the environment happens to carry them in. Windows resolves
// environment variable names case-insensitively, so a lower/mixed-case entry (e.g. from a
// caller's shell profile) must be stripped exactly like the canonical uppercase form, and an
// unrelated variable like PATH must survive untouched.
func TestFilterGitDirEnvCaseInsensitive(t *testing.T) {
	env := []string{
		"git_dir=/some/repo/.git",
		"Git_Work_Tree=/some/repo",
		"GIT_INDEX_FILE=/some/repo/.git/index",
		"PATH=/usr/bin:/bin",
	}

	kept := filterGitDirEnv(env)

	require.NotContains(t, kept, "git_dir=/some/repo/.git")
	require.NotContains(t, kept, "Git_Work_Tree=/some/repo")
	require.NotContains(t, kept, "GIT_INDEX_FILE=/some/repo/.git/index")
	require.Contains(t, kept, "PATH=/usr/bin:/bin")
}

// TestFilterGitConfigEnvCaseInsensitive verifies filterGitConfigEnv drops all the
// GIT_CONFIG_COUNT/KEY_n/VALUE_n/PARAMETERS forms of ambient git configuration regardless of
// casing, since Windows resolves environment variable names case-insensitively and a
// lower/mixed-case entry would otherwise slip through and stay active for git. It is pure (no
// git invocation) and runs identically on every OS.
func TestFilterGitConfigEnvCaseInsensitive(t *testing.T) {
	env := []string{
		"git_config_count=1",
		"Git_Config_Key_0=user.name",
		"git_config_value_0=Someone",
		"GIT_CONFIG_PARAMETERS='user.name=Someone'",
		"PATH=/usr/bin:/bin",
	}

	kept := filterGitConfigEnv(env)

	require.NotContains(t, kept, "git_config_count=1")
	require.NotContains(t, kept, "Git_Config_Key_0=user.name")
	require.NotContains(t, kept, "git_config_value_0=Someone")
	require.NotContains(t, kept, "GIT_CONFIG_PARAMETERS='user.name=Someone'")
	require.Contains(t, kept, "PATH=/usr/bin:/bin")
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
