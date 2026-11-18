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
	if !fixtures.Symlink(t, dir, "link", "cache/a") {
		t.Skip("symlinks are unavailable on this platform")
	}
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

// Containment and exclusion are both decided from the links as they were captured. Each case builds
// a source, optionally changes it while the capture runs in a way the final recheck cannot see (below
// an excluded directory, which is never walked), and states the one outcome the capture may have.
func TestCapture_LinkTargetsAreJudgedFromTheCapturedLinks(t *testing.T) {
	tests := []struct {
		name    string
		files   []string
		links   [][2]string
		mutate  func(t *testing.T, dir string)
		wantErr error
		wantMsg string
		want    []string
	}{
		{
			name:    "a link to an excluded file is rejected",
			files:   []string{"keep.txt", "scratch.tmp"},
			links:   [][2]string{{"link", "scratch.tmp"}},
			wantErr: errUtils.ErrArtifactUnsafePath,
			wantMsg: `symlink "link" targets excluded path "scratch.tmp"`,
		},
		{
			name:    "a chain of two links that ends below an excluded directory is rejected",
			files:   []string{"keep.txt", "cache/a"},
			links:   [][2]string{{"first", "second"}, {"second", "cache/a"}},
			wantErr: errUtils.ErrArtifactUnsafePath,
			wantMsg: `symlink "first" targets excluded path "cache"`,
		},
		{
			name:    "a chain that climbs out of the source is rejected as an escape",
			files:   []string{"keep.txt", "sub/keep.txt"},
			links:   [][2]string{{"sub/up", ".."}, {"out", "sub/up/../gone"}},
			wantErr: errUtils.ErrArtifactUnsafePath,
			wantMsg: `symlink "out" resolves outside the source`,
		},
		{
			name:  "a link to an allowed path is captured",
			files: []string{"keep.txt", "cache/a"},
			links: [][2]string{{"link", "keep.txt"}},
			want:  []string{"keep.txt", "link"},
		},
		{
			// The filesystem looks up cache/p before it processes "..", so excluding cache turns this
			// link into a broken one even though it reduces to keep.txt. A later change below the
			// excluded directory (cache/p becoming a link to cache/q/r) cannot change the answer.
			name:  "a link that passes through an excluded directory is rejected even when it reduces to an allowed path",
			files: []string{"keep.txt", "cache/p/a", "cache/q/r/a"},
			links: [][2]string{{"link", "cache/p/../../keep.txt"}},
			mutate: func(t *testing.T, dir string) {
				require.NoError(t, os.RemoveAll(filepath.Join(dir, "cache", "p")))
				require.NoError(t, os.Symlink(filepath.Join("q", "r"), filepath.Join(dir, "cache", "p")))
			},
			wantErr: errUtils.ErrArtifactUnsafePath,
			wantMsg: `symlink "link" targets excluded path "cache"`,
		},
		{
			name:  "a link that only passes through allowed directories is captured",
			files: []string{"keep.txt", "sub/a"},
			links: [][2]string{{"link", "sub/../keep.txt"}},
			want:  []string{"keep.txt", "link", "sub", "sub/a"},
		},
		{
			name:  "removing the target below an excluded directory cannot turn a rejected link into an allowed one",
			files: []string{"keep.txt", "cache/a"},
			links: [][2]string{{"link", "cache/a"}},
			mutate: func(t *testing.T, dir string) {
				require.NoError(t, os.Remove(filepath.Join(dir, "cache", "a")))
			},
			wantErr: errUtils.ErrArtifactUnsafePath,
			wantMsg: `symlink "link" targets excluded path "cache"`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, file := range tc.files {
				fixtures.WriteFile(t, dir, file, "x", 0o644)
			}
			for _, link := range tc.links {
				if !fixtures.Symlink(t, dir, link[0], link[1]) {
					t.Skip("symlinks are unavailable on this platform")
				}
			}
			mutated := false
			opts := []snapshot.Option{snapshot.WithStagingRoot(t.TempDir()), snapshot.WithExcluder(byName)}
			if tc.mutate != nil {
				// The hook runs after a regular file was copied. By then the walk has classified
				// "cache", which sorts before every captured file in these sources.
				opts = append(opts, snapshot.WithCaptureHook(func(string) {
					if !mutated {
						mutated = true
						tc.mutate(t, dir)
					}
				}))
			}

			snap, err := snapshot.Capture(context.Background(), dir, opts...)

			require.Equal(t, tc.mutate != nil, mutated, "the source is changed exactly when the case asks for it")
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				assert.Contains(t, err.Error(), tc.wantMsg)
				return
			}
			require.NoError(t, err)
			t.Cleanup(func() { _ = snap.Close() })
			require.NotEmpty(t, snap.Entries)
			assert.Equal(t, tc.want, entryPaths(snap.Entries))
		})
	}
}
