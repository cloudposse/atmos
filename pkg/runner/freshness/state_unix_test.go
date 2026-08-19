//go:build !windows

package freshness

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestFileStateStore_SaveCreateTempErrorPropagates covers os.CreateTemp failing because stateDir
// itself is not writable, isolated from Save's own lock-acquisition (which would otherwise fail
// first against the exact same permission barrier, since the lock file lives alongside the record
// file in stateDir): the lock file is pre-created before stateDir is made read-only, so opening an
// *existing* file for locking needs no directory-write permission, while os.CreateTemp -- which
// must create a brand-new file -- does, isolating the CreateTemp branch specifically. Unix-only:
// Windows' permission model doesn't map onto POSIX mode bits the same way, so a chmod-based test
// there would be unreliable (see this package's own StateStore doc comment on Windows being
// best-effort).
func TestFileStateStore_SaveCreateTempErrorPropagates(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("Skipping permission test when running as root")
	}

	tmpDir := t.TempDir()
	stateDir := filepath.Join(tmpDir, "state")
	require.NoError(t, os.MkdirAll(stateDir, 0o755))

	key := "key"
	lockPath := filepath.Join(stateDir, key+".json.lock")
	require.NoError(t, os.WriteFile(lockPath, nil, 0o644))

	require.NoError(t, os.Chmod(stateDir, 0o500)) // read+exec, no write: blocks new-file creation.
	t.Cleanup(func() {
		_ = os.Chmod(stateDir, 0o755)
	})

	store := NewStateStore()
	err := store.Save(stateDir, key, Record{SourcesHash: "abc"})
	require.Error(t, err)
}
