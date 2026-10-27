package sandbox

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCopyMergeHeadsRejectsEscapingSymlink(t *testing.T) {
	root := pendingMerge(t)
	for _, metadataDirectory := range []bool{false, true} {
		name := "merge head symlink"
		if metadataDirectory {
			name = "metadata directory symlink"
		}
		t.Run(name, func(t *testing.T) {
			dest := t.TempDir()
			outside := t.TempDir()
			protected := filepath.Join(outside, mergeHeadRef)
			mustWrite(t, protected, "unchanged\n")
			target, link := protected, filepath.Join(dest, ".git", mergeHeadRef)
			if metadataDirectory {
				target, link = outside, filepath.Join(dest, ".git")
			} else {
				require.NoError(t, os.Mkdir(filepath.Join(dest, ".git"), privateDirMode))
			}
			if err := os.Symlink(target, link); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
			require.Error(t, copyMergeHeads(t.Context(), root, dest))
			data, err := os.ReadFile(protected)
			require.NoError(t, err)
			require.Equal(t, "unchanged\n", string(data))
		})
	}
}
