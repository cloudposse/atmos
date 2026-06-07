package provisioner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRestorePerInstanceLock_RestoresWhenPresent(t *testing.T) {
	srcDir := t.TempDir()
	workingDir := t.TempDir()
	cc := map[string]any{"atmos_stack": "dev", "atmos_component": "vpc"}

	want := "# pinned providers for dev-vpc\n"
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, InstanceLockFilename(cc)), []byte(want), 0o644))

	require.NoError(t, RestorePerInstanceLock(srcDir, workingDir, cc))

	got, err := os.ReadFile(filepath.Join(workingDir, CanonicalLockFilename))
	require.NoError(t, err)
	assert.Equal(t, want, string(got))
}

func TestRestorePerInstanceLock_NoopWhenAbsent(t *testing.T) {
	srcDir := t.TempDir()
	workingDir := t.TempDir()
	cc := map[string]any{"atmos_stack": "dev", "atmos_component": "vpc"}

	// No per-instance lock in srcDir.
	require.NoError(t, RestorePerInstanceLock(srcDir, workingDir, cc))

	// The canonical lock must NOT be created when there is nothing to restore.
	_, err := os.Stat(filepath.Join(workingDir, CanonicalLockFilename))
	assert.True(t, os.IsNotExist(err), "canonical lock should not be written when no per-instance lock exists")
}

func TestRestorePerInstanceLock_EmptyDirsAreNoop(t *testing.T) {
	cc := map[string]any{"atmos_stack": "dev", "atmos_component": "vpc"}
	assert.NoError(t, RestorePerInstanceLock("", t.TempDir(), cc))
	assert.NoError(t, RestorePerInstanceLock(t.TempDir(), "", cc))
}

func TestLockCoordPath_StableAndOutsideWorkingDir(t *testing.T) {
	working := t.TempDir()
	lockPath := filepath.Join(working, CanonicalLockFilename)

	a := LockCoordPath(lockPath)
	b := LockCoordPath(lockPath)
	assert.Equal(t, a, b, "coord path must be deterministic for the same lock path")
	assert.Equal(t, os.TempDir(), filepath.Dir(a), "coord path must live under the temp dir, not the component dir")
}
