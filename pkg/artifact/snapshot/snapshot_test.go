package snapshot_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/fixtures"
	"github.com/cloudposse/atmos/pkg/artifact/snapshot"
)

func entryPaths(entries []artifact.Entry) []string {
	paths := make([]string, 0, len(entries))
	for _, e := range entries {
		paths = append(paths, e.Path)
	}
	return paths
}

func TestCapture_FilePackageInventory(t *testing.T) {
	src := t.TempDir()
	pkg := fixtures.WriteFilePackage(t, src)
	snap, err := snapshot.Capture(context.Background(), src, snapshot.WithStagingRoot(t.TempDir()))
	require.NoError(t, err)
	defer snap.Close()

	want := append(fixtures.FilePackagePaths(pkg.Symlinks), ".atmos", ".atmos/artifact", ".atmos/artifact/CHANGELOG.md", ".atmos/artifact/README.md")
	assert.ElementsMatch(t, want, entryPaths(snap.Entries))
	assert.IsIncreasing(t, entryPaths(snap.Entries), "inventory must be sorted by path")

	helper, ok := snap.Lookup("bin/helper.sh")
	require.True(t, ok)
	assert.Equal(t, artifact.EntryFile, helper.Type)
	if fixtures.POSIXModes() {
		assert.Equal(t, uint32(0o755), helper.Mode, "executable bit preserved")
	}
	assert.Equal(t, int64(len(fixtures.HelperScript)), helper.Size)
	assert.Equal(t, digest.FromString(fixtures.HelperScript), helper.Digest)

	empty, ok := snap.Lookup("empty.txt")
	require.True(t, ok)
	assert.Zero(t, empty.Size)
	assert.Equal(t, digest.FromString(""), empty.Digest)

	dir, ok := snap.Lookup("emptydir")
	require.True(t, ok)
	assert.Equal(t, artifact.EntryDirectory, dir.Type)

	_, ok = snap.Lookup(fixtures.UnicodeName)
	assert.True(t, ok, "Unicode filenames are preserved")

	if pkg.Symlinks {
		link, ok := snap.Lookup("link-to-file")
		require.True(t, ok)
		assert.Equal(t, artifact.EntrySymlink, link.Type)
		assert.Equal(t, "nested/deep/file.txt", link.Target)
		assert.Zero(t, link.Mode)
	}

	reader, err := snap.Open("nested/deep/file.txt")
	require.NoError(t, err)
	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.NoError(t, reader.Close())
	assert.Equal(t, "deep content\n", string(data))
}

func TestCapture_SingleFileSource(t *testing.T) {
	dir := t.TempDir()
	fixtures.WriteFile(t, dir, "tool.bin", "bytes", 0o755)
	snap, err := snapshot.Capture(context.Background(), filepath.Join(dir, "tool.bin"), snapshot.WithStagingRoot(t.TempDir()))
	require.NoError(t, err)
	defer snap.Close()
	assert.True(t, snap.SourceIsFile)
	require.Len(t, snap.Entries, 1)
	assert.Equal(t, "tool.bin", snap.Entries[0].Path)
	if fixtures.POSIXModes() {
		assert.Equal(t, uint32(0o755), snap.Entries[0].Mode)
	}
}

func TestCapture_ClearsPrivilegedBitsAndIgnoresOwnership(t *testing.T) {
	src := t.TempDir()
	fixtures.WriteFile(t, src, "a.txt", "x", 0o644)
	path := filepath.Join(src, "a.txt")
	// Setuid is not representable on every platform; failure to set it only weakens the assertion.
	_ = os.Chmod(path, 0o755|os.ModeSetuid)
	snap, err := snapshot.Capture(context.Background(), src, snapshot.WithStagingRoot(t.TempDir()))
	require.NoError(t, err)
	defer snap.Close()
	entry, _ := snap.Lookup("a.txt")
	assert.Zero(t, entry.Mode&^0o777, "privileged bits must be cleared")
}

