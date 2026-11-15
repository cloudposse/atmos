package materialize

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stagedAndDestination lays out a staged tree and an existing destination that hold different content.
func stagedAndDestination(t *testing.T) (stage, dest string) {
	t.Helper()
	root := t.TempDir()
	stage = filepath.Join(root, "scratch", "tree")
	dest = filepath.Join(root, "dest")
	require.NoError(t, os.MkdirAll(stage, 0o700))
	require.NoError(t, os.Mkdir(dest, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(stage, "new"), []byte("new"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dest, "original"), []byte("original"), 0o600))
	return stage, dest
}

func TestInstallStaged_CancellationDuringTheReplaceGuardIsFinal(t *testing.T) {
	tests := []struct {
		name          string
		cancelInGuard bool
	}{
		{"canceled while the guard runs", true},
		{"not canceled", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if !tc.cancelInGuard && runtime.GOOS == "windows" {
				t.Skipf("atomic exchange of an existing destination is unavailable on %s", runtime.GOOS)
			}
			stage, dest := stagedAndDestination(t)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			guard := func(string) error {
				if tc.cancelInGuard {
					cancel()
				}
				return nil
			}

			_, err := installStaged(ctx, stage, dest, guard)

			if !tc.cancelInGuard {
				require.NoError(t, err)
				data, readErr := os.ReadFile(filepath.Join(dest, "new"))
				require.NoError(t, readErr)
				assert.Equal(t, "new", string(data))
				return
			}
			require.ErrorIs(t, err, context.Canceled)
			data, readErr := os.ReadFile(filepath.Join(dest, "original"))
			require.NoError(t, readErr)
			assert.Equal(t, "original", string(data), "the previous content stays in place")
			assert.NoFileExists(t, filepath.Join(dest, "new"))
			assert.FileExists(t, filepath.Join(stage, "new"), "the staged tree is not installed")
		})
	}
}
