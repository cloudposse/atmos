package local

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/artifact"
)

// dirEntries lists every regular file under root as slash-separated relative paths.
func dirEntries(t *testing.T, root string) []string {
	t.Helper()
	var entries []string
	require.NoError(t, filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		require.NoError(t, err)
		if !d.IsDir() {
			rel, relErr := filepath.Rel(root, path)
			require.NoError(t, relErr)
			entries = append(entries, filepath.ToSlash(rel))
		}
		return nil
	}))
	sort.Strings(entries)
	return entries
}

// TestTransportKeepsTheLegacyOnDiskLayout pins the wire format: one file per artifact plus a
// ".metadata.json" sidecar holding indented JSON, and nothing else in the directory.
func TestTransportKeepsTheLegacyOnDiskLayout(t *testing.T) {
	root := t.TempDir()
	store := &Store{basePath: root}
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	metadata := &artifact.Metadata{Stack: "dev", Component: "vpc", SHA: "abc", CreatedAt: created, Custom: map[string]string{"k": "v"}}
	body := []byte("tar-bytes\x00\x01")

	require.NoError(t, store.Upload(context.Background(), "dev/vpc/abc.tar", bytes.NewReader(body), int64(len(body)), metadata))

	assert.Equal(t, []string{"dev/vpc/abc.tar", "dev/vpc/abc.tar.metadata.json"}, dirEntries(t, root))
	onDisk, err := os.ReadFile(filepath.Join(root, "dev", "vpc", "abc.tar"))
	require.NoError(t, err)
	assert.Equal(t, body, onDisk)
	sidecar, err := os.ReadFile(filepath.Join(root, "dev", "vpc", "abc.tar.metadata.json"))
	require.NoError(t, err)
	want, err := json.MarshalIndent(metadata, "", "  ")
	require.NoError(t, err)
	assert.Equal(t, want, sidecar, "the sidecar is byte-identical to the legacy encoding")
	if info, statErr := os.Stat(filepath.Join(root, "dev", "vpc", "abc.tar")); statErr == nil && filepath.Separator == '/' {
		assert.Equal(t, os.FileMode(0o644), info.Mode().Perm()&0o644)
	}
}

// TestBundledStoreTarBytesAndChecksumSurviveTheTransport verifies the tar archive round trip and
// the SHA-256 recorded in the sidecar still describe the stored bytes.
func TestBundledStoreTarBytesAndChecksumSurviveTheTransport(t *testing.T) {
	root := t.TempDir()
	bundled := artifact.NewBundledStore(&Store{basePath: root})
	files := []artifact.FileEntry{
		{Name: "plan.tfplan", Data: strings.NewReader("plan-bytes"), Size: 10},
		{Name: "lock.hcl", Data: strings.NewReader("lock"), Size: 4},
	}
	require.NoError(t, bundled.Upload(context.Background(), "dev/vpc/sha.tfplan.tar", files, &artifact.Metadata{Stack: "dev", Component: "vpc", SHA: "sha"}))

	stored, err := os.ReadFile(filepath.Join(root, "dev", "vpc", "sha.tfplan.tar"))
	require.NoError(t, err)
	sum := sha256.Sum256(stored)
	meta, err := bundled.GetMetadata(context.Background(), "dev/vpc/sha.tfplan.tar")
	require.NoError(t, err)
	assert.Equal(t, hex.EncodeToString(sum[:]), meta.SHA256)

	got, _, err := bundled.Download(context.Background(), "dev/vpc/sha.tfplan.tar")
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "plan.tfplan", got[0].Name)
	assert.Equal(t, "lock.hcl", got[1].Name)
}

