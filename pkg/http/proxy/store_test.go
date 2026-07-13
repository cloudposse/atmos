package proxy

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func TestFileStore_CommitStatOpenRoundTrip(t *testing.T) {
	store := NewFileStore(t.TempDir())
	payload := []byte("provider-zip-bytes")
	key := "providers/registry.terraform.io/hashicorp/aws/terraform-provider-aws_5.95.0_linux_amd64.zip"

	meta, err := store.Commit(t.Context(), CommitRequest{
		Key:         key,
		Data:        bytes.NewReader(payload),
		Kind:        KindArtifact,
		ContentType: "application/zip",
	})
	require.NoError(t, err)
	assert.Equal(t, int64(len(payload)), meta.Size)
	assert.Equal(t, sha256Hex(payload), meta.SHA256)
	assert.Equal(t, KindArtifact, meta.Kind)
	assert.Equal(t, "application/zip", meta.ContentType)

	// The object and its sidecar exist on disk under the nested key path.
	objPath := filepath.Join(store.Root(), filepath.FromSlash(key))
	_, err = os.Stat(objPath)
	require.NoError(t, err)
	_, err = os.Stat(objPath + metadataSuffix)
	require.NoError(t, err)

	// Stat returns matching metadata.
	got, ok, err := store.Stat(key)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, meta.Size, got.Size)
	assert.Equal(t, meta.SHA256, got.SHA256)
	assert.Equal(t, KindArtifact, got.Kind)
	assert.Equal(t, "application/zip", got.ContentType)
	assert.False(t, got.FetchedAt.IsZero())

	// Open returns the stored bytes.
	rc, openMeta, err := store.Open(key)
	require.NoError(t, err)
	defer rc.Close()
	body, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, payload, body)
	assert.Equal(t, meta.SHA256, openMeta.SHA256)
}

func TestFileStore_CommitVerifyRejection(t *testing.T) {
	store := NewFileStore(t.TempDir())
	key := "objects/bad.zip"
	wantErr := errors.New("hash mismatch")

	_, err := store.Commit(t.Context(), CommitRequest{
		Key:    key,
		Data:   bytes.NewReader([]byte("corrupt")),
		Kind:   KindArtifact,
		Verify: func(string) error { return wantErr },
	})
	require.ErrorIs(t, err, wantErr)

	// Nothing was committed and no temp file was left behind.
	_, ok, statErr := store.Stat(key)
	require.NoError(t, statErr)
	assert.False(t, ok)

	dir := filepath.Join(store.Root(), "objects")
	entries, derr := os.ReadDir(dir)
	require.NoError(t, derr)
	assert.Empty(t, entries, "verify failure must clean up the staged temp object")
}

// errReader fails partway through a read to exercise the commit copy-error path.
type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

func TestFileStore_CommitDataReadError(t *testing.T) {
	store := NewFileStore(t.TempDir())
	_, err := store.Commit(t.Context(), CommitRequest{
		Key:  "objects/broken.zip",
		Data: errReader{},
		Kind: KindArtifact,
	})
	require.Error(t, err)

	// The failed write leaves no committed object behind.
	_, ok, statErr := store.Stat("objects/broken.zip")
	require.NoError(t, statErr)
	assert.False(t, ok)
}

func TestFileStore_StatMissing(t *testing.T) {
	store := NewFileStore(t.TempDir())
	meta, ok, err := store.Stat("objects/nope.zip")
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Equal(t, Meta{}, meta)
}

func TestFileStore_StatDirectoryIsNotAnObject(t *testing.T) {
	store := NewFileStore(t.TempDir())
	require.NoError(t, os.MkdirAll(filepath.Join(store.Root(), "objects", "dir"), storeDirPerm))
	_, ok, err := store.Stat("objects/dir")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestFileStore_StatWithoutSidecarFallsBackToModTime(t *testing.T) {
	store := NewFileStore(t.TempDir())
	key := "objects/no-sidecar.bin"
	objPath := filepath.Join(store.Root(), filepath.FromSlash(key))
	require.NoError(t, os.MkdirAll(filepath.Dir(objPath), storeDirPerm))
	require.NoError(t, os.WriteFile(objPath, []byte("data"), storeFilePerm))

	meta, ok, err := store.Stat(key)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, int64(4), meta.Size)
	assert.False(t, meta.FetchedAt.IsZero(), "FetchedAt falls back to the file mtime")
	assert.Empty(t, meta.SHA256)
}

func TestFileStore_OpenMissing(t *testing.T) {
	store := NewFileStore(t.TempDir())
	_, _, err := store.Open("objects/missing.zip")
	require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
}

func TestFileStore_SidecarRoundTrip(t *testing.T) {
	store := NewFileStore(t.TempDir())
	key := "metadata/version.json"
	_, err := store.Commit(t.Context(), CommitRequest{
		Key:         key,
		Data:        bytes.NewReader([]byte(`{"versions":{}}`)),
		Kind:        KindMetadata,
		ContentType: "application/json",
	})
	require.NoError(t, err)

	sc, ok := store.readSidecar(key)
	require.True(t, ok)
	assert.Equal(t, "metadata", sc.Custom.Kind)
	assert.Equal(t, "application/json", sc.Custom.ContentType)
	assert.NotEmpty(t, sc.Custom.FetchedAt)
}

func TestKindFromString(t *testing.T) {
	tests := []struct {
		in   string
		want ArtifactKind
	}{
		{"metadata", KindMetadata},
		{"artifact", KindArtifact},
		{"passthrough", KindPassthrough},
		{"", KindMetadata},
		{"garbage", KindMetadata},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			assert.Equal(t, tt.want, kindFromString(tt.in))
		})
	}
}
