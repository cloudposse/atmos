package snapshot_test

import (
	"context"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
