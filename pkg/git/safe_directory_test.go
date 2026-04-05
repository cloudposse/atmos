package git

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsureSafeDirectory_NoOp_WhenWorkspaceUnset(t *testing.T) {
	// GITHUB_WORKSPACE is not set in normal test environments.
	// EnsureSafeDirectory should return nil without running any git commands.
	t.Setenv("GITHUB_WORKSPACE", "")
	err := EnsureSafeDirectory()
	assert.NoError(t, err)
}

func TestEnsureSafeDirectory_ConfiguresSafeDirectory(t *testing.T) {
	// Set GITHUB_WORKSPACE to the test's temp dir (which exists and is safe).
	tmpDir := t.TempDir()
	t.Setenv("GITHUB_WORKSPACE", tmpDir)

	// Sandbox git global config to avoid mutating the real global config.
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(fakeHome, ".config"))
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", fakeHome)
	}

	err := EnsureSafeDirectory()
	require.NoError(t, err)

	// Verify the safe.directory was actually set.
	cmd := exec.Command("git", "config", "--global", "--get-all", "safe.directory")
	output, err := cmd.Output()
	require.NoError(t, err)

	values := strings.Split(strings.TrimSpace(string(output)), "\n")
	assert.Contains(t, values, filepath.Clean(tmpDir))
}
