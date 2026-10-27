package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRemoveTempDir(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func() string // Returns path to test with
		wantLog bool          // Should log a warning
	}{
		{
			name: "remove existing directory",
			setup: func() string {
				return t.TempDir()
			},
			wantLog: false,
		},
		{
			name: "remove non-existent directory - no error logged",
			setup: func() string {
				return filepath.Join(os.TempDir(), "atmos-non-existent-dir-12345")
			},
			wantLog: false, // os.RemoveAll doesn't error on non-existent paths
		},
		{
			name: "remove directory with files",
			setup: func() string {
				dir := t.TempDir()
				// Create a file inside
				file := filepath.Join(dir, "test.txt")
				err := os.WriteFile(file, []byte("test"), 0o644)
				assert.NoError(t, err)
				return dir
			},
			wantLog: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := tt.setup()

			// Call removeTempDir - it doesn't return anything
			removeTempDir(path)

			// Verify directory was removed
			_, err := os.Stat(path)
			assert.True(t, os.IsNotExist(err), "Directory should be removed")
		})
	}
}
