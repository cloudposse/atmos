package git

import (
	"os"
	"path/filepath"
	"testing"

	gogit "github.com/go-git/go-git/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetRootAt(t *testing.T) {
	repo, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	_, err = gogit.PlainInit(repo, false)
	require.NoError(t, err)
	nested := filepath.Join(repo, "a", "b")
	require.NoError(t, os.MkdirAll(nested, 0o755))
	outside, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	// The process working directory is neither the repository nor the directory asked about.
	t.Chdir(outside)

	tests := []struct {
		name    string
		dir     string
		want    string
		wantErr bool
	}{
		{"repository root", repo, repo, false},
		{"nested directory", nested, repo, false},
		{"directory outside a repository", outside, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetRootAt(tt.dir)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
