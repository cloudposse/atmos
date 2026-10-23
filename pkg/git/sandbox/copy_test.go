package sandbox

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParallelCopyPreservesFilesAndReportsProgress(t *testing.T) {
	root, dest := t.TempDir(), t.TempDir()
	entries := copyFixture(t, root, 305)
	stamp := time.Unix(1700000000, 0)
	path := filepath.Join(root, "dir-0", "file-0")
	require.NoError(t, os.Chmod(path, 0o751))
	require.NoError(t, os.Chtimes(path, stamp, stamp))
	require.NoError(t, os.WriteFile(filepath.Join(root, "binary"), []byte{0, 255, 1}, 0o644))
	require.NoError(t, os.Symlink("dir-0/file-0", filepath.Join(root, "symlink")))
	entries = append(entries, "100644 blob 0\tbinary", "120000 blob 0\tsymlink", "100644 blob 0\tdeleted", "160000 blob 0\tsubmodule")
	var progress []Progress
	ctx := context.WithValue(t.Context(), servicesKey{}, gitServices{progress: func(p Progress) {
		progress = append(progress, p)
	}})
	require.NoError(t, overlayFiles(ctx, root, dest, "all", entries))
	for _, entry := range entries[:len(entries)-1] {
		_, name, _ := strings.Cut(entry, "\t")
		src, err := readState(filepath.Join(root, name))
		require.NoError(t, err)
		dst, err := readState(filepath.Join(dest, name))
		require.NoError(t, err)
		require.True(t, equalState(src, dst), name)
	}
	info, err := os.Stat(filepath.Join(dest, "dir-0", "file-0"))
	require.NoError(t, err)
	require.Equal(t, stamp, info.ModTime())
	var completed []int
	for _, p := range progress {
		require.Equal(t, len(entries), p.Total)
		completed = append(completed, p.Completed)
	}
	require.Equal(t, []int{0, 100, 200, 300, len(entries)}, completed)
}

func TestParallelCopyStopsOnCancellationAndError(t *testing.T) {
	for _, failCopy := range []bool{false, true} {
		t.Run(strconv.FormatBool(failCopy), func(t *testing.T) {
			root, dest := t.TempDir(), t.TempDir()
			entries := copyFixture(t, root, 600)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if failCopy {
				entries[0] = "100644 blob 0\t../escape"
			}
			last := 0
			ctx = context.WithValue(ctx, servicesKey{}, gitServices{progress: func(p Progress) {
				last = p.Completed
				if !failCopy && last >= 100 {
					cancel()
				}
			}})
			err := overlayFiles(ctx, root, dest, "all", entries)
			if failCopy {
				require.ErrorIs(t, err, ErrUnsafePath)
			} else {
				require.ErrorIs(t, err, context.Canceled)
			}
			// Reading immediately after return also detects workers left running under -race.
			require.Less(t, last, len(entries), "incomplete copies must not report completion")
		})
	}
}

func copyFixture(t testing.TB, root string, count int) []string {
	t.Helper()
	entries := make([]string, 0, count)
	for i := range count {
		name := fmt.Sprintf("dir-%d/file-%d", i%20, i)
		path := filepath.Join(root, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(strings.Repeat("contents\n", 128)), 0o644))
		entries = append(entries, "100644 blob 0\t"+name)
	}
	return entries
}

func BenchmarkOverlayFiles(b *testing.B) {
	root := b.TempDir()
	entries := copyFixture(b, root, 2000)
	for _, workers := range []int{1, 4, 8, 16} {
		b.Run(fmt.Sprintf("workers-%d", workers), func(b *testing.B) {
			for b.Loop() {
				b.StopTimer()
				dest := b.TempDir()
				b.StartTimer()
				copier := checkoutCopier{root: root, dest: dest, scope: "all"}
				err := copier.copyFiles(b.Context(), entries, workers)
				b.StopTimer()
				require.NoError(b, err)
				require.NoError(b, os.RemoveAll(dest))
				b.StartTimer()
			}
		})
	}
}