// TestNamesAreCleanedLikeTheFilesystemDid pins that equivalent spellings address one artifact.
func TestNamesAreCleanedLikeTheFilesystemDid(t *testing.T) {
	root := t.TempDir()
	store := &Store{basePath: root}
	ctx := context.Background()
	require.NoError(t, store.Upload(ctx, "a/./b//c.tar", strings.NewReader("x"), 1, nil))
	assert.Equal(t, []string{"a/b/c.tar", "a/b/c.tar.metadata.json"}, dirEntries(t, root))

	exists, err := store.Exists(ctx, "a/b/c.tar")
	require.NoError(t, err)
	assert.True(t, exists)
	exists, err = store.Exists(ctx, "a/x/../b/c.tar")
	require.NoError(t, err)
	assert.True(t, exists)
}

// TestDirectoryNamesKeepTheirLegacyErrors pins error behavior for a name that is a directory.
func TestDirectoryNamesKeepTheirLegacyErrors(t *testing.T) {
	root := t.TempDir()
	store := &Store{basePath: root}
	ctx := context.Background()
	require.NoError(t, store.Upload(ctx, "dev/vpc/a.tar", strings.NewReader("x"), 1, nil))

	_, _, err := store.Download(ctx, "dev/vpc")
	require.ErrorIs(t, err, errUtils.ErrArtifactDownloadFailed)
	_, err = store.GetMetadata(ctx, "dev/vpc")
	require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
	exists, err := store.Exists(ctx, "dev/vpc")
	require.NoError(t, err)
	assert.False(t, exists)
}

// TestUploadReplacesAtomically checks a failed overwrite leaves the previous artifact intact.
func TestUploadReplacesAtomically(t *testing.T) {
	root := t.TempDir()
	store := &Store{basePath: root}
	ctx := context.Background()
	require.NoError(t, store.Upload(ctx, "a.tar", strings.NewReader("original"), 8, nil))

	err := store.Upload(ctx, "a.tar", &brokenReader{}, -1, nil)
	require.ErrorIs(t, err, errUtils.ErrArtifactUploadFailed)

	reader, _, err := store.Download(ctx, "a.tar")
	require.NoError(t, err)
	defer reader.Close()
	var buf bytes.Buffer
	_, err = buf.ReadFrom(reader)
	require.NoError(t, err)
	assert.Equal(t, "original", buf.String(), "a failed upload no longer truncates the existing artifact")
	for _, entry := range dirEntries(t, root) {
		assert.NotContains(t, entry, ".atmos-blob-tmp-", "no temporary files are left behind")
	}
}

type brokenReader struct{ done bool }

func (b *brokenReader) Read(p []byte) (int, error) {
	if !b.done {
		b.done = true
		return copy(p, "partial"), nil
	}
	return 0, os.ErrClosed
}

// TestListSkipsOrphanSidecarsAndFiltersByQuery covers the sidecar-driven listing semantics.
func TestListSkipsOrphanSidecarsAndFiltersByQuery(t *testing.T) {
	root := t.TempDir()
	store := &Store{basePath: root}
	ctx := context.Background()
	require.NoError(t, store.Upload(ctx, "dev/vpc/1.tar", strings.NewReader("a"), 1, &artifact.Metadata{Stack: "dev", Component: "vpc", SHA: "1"}))
	require.NoError(t, store.Upload(ctx, "prod/vpc/2.tar", strings.NewReader("bb"), 2, &artifact.Metadata{Stack: "prod", Component: "vpc", SHA: "2"}))
	require.NoError(t, os.WriteFile(filepath.Join(root, "orphan.tar.metadata.json"), []byte(`{"stack":"x"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "no-sidecar.tar"), []byte("data"), 0o644))

	all, err := store.List(ctx, artifact.Query{All: true})
	require.NoError(t, err)
	names := make([]string, 0, len(all))
	for _, info := range all {
		names = append(names, filepath.ToSlash(info.Name))
	}
	sort.Strings(names)
	assert.Equal(t, []string{"dev/vpc/1.tar", "prod/vpc/2.tar"}, names, "artifacts need both the file and its sidecar")

	prod, err := store.List(ctx, artifact.Query{Stacks: []string{"prod"}})
	require.NoError(t, err)
	require.Len(t, prod, 1)
	assert.Equal(t, int64(2), prod[0].Size)
	assert.Equal(t, "prod", prod[0].Metadata.Stack)
}
