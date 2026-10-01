package filesystem

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestSafeJoin(t *testing.T) {
	destDir := filepath.Join("root", "extract")

	tests := []struct {
		name      string
		entryName string
		wantPath  string
		wantErr   bool
	}{
		{
			name:      "plain relative file",
			entryName: "file.txt",
			wantPath:  filepath.Join(destDir, "file.txt"),
		},
		{
			name:      "nested relative file",
			entryName: "a/b/c.txt",
			wantPath:  filepath.Join(destDir, "a", "b", "c.txt"),
		},
		{
			name:      "entry equal to dest dir",
			entryName: ".",
			wantPath:  destDir,
		},
		{
			name:      "traversal component",
			entryName: "../escape.txt",
			wantErr:   true,
		},
		{
			name:      "nested traversal component",
			entryName: "a/../../escape.txt",
			wantErr:   true,
		},
		{
			name:      "traversal-prefixed sibling directory name",
			entryName: "../extractEVIL/file.txt",
			wantErr:   true,
		},
		{
			name:      "absolute path",
			entryName: "/etc/passwd",
			wantErr:   true,
		},
		{
			name:      "backslash traversal",
			entryName: "..\\escape.txt",
			wantErr:   true,
		},
		{
			name:      "name that merely starts with dotdot",
			entryName: "..hidden.txt",
			wantPath:  filepath.Join(destDir, "..hidden.txt"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SafeJoin(destDir, tt.entryName)
			if tt.wantErr {
				require.Error(t, err)
				require.True(t, errors.Is(err, errUtils.ErrArchiveEntryEscapesDest))
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantPath, got)
		})
	}
}

// TestSafeJoin_RootDestinations is a regression test: a naive
// strings.HasPrefix(target, cleanDestDir+separator) containment check
// incorrectly rejects a valid entry when destDir has no room for a trailing
// separator before the next path segment -- a relative "." (or empty string,
// which filepath.Clean also normalizes to ".") or an absolute "/" both
// collapse cleanDestDir and target's shared prefix to exactly cleanDestDir
// with nothing after it.
func TestSafeJoin_RootDestinations(t *testing.T) {
	tests := []struct {
		name    string
		destDir string
	}{
		{name: "dot destination", destDir: "."},
		{name: "empty destination", destDir: ""},
		{name: "root destination", destDir: "/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SafeJoin(tt.destDir, "file.txt")
			require.NoError(t, err)
			require.Equal(t, filepath.Join(filepath.Clean(tt.destDir), "file.txt"), got)
		})
	}
}
