package generator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

const testGeneratedFile = "providers_override.tf.json"

// TestRemoveGenerated tests the RemoveGenerated function.
func TestRemoveGenerated(t *testing.T) {
	tests := []struct {
		name string
		// setup prepares the directory the removal runs against.
		setup func(t *testing.T, dir string)
		// wantErr is the sentinel the removal must report, or nil when it must succeed.
		wantErr error
		// verify asserts the state of the directory after the removal.
		verify func(t *testing.T, dir string)
	}{
		{
			name: "removes an existing file",
			setup: func(t *testing.T, dir string) {
				require.NoError(t, os.WriteFile(filepath.Join(dir, testGeneratedFile), []byte(`{"provider":{}}`), 0o600))
			},
			verify: func(t *testing.T, dir string) {
				_, err := os.Stat(filepath.Join(dir, testGeneratedFile))
				assert.True(t, os.IsNotExist(err), "file should be removed")
			},
		},
		{
			name:  "missing file is not an error",
			setup: func(t *testing.T, dir string) {},
		},
		{
			name: "leaves sibling files alone",
			setup: func(t *testing.T, dir string) {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "backend.tf.json"), []byte(`{"terraform":{}}`), 0o600))
			},
			verify: func(t *testing.T, dir string) {
				_, err := os.Stat(filepath.Join(dir, "backend.tf.json"))
				assert.NoError(t, err, "unrelated generated files must be preserved")
			},
		},
		{
			name: "returns error when the target is a non-empty directory",
			setup: func(t *testing.T, dir string) {
				nested := filepath.Join(dir, testGeneratedFile)
				require.NoError(t, os.Mkdir(nested, 0o700))
				require.NoError(t, os.WriteFile(filepath.Join(nested, "child"), []byte("x"), 0o600))
			},
			wantErr: errUtils.ErrGeneratedNotRegular,
		},
		{
			// `os.Remove` would delete an empty directory; generation never created one, so refuse.
			name: "leaves an empty directory at the target path intact",
			setup: func(t *testing.T, dir string) {
				require.NoError(t, os.Mkdir(filepath.Join(dir, testGeneratedFile), 0o700))
			},
			wantErr: errUtils.ErrGeneratedNotRegular,
			verify: func(t *testing.T, dir string) {
				info, err := os.Stat(filepath.Join(dir, testGeneratedFile))
				require.NoError(t, err, "empty directory must be preserved")
				assert.True(t, info.IsDir())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			tt.setup(t, dir)

			err := RemoveGenerated(dir, testGeneratedFile)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}

			if tt.verify != nil {
				tt.verify(t, dir)
			}
		})
	}
}
