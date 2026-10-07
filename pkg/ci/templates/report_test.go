package templates

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// reportConfig returns a configuration whose templates base path is dir.
func reportConfig(dir string) *schema.AtmosConfiguration {
	cfg := &schema.AtmosConfiguration{}
	cfg.CI.Templates.BasePath = dir
	return cfg
}

func writeReportTemplate(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
}

func TestRenderReport(t *testing.T) {
	dir := t.TempDir()
	writeReportTemplate(t, dir, "plan.md", "## {{ replace .Name \"-\" \"_\" }}\n\n{{ .Counts.Add }} to add\n")
	writeReportTemplate(t, dir, "plain.txt", "n={{ .N }}")
	writeReportTemplate(t, dir, "bad-syntax.md", "{{ .Name ")
	writeReportTemplate(t, dir, "bad-exec.md", "{{ .Missing.Field }}")

	absolute := filepath.Join(dir, "plan.md")

	tests := []struct {
		name     string
		template string
		data     any
		want     string
		wantErr  error
	}{
		{
			name:     "function and nested key",
			template: "plan.md",
			data:     map[string]any{"Name": "my-stack", "Counts": map[string]any{"Add": 3}},
			want:     "## my_stack\n\n3 to add\n",
		},
		{
			name:     "extension added when the exact name is missing",
			template: "plan",
			data:     map[string]any{"Name": "a-b", "Counts": map[string]any{"Add": 1}},
			want:     "## a_b\n\n1 to add\n",
		},
		{
			name:     "exact name wins over extension",
			template: "plain.txt",
			data:     map[string]any{"N": 7},
			want:     "n=7",
		},
		{
			name:     "absolute path",
			template: absolute,
			data:     map[string]any{"Name": "x", "Counts": map[string]any{"Add": 0}},
			want:     "## x\n\n0 to add\n",
		},
		{
			name:     "missing file",
			template: "nope.md",
			wantErr:  errUtils.ErrCITemplateNotFound,
		},
		{
			name:     "parse error",
			template: "bad-syntax.md",
			data:     map[string]any{"Name": "x"},
			wantErr:  errUtils.ErrTemplateEvaluation,
		},
		{
			name:     "execute error",
			template: "bad-exec.md",
			data:     struct{}{},
			wantErr:  errUtils.ErrTemplateEvaluation,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RenderReport(reportConfig(dir), tt.template, tt.data)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Empty(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRenderReport_RelativeBasePathUnderAtmosBasePath(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "tpl"), 0o755))
	writeReportTemplate(t, filepath.Join(root, "tpl"), "r.md", "{{ .V }}")

	cfg := &schema.AtmosConfiguration{BasePath: root}
	cfg.CI.Templates.BasePath = "tpl"

	got, err := RenderReport(cfg, "r.md", map[string]any{"V": "ok"})
	require.NoError(t, err)
	assert.Equal(t, "ok", got)
}

func TestRenderReport_NilConfigMissingFile(t *testing.T) {
	_, err := RenderReport(nil, filepath.Join(t.TempDir(), "absent.md"), nil)
	require.ErrorIs(t, err, errUtils.ErrCITemplateNotFound)
}

func TestRenderReport_UnreadableTemplateIsNotNotFound(t *testing.T) {
	dir := t.TempDir()
	// A directory where a file is expected fails to read, but not because it is missing.
	require.NoError(t, os.Mkdir(filepath.Join(dir, "dir.md"), 0o755))

	_, err := RenderReport(reportConfig(dir), "dir.md", nil)
	require.ErrorIs(t, err, errUtils.ErrReadFile)
	assert.NotErrorIs(t, err, errUtils.ErrCITemplateNotFound)
}

func TestLoaderLoad_ContainerConfigOverride(t *testing.T) {
	dir := t.TempDir()
	writeReportTemplate(t, dir, "custom-image.md", "custom {{ .X }}")

	cfg := reportConfig(dir)
	cfg.CI.Templates.Container = map[string]string{"image": "custom-image.md"}

	got, err := NewLoader(cfg).LoadAndRender("container", "image", testEmbeddedFS(), map[string]any{"X": "y"})
	require.NoError(t, err)
	assert.Equal(t, "custom y", got)

	// Negative path: without the override the loader does not read custom-image.md.
	_, err = NewLoader(reportConfig(dir)).Load("container", "image", testEmbeddedFS())
	require.Error(t, err)
}
