package snapshot_test

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/fixtures"
	"github.com/cloudposse/atmos/pkg/artifact/snapshot"
)

// byName excludes directories called "cache" and any entry whose name ends in ".tmp".
func byName(entryPath string, isDir bool) (string, bool) {
	name := path.Base(entryPath)
	switch {
	case isDir && name == "cache":
		return "cache/**", true
	case path.Ext(name) == ".tmp":
		return "*.tmp", true
	default:
		return "", false
	}
}

func TestCapture_ExcluderLeavesEntriesOutAndReportsThem(t *testing.T) {
	src := t.TempDir()
	fixtures.WriteFile(t, src, "keep.txt", "keep\n", 0o644)
	fixtures.WriteFile(t, src, "scratch.tmp", "tmp\n", 0o644)
	fixtures.WriteFile(t, src, "cache/a/b.txt", "deep\n", 0o644)
	fixtures.WriteFile(t, src, "nested/cache/c.txt", "deep\n", 0o644)
	fixtures.WriteFile(t, src, "nested/keep.txt", "keep\n", 0o644)

	snap, err := snapshot.Capture(context.Background(), src, snapshot.WithStagingRoot(t.TempDir()), snapshot.WithExcluder(byName))

	require.NoError(t, err)
	t.Cleanup(func() { _ = snap.Close() })
	assert.Equal(t, []string{"keep.txt", "nested", "nested/keep.txt"}, entryPaths(snap.Entries))
	assert.Equal(t, []artifact.Exclusion{
		{Path: "cache", Rule: "cache/**"},
		{Path: "nested/cache", Rule: "cache/**"},
		{Path: "scratch.tmp", Rule: "*.tmp"},
	}, snap.Exclusions, "an excluded directory is reported once, not once per descendant")
	_, staged := snap.StagedPath("cache/a/b.txt")
	assert.False(t, staged, "excluded content is never staged")
}

func TestCapture_WithoutExcluderKeepsEverything(t *testing.T) {
	src := t.TempDir()
	fixtures.WriteFile(t, src, "scratch.tmp", "tmp\n", 0o644)
	fixtures.WriteFile(t, src, "cache/a.txt", "a\n", 0o644)

	snap, err := snapshot.Capture(context.Background(), src, snapshot.WithStagingRoot(t.TempDir()))

	require.NoError(t, err)
	t.Cleanup(func() { _ = snap.Close() })
	assert.Equal(t, []string{"cache", "cache/a.txt", "scratch.tmp"}, entryPaths(snap.Entries))
	assert.Empty(t, snap.Exclusions)
}

func TestCapture_ExcludedFileAndLimit(t *testing.T) {
	dir := t.TempDir()
	fixtures.WriteFile(t, dir, "a.tmp", "a", 0o644)
	fixtures.WriteFile(t, dir, "b.tmp", "b", 0o644)
	snap, err := snapshot.Capture(context.Background(), filepath.Join(dir, "a.tmp"), snapshot.WithExcluder(byName))
	require.NoError(t, err)
	defer snap.Close()
	assert.Empty(t, snap.Entries)
	assert.Equal(t, []artifact.Exclusion{{Path: "a.tmp", Rule: "*.tmp"}}, snap.Exclusions)
	_, err = snapshot.Capture(context.Background(), dir, snapshot.WithExcluder(byName), snapshot.WithLimits(snapshot.Limits{MaxEntries: 1}))
	require.ErrorIs(t, err, errUtils.ErrArtifactTooLarge)
}

func TestCapture_RejectsLinkToExcludedDirectory(t *testing.T) {
	dir := t.TempDir()
	fixtures.WriteFile(t, dir, "cache/a", "a", 0o644)
	require.NoError(t, os.Symlink("cache/a", filepath.Join(dir, "link")))
	_, err := snapshot.Capture(context.Background(), dir, snapshot.WithExcluder(byName))
	require.ErrorIs(t, err, errUtils.ErrArtifactUnsafePath)
}

func TestCapture_RejectsExcludedTypeChange(t *testing.T) {
	dir := t.TempDir()
	fixtures.WriteFile(t, dir, "a.tmp", "a", 0o644)
	fixtures.WriteFile(t, dir, "z.txt", "z", 0o644)
	_, err := snapshot.Capture(context.Background(), dir, snapshot.WithExcluder(byName), snapshot.WithCaptureHook(func(string) {
		require.NoError(t, os.Remove(filepath.Join(dir, "a.tmp")))
		require.NoError(t, os.Mkdir(filepath.Join(dir, "a.tmp"), 0o755))
	}))
	require.ErrorIs(t, err, errUtils.ErrArtifactSourceChanged)
}
