package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/project/config"
)

// TestProcessFile_FileDelimitersOverrideScaffoldConfig proves File.Delimiters
// wins over the scaffold config's spec.delimiters for content and path, and
// that an invalid pair falls back to the config's.
func TestProcessFile_FileDelimitersOverrideScaffoldConfig(t *testing.T) {
	specLevel := &config.ScaffoldConfig{Spec: config.ScaffoldSpec{Delimiters: []string{"<<", ">>"}}}
	values := map[string]interface{}{"name": "demo"}
	const content = "a: [[ .Config.name ]]\nb: << .Config.name >>\nc: {{ .Config.name }}\n"
	const squareRendered = "a: demo\nb: << .Config.name >>\nc: {{ .Config.name }}\n"
	const angleRendered = "a: [[ .Config.name ]]\nb: demo\nc: {{ .Config.name }}\n"

	tests := []struct {
		name           string
		scaffoldConfig interface{}
		fileDelims     []string
		path           string
		wantContent    string
	}{
		{name: "file pair beats spec pair", scaffoldConfig: specLevel, fileDelims: []string{"[[", "]]"}, path: "[[ .Config.name ]].txt", wantContent: squareRendered},
		{name: "file pair beats absent config", scaffoldConfig: nil, fileDelims: []string{"[[", "]]"}, path: "[[ .Config.name ]].txt", wantContent: squareRendered},
		{name: "one-element file pair falls back to the spec pair", scaffoldConfig: specLevel, fileDelims: []string{"[["}, path: "<< .Config.name >>.txt", wantContent: angleRendered},
		{name: "empty-string file pair falls back to the spec pair", scaffoldConfig: specLevel, fileDelims: []string{"", ""}, path: "<< .Config.name >>.txt", wantContent: angleRendered},
		{name: "no file pair uses the spec pair", scaffoldConfig: specLevel, path: "<< .Config.name >>.txt", wantContent: angleRendered},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			targetDir := t.TempDir()
			file := File{Path: tt.path, Content: content, IsTemplate: true, Permissions: 0o644, Delimiters: tt.fileDelims}

			err := NewProcessor().ProcessFile(file, targetDir, false, false, tt.scaffoldConfig, values)
			require.NoError(t, err)

			got, err := os.ReadFile(filepath.Join(targetDir, "demo.txt"))
			require.NoError(t, err)
			assert.Equal(t, tt.wantContent, string(got))
		})
	}
}

// TestProcessFile_FileDelimitersRenderPathTemplate proves the discovered path
// is rendered with the file's own pair, and that the unrendered-marker guard
// uses that same pair (a literal "{{" survives under "[[ ]]").
func TestProcessFile_FileDelimitersRenderPathTemplate(t *testing.T) {
	targetDir := t.TempDir()
	file := File{
		Path:        "[[ .Config.name ]]-{{literal}}.txt",
		Content:     "payload ${{ github.sha }}",
		IsTemplate:  true,
		Permissions: 0o644,
		Delimiters:  []string{"[[", "]]"},
	}

	err := NewProcessor().ProcessFile(file, targetDir, false, false, nil, map[string]interface{}{"name": "demo"})
	require.NoError(t, err)

	got, err := os.ReadFile(filepath.Join(targetDir, "demo-{{literal}}.txt"))
	require.NoError(t, err)
	assert.Equal(t, "payload ${{ github.sha }}", string(got))
}
