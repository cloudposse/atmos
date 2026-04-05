package git

import (
	"testing"

	"github.com/stretchr/testify/assert"
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

	err := EnsureSafeDirectory()
	assert.NoError(t, err)
}
