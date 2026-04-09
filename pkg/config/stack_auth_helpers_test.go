package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Table-driven edge-case tests for the import-resolution and path-extraction
// helpers used by loadAuthWithImports. Split from stack_auth_loader_test.go
// to keep that file under the 600-line guideline.

func TestResolveAuthImportPaths(t *testing.T) {
	// Create a temp dir with a .yml file for the fallback test.
	tmpDir := t.TempDir()
	ymlPath := filepath.Join(tmpDir, "defaults.yml")
	require.NoError(t, os.WriteFile(ymlPath, []byte(""), 0o644))
	targetPath := filepath.Join(tmpDir, "target.yaml")
	require.NoError(t, os.WriteFile(targetPath, []byte(""), 0o644))

	tests := []struct {
		name          string
		imp           any
		importingFile string
		stacksBase    string
		wantLen       int
		wantFirst     string // expected first result (empty = nil expected).
	}{
		{
			name:          "map form with path key",
			imp:           map[string]any{"path": "target", "context": map[string]any{"k": "v"}},
			importingFile: filepath.Join(tmpDir, "importer.yaml"),
			stacksBase:    tmpDir,
			wantLen:       1,
			wantFirst:     targetPath,
		},
		{
			name:          "unknown type integer",
			imp:           42,
			importingFile: filepath.Join(tmpDir, "x.yaml"),
			stacksBase:    tmpDir,
			wantLen:       0,
		},
		{
			name:          "unknown type nil",
			imp:           nil,
			importingFile: filepath.Join(tmpDir, "x.yaml"),
			stacksBase:    tmpDir,
			wantLen:       0,
		},
		{
			name:          "unknown type slice",
			imp:           []string{"a", "b"},
			importingFile: filepath.Join(tmpDir, "x.yaml"),
			stacksBase:    tmpDir,
			wantLen:       0,
		},
		{
			name:          "empty stacksBasePath for non-relative import",
			imp:           "orgs/acme/_defaults",
			importingFile: filepath.Join(tmpDir, "importer.yaml"),
			stacksBase:    "",
			wantLen:       0,
		},
		{
			name:          "fallback to .yml extension",
			imp:           "defaults",
			importingFile: filepath.Join(tmpDir, "importer.yaml"),
			stacksBase:    tmpDir,
			wantLen:       1,
			wantFirst:     ymlPath,
		},
		{
			name:          "non-existent candidate",
			imp:           "nonexistent-import",
			importingFile: filepath.Join(tmpDir, "importer.yaml"),
			stacksBase:    tmpDir,
			wantLen:       0,
		},
		{
			name:          "glob with no matches",
			imp:           "mixins/*",
			importingFile: filepath.Join(tmpDir, "importer.yaml"),
			stacksBase:    tmpDir,
			wantLen:       0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolveAuthImportPaths(tt.imp, tt.importingFile, tt.stacksBase)
			if tt.wantLen == 0 {
				assert.Nil(t, result)
			} else {
				require.Len(t, result, tt.wantLen)
				assert.Equal(t, tt.wantFirst, result[0])
			}
		})
	}
}

func TestExtractImportPathString(t *testing.T) {
	tests := []struct {
		name string
		imp  any
		want string
	}{
		{
			name: "plain string",
			imp:  "orgs/acme/_defaults",
			want: "orgs/acme/_defaults",
		},
		{
			name: "empty string",
			imp:  "",
			want: "",
		},
		{
			name: "map[string]any with path",
			imp:  map[string]any{"path": "target"},
			want: "target",
		},
		{
			name: "map[string]any with non-string path",
			imp:  map[string]any{"path": 42},
			want: "",
		},
		{
			name: "map[string]any without path key",
			imp:  map[string]any{"context": "value"},
			want: "",
		},
		{
			name: "map[any]any with path",
			imp:  map[any]any{"path": "target"},
			want: "target",
		},
		{
			name: "map[any]any with non-string path",
			imp:  map[any]any{"path": 42},
			want: "",
		},
		{
			name: "nil",
			imp:  nil,
			want: "",
		},
		{
			name: "integer",
			imp:  42,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, extractImportPathString(tt.imp))
		})
	}
}

func TestLoadAuthWithImports_EdgeCases(t *testing.T) {
	tmpDir := t.TempDir()
	readmePath := filepath.Join(tmpDir, "readme.md")
	require.NoError(t, os.WriteFile(readmePath, []byte(""), 0o644))
	missingYAMLPath := filepath.Join(tmpDir, "nonexistent", "file.yaml")

	tests := []struct {
		name     string
		filePath string
		basePath string
		wantNil  bool
	}{
		{
			name:     "non-YAML extension",
			filePath: readmePath,
			basePath: tmpDir,
			wantNil:  true,
		},
		{
			name:     "nonexistent file",
			filePath: missingYAMLPath,
			basePath: tmpDir,
			wantNil:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := loadAuthWithImports(tt.filePath, tt.basePath, map[string]bool{})
			if tt.wantNil {
				assert.Nil(t, result)
			}
		})
	}
}