func TestCapture_HardLinksBecomeIndependentFiles(t *testing.T) {
	src := t.TempDir()
	fixtures.WriteFile(t, src, "original.txt", "shared", 0o644)
	if err := os.Link(filepath.Join(src, "original.txt"), filepath.Join(src, "hard.txt")); err != nil {
		t.Skipf("hard links are unavailable here: %v", err)
	}
	snap, err := snapshot.Capture(context.Background(), src, snapshot.WithStagingRoot(t.TempDir()))
	require.NoError(t, err)
	defer snap.Close()
	original, _ := snap.Lookup("original.txt")
	hard, _ := snap.Lookup("hard.txt")
	assert.Equal(t, artifact.EntryFile, hard.Type)
	assert.Equal(t, original.Digest, hard.Digest)
	first, _ := snap.StagedPath("original.txt")
	second, _ := snap.StagedPath("hard.txt")
	assert.NotEqual(t, first, second, "each hard link is staged as its own file")
}

func TestCapture_InvalidSources(t *testing.T) {
	tests := []struct {
		name  string
		build func(t *testing.T, dir string) bool
		want  error
	}{
		{"absolute symlink", func(t *testing.T, dir string) bool {
			return fixtures.Symlink(t, dir, "abs", filepath.ToSlash(os.TempDir()))
		}, errUtils.ErrArtifactUnsafePath},
		{"escaping relative symlink", func(t *testing.T, dir string) bool {
			fixtures.MkdirAll(t, dir, "sub")
			return fixtures.Symlink(t, dir, "sub/escape", "../../outside")
		}, errUtils.ErrArtifactUnsafePath},
		{"traversal target", func(t *testing.T, dir string) bool {
			return fixtures.Symlink(t, dir, "up", "..")
		}, errUtils.ErrArtifactUnsafePath},
		{"chained symlink escape", func(t *testing.T, dir string) bool {
			fixtures.MkdirAll(t, dir, "a")
			return fixtures.Symlink(t, dir, "a/b", "..") && fixtures.Symlink(t, dir, "c", "a/b/../..")
		}, errUtils.ErrArtifactUnsafePath},
		{"special file", func(t *testing.T, dir string) bool {
			return fixtures.SpecialFile(t, dir, "pipe")
		}, errUtils.ErrArtifactPackagingInvalid},
		{"reserved namespace", func(t *testing.T, dir string) bool {
			fixtures.WriteFile(t, dir, artifact.ReservedNamespace+"/v1/refs/x.json", "{}", 0o644)
			return true
		}, errUtils.ErrArtifactUnsafePath},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := t.TempDir()
			if !tt.build(t, src) {
				t.Skip("fixture is unavailable on this platform")
			}
			staging := t.TempDir()
			snap, err := snapshot.Capture(context.Background(), src, snapshot.WithStagingRoot(staging))
			require.Error(t, err)
			assert.Nil(t, snap)
			assert.ErrorIs(t, err, tt.want)
			assertEmptyDir(t, staging)
		})
	}
}

func assertEmptyDir(t *testing.T, dir string) {
	t.Helper()
	items, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, items, "owned temporary data must be removed")
}

func TestCapture_RejectsMissingAndNonRegularSource(t *testing.T) {
	_, err := snapshot.Capture(context.Background(), filepath.Join(t.TempDir(), "absent"), snapshot.WithStagingRoot(t.TempDir()))
	require.ErrorIs(t, err, errUtils.ErrArtifactPackagingInvalid)
	_, err = snapshot.Capture(context.Background(), "", snapshot.WithStagingRoot(t.TempDir()))
	require.ErrorIs(t, err, errUtils.ErrArtifactPackagingInvalid)
}

