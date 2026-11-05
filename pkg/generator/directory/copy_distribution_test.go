package directory

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeSourceFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func TestCopyWithholdsDistributionMetadata(t *testing.T) {
	tests := []struct {
		name       string
		files      map[string]string
		wantFiles  []string
		wantAbsent []string
	}{
		{
			name: "package documents only",
			files: map[string]string{
				"README.md":                    "root readme\n",
				".atmos/artifact/README.md":    "package readme\n",
				".atmos/artifact/CHANGELOG.md": "changelog\n",
			},
			wantFiles:  []string{"README.md"},
			wantAbsent: []string{".atmos"},
		},
		{
			name: "other files under .atmos are ordinary content",
			files: map[string]string{
				"README.md":                 "root readme\n",
				".atmos/scaffold.yaml":      "kind: x\n",
				".atmos/artifact/README.md": "package readme\n",
			},
			wantFiles:  []string{"README.md", filepath.Join(".atmos", "scaffold.yaml")},
			wantAbsent: []string{filepath.Join(".atmos", "artifact")},
		},
		{
			name: "similarly named directories are kept",
			files: map[string]string{
				".atmos/artifacts/x.txt":    "x\n",
				"docs/.atmos/artifact/y.md": "y\n",
			},
			wantFiles: []string{filepath.Join(".atmos", "artifacts", "x.txt"), filepath.Join("docs", ".atmos", "artifact", "y.md")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src, dst := t.TempDir(), filepath.Join(t.TempDir(), "target")
			for rel, content := range tt.files {
				writeSourceFile(t, src, rel, content)
			}

			require.NoError(t, Copy(context.Background(), src, dst, false))

			for _, kept := range tt.wantFiles {
				assert.FileExists(t, filepath.Join(dst, kept))
			}
			for _, absent := range tt.wantAbsent {
				assert.NoDirExists(t, filepath.Join(dst, absent))
			}
		})
	}
}
