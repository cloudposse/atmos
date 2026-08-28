package archiveutil

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
