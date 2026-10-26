package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWorkingCopyProgressAndCancellation(t *testing.T) {
	for _, cancelCopy := range []bool{false, true} {
		t.Run(map[bool]string{false: "complete", true: "cancel"}[cancelCopy], func(t *testing.T) {
			root := newRepo(t)
			path := filepath.Join(root, "file.txt")
			mustWrite(t, path, "indexed\n")
			git(t, root, "add", ".")
			before := git(t, root, "ls-files", "--stage")
			mustWrite(t, path, "working\n")
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			var updates []Progress
			box, err := Open(ctx, Options{Directory: root, Scope: "all", OnProgress: func(p Progress) {
				updates = append(updates, p)
				if cancelCopy && p.Phase == "Copying working files" {
					cancel()
				}
			}})
			if cancelCopy {
				require.ErrorIs(t, err, context.Canceled)
				require.Nil(t, box)
			} else {
				require.NoError(t, err)
				defer box.Close()
				data, readErr := os.ReadFile(filepath.Join(box.Path, "file.txt"))
				require.NoError(t, readErr)
				require.Equal(t, "working\n", string(data))
				require.Contains(t, updates, Progress{Phase: "Copying working files", Completed: 1, Total: 1})
			}
			require.NotEmpty(t, updates)
			require.Equal(t, "Selecting files for isolated checks", updates[0].Phase)
			for _, p := range updates {
				require.NotEqual(t, "Copying staged files", p.Phase, "working copies must not first check out index blobs")
			}
			require.Equal(t, before, git(t, root, "ls-files", "--stage"))
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, "working\n", string(data))
		})
	}
}
