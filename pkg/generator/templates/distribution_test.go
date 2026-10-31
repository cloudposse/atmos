package templates

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func filePaths(files []File) []string {
	paths := make([]string, 0, len(files))
	for index := range files {
		paths = append(paths, files[index].Path)
	}
	return paths
}

func TestWithoutDistributionMetadata(t *testing.T) {
	dir := func(path string) File { return File{Path: path, IsDirectory: true} }
	file := func(path string) File { return File{Path: path} }
	tests := []struct {
		name  string
		files []File
		want  []string
	}{
		{
			name:  "package documents and their parent disappear",
			files: []File{file("README.md"), dir(".atmos"), dir(".atmos/artifact"), file(".atmos/artifact/README.md"), file(".atmos/artifact/CHANGELOG.md"), file("scaffold.yaml")},
			want:  []string{"README.md", "scaffold.yaml"},
		},
		{
			name:  "the parent stays when something else lives below it",
			files: []File{dir(".atmos"), dir(".atmos/artifact"), file(".atmos/artifact/README.md"), file(".atmos/scaffold.yaml")},
			want:  []string{".atmos", ".atmos/scaffold.yaml"},
		},
		{
			name:  "the root README is ordinary content",
			files: []File{file("README.md"), file("docs/README.md")},
			want:  []string{"README.md", "docs/README.md"},
		},
		{
			name:  "lookalike paths are kept",
			files: []File{file(".atmos/artifacts/x"), file("docs/.atmos/artifact/y")},
			want:  []string{".atmos/artifacts/x", "docs/.atmos/artifact/y"},
		},
		{name: "no files", files: nil, want: []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, filePaths(withoutDistributionMetadata(tt.files)))
		})
	}
}

func TestLoadConfigurationFromDir_NeverLoadsDistributionMetadata(t *testing.T) {
	dir := t.TempDir()
	writeTemplateFile(t, dir, "scaffold.yaml", "apiVersion: atmos/v1\nkind: AtmosScaffoldConfig\nmetadata:\n  name: demo\n")
	writeTemplateFile(t, dir, "README.md", "# root readme\n")
	writeTemplateFile(t, dir, ".atmos/artifact/README.md", "# package readme\n")
	writeTemplateFile(t, dir, ".atmos/artifact/CHANGELOG.md", "# changelog\n")

	configuration, err := LoadConfigurationFromDir("demo", dir)

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"README.md", "scaffold.yaml"}, filePaths(configuration.Files))
	assert.Equal(t, "# root readme\n", configuration.README, "the root README keeps its behavior")
}

func writeTemplateFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}