func TestCapture_DetectsChangeDuringCapture(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, src string)
	}{
		{"grown", func(t *testing.T, src string) {
			require.NoError(t, os.WriteFile(filepath.Join(src, "a.txt"), []byte("original-and-more"), 0o644))
		}},
		{"same size new mtime", func(t *testing.T, src string) {
			path := filepath.Join(src, "a.txt")
			require.NoError(t, os.WriteFile(path, []byte("ORIGINAL"), 0o644))
			require.NoError(t, os.Chtimes(path, time.Now().Add(time.Hour), time.Now().Add(time.Hour)))
		}},
		{"same size same mtime different digest", func(t *testing.T, src string) {
			path := filepath.Join(src, "a.txt")
			info, err := os.Stat(path)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(path, []byte("ORIGINAL"), 0o644))
			require.NoError(t, os.Chtimes(path, info.ModTime(), info.ModTime()))
		}},
		{"file added", func(t *testing.T, src string) {
			fixtures.WriteFile(t, src, "new.txt", "new", 0o644)
		}},
		{"file removed", func(t *testing.T, src string) {
			require.NoError(t, os.Remove(filepath.Join(src, "b.txt")))
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := t.TempDir()
			fixtures.WriteFile(t, src, "a.txt", "original", 0o644)
			fixtures.WriteFile(t, src, "b.txt", "other", 0o644)
			staging := t.TempDir()
			snap, err := snapshot.Capture(context.Background(), src,
				snapshot.WithStagingRoot(staging),
				snapshot.WithCaptureHook(func(path string) {
					if path == "b.txt" {
						tt.mutate(t, src)
					}
				}))
			require.Error(t, err)
			assert.Nil(t, snap)
			assert.ErrorIs(t, err, errUtils.ErrArtifactSourceChanged)
			assertEmptyDir(t, staging)
		})
	}
}

func TestCapture_Limits(t *testing.T) {
	src := t.TempDir()
	fixtures.WriteFile(t, src, "a.txt", "0123456789", 0o644)
	fixtures.WriteFile(t, src, "b.txt", "0123456789", 0o644)

	staging := t.TempDir()
	_, err := snapshot.Capture(context.Background(), src, snapshot.WithStagingRoot(staging), snapshot.WithLimits(snapshot.Limits{MaxBytes: 15}))
	require.ErrorIs(t, err, errUtils.ErrArtifactTooLarge)
	assertEmptyDir(t, staging)

	_, err = snapshot.Capture(context.Background(), src, snapshot.WithStagingRoot(t.TempDir()), snapshot.WithLimits(snapshot.Limits{MaxEntries: 1}))
	require.ErrorIs(t, err, errUtils.ErrArtifactTooLarge)

	snap, err := snapshot.Capture(context.Background(), src, snapshot.WithStagingRoot(t.TempDir()), snapshot.WithLimits(snapshot.Limits{MaxBytes: 20, MaxEntries: 2}))
	require.NoError(t, err)
	assert.Equal(t, int64(20), snap.TotalBytes)
	require.NoError(t, snap.Close())
}

func TestCapture_CancellationRemovesStaging(t *testing.T) {
	src := t.TempDir()
	fixtures.WriteFile(t, src, "a.txt", "a", 0o644)
	fixtures.WriteFile(t, src, "b.txt", "b", 0o644)
	ctx, cancel := context.WithCancel(context.Background())
	staging := t.TempDir()
	snap, err := snapshot.Capture(ctx, src, snapshot.WithStagingRoot(staging), snapshot.WithCaptureHook(func(string) { cancel() }))
	require.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, snap)
	assertEmptyDir(t, staging)
}

func TestSnapshot_CloseRemovesStagingAndIsIdempotent(t *testing.T) {
	src := t.TempDir()
	fixtures.WriteFile(t, src, "a.txt", "a", 0o644)
	staging := t.TempDir()
	snap, err := snapshot.Capture(context.Background(), src, snapshot.WithStagingRoot(staging))
	require.NoError(t, err)
	require.NoError(t, snap.Close())
	require.NoError(t, snap.Close())
	assertEmptyDir(t, staging)
	_, ok := snap.StagedPath("a.txt")
	assert.False(t, ok)
}

func TestSnapshot_OpenUnknownPath(t *testing.T) {
	src := t.TempDir()
	fixtures.WriteFile(t, src, "a.txt", "a", 0o644)
	snap, err := snapshot.Capture(context.Background(), src, snapshot.WithStagingRoot(t.TempDir()))
	require.NoError(t, err)
	defer snap.Close()
	_, err = snap.Open("missing")
	require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
	_, ok := snap.Lookup("missing")
	assert.False(t, ok)
}
