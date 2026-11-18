package oci

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// A layout whose index cannot be inspected is an inaccessible repository, not an absent one:
// listing must report the failure instead of an empty collection, and reading must not claim the
// layout does not exist.
func TestLayoutBackend_UninspectableLayoutIsNotAbsent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skipf("a file used as a directory reports a not-found error on Windows")
	}
	dir := t.TempDir()
	blocker := filepath.Join(dir, "not-a-directory")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o600))
	backend := newLayoutBackend(blocker)

	tests := []struct {
		name    string
		call    func() error
		wantErr error
	}{
		{name: "references", call: func() error { _, err := backend.references(context.Background()); return err }, wantErr: errUtils.ErrArtifactListFailed},
		{name: "versions", call: func() error { _, err := backend.versions(context.Background(), "team/app"); return err }, wantErr: errUtils.ErrArtifactListFailed},
		{name: "names", call: func() error { _, err := backend.names(context.Background()); return err }, wantErr: errUtils.ErrArtifactListFailed},
		{name: "layout read", call: func() error { _, err := backend.layout(context.Background(), false); return err }, wantErr: errUtils.ErrArtifactDownloadFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			require.Error(t, err)
			assert.NotErrorIs(t, err, errUtils.ErrArtifactNotFound)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

// An absent layout stays an empty collection for listing and not-found for reading.
func TestLayoutBackend_AbsentLayoutStaysAbsent(t *testing.T) {
	backend := newLayoutBackend(filepath.Join(t.TempDir(), "missing"))

	refs, err := backend.references(context.Background())
	require.NoError(t, err)
	assert.Empty(t, refs)

	_, err = backend.layout(context.Background(), false)
	require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
}
