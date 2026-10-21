package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestTransactionRecoversPartialCommitAndProtectsExternalEdits(t *testing.T) {
	project := t.TempDir()
	changes := []Change{
		{Name: "atmos.yaml", Before: []byte("before"), After: []byte("after")},
		{Name: ".atmos.plugins.lock.yaml", Before: nil, After: []byte("locked")},
	}
	require.NoError(t, os.MkdirAll(filepath.Dir(journalPath(project)), 0o700))
	raw, err := json.Marshal(changes)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(journalPath(project), raw, 0o600))
	// Simulate a crash after the first rename and before the lock replacement.
	require.NoError(t, os.WriteFile(filepath.Join(project, "atmos.yaml"), []byte("after"), 0o644))
	require.ErrorIs(t, CheckTransaction(project), errUtils.ErrPluginTransactionPending)
	require.NoError(t, Mutate(t.Context(), project, func() ([]Change, error) { return nil, nil }))
	locked, err := os.ReadFile(filepath.Join(project, ".atmos.plugins.lock.yaml"))
	require.NoError(t, err)
	require.Equal(t, "locked", string(locked))
	require.NoError(t, CheckTransaction(project))
	require.NoError(t, os.WriteFile(journalPath(project), raw, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(project, "atmos.yaml"), []byte("external edit"), 0o644))
	require.ErrorIs(t, Mutate(t.Context(), project, func() ([]Change, error) { return nil, nil }), errUtils.ErrPluginTransactionPending)
	actual, err := os.ReadFile(filepath.Join(project, "atmos.yaml"))
	require.NoError(t, err)
	require.Equal(t, "external edit", string(actual))
	require.FileExists(t, journalPath(project), "recovery copies must survive a conflict")
}

func TestTransactionPreservesPrivateConfigurationPermissions(t *testing.T) {
	project := t.TempDir()
	path := filepath.Join(project, "atmos.yaml")
	require.NoError(t, os.WriteFile(path, []byte("before"), 0o600))
	original, err := os.Stat(path)
	require.NoError(t, err)
	require.NoError(t, Mutate(t.Context(), project, func() ([]Change, error) {
		return []Change{{Name: "atmos.yaml", Before: []byte("before"), After: []byte("after")}}, nil
	}))
	info, err := os.Stat(path)
	require.NoError(t, err)
	// Compare the permissions the platform actually exposes: Windows does not implement POSIX mode bits.
	require.Equal(t, original.Mode().Perm(), info.Mode().Perm())
	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "after", string(contents))
}
