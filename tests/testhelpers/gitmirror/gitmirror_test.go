package gitmirror

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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
	out, err := exec.Command("git", "clone", "--quiet", "--branch", "main", bareDir, cloneDir).CombinedOutput()
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

// TestInsteadOfRules verifies the two per-owner rewrite rules (https and ssh) point at
// the mirror path and that InsteadOfRules covers every owner passed in.
func TestInsteadOfRules(t *testing.T) {
	root := "/mirror/root"
	entries := InsteadOfRules(root, "cloudposse", "example-org")
	require.Len(t, entries, 4)

	base := FileURI(filepath.Join(root, "cloudposse")) + "/"
	wantKey := "url." + base + ".insteadOf"
	require.Equal(t, wantKey, entries[0].Key)
	require.Equal(t, "https://github.com/cloudposse/", entries[0].Value)
	require.Equal(t, wantKey, entries[1].Key)
	require.Equal(t, "ssh://git@github.com/cloudposse/", entries[1].Value)

	otherBase := FileURI(filepath.Join(root, "example-org")) + "/"
	otherKey := "url." + otherBase + ".insteadOf"
	require.Equal(t, otherKey, entries[2].Key)
	require.Equal(t, "https://github.com/example-org/", entries[2].Value)
	require.Equal(t, otherKey, entries[3].Key)
	require.Equal(t, "ssh://git@github.com/example-org/", entries[3].Value)

	// Every generated key must actually start with the file:// scheme, or a stray
	// host-wide rule could slip in and reintroduce the token-injection pitfall
	// (see custom_git_detector.go): rules here must always be per-owner file:// targets.
	for _, e := range entries {
		require.True(t, strings.HasPrefix(e.Key, "url.file://"), "key %q must target a file:// mirror path", e.Key)
	}
}
