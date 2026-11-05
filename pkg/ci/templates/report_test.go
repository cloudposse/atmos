package templates

import (
	"os"
	"path/filepath"
	"strings"
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

// formatted renders an error the way the CLI prints it with all whitespace removed, because the
// formatter wraps long lines. Compare against compact(want).
func formatted(err error) string {
	return compact(errUtils.Format(err, errUtils.DefaultFormatterConfig()))
}

// compact removes all whitespace from s.
func compact(s string) string {
	return strings.Join(strings.Fields(s), "")
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

	// Negative path: without the override the loader does not read custom-image.md, so it falls
	// through to the embedded filesystem, which has no container image template.
	_, err = NewLoader(reportConfig(dir)).Load("container", "image", testEmbeddedFS())
	require.ErrorIs(t, err, errUtils.ErrFileNotFound)
}

func TestLoaderLoad_ContainerOverrideMissingFileIsAnError(t *testing.T) {
	dir := t.TempDir()
	cfg := reportConfig(dir)
	cfg.CI.Templates.Container = map[string]string{"image": "absent-image.md"}

	_, err := NewLoader(cfg).Load("container", "image", ContainerDefaults())
	require.ErrorIs(t, err, errUtils.ErrCITemplateNotFound)
	assert.Contains(t, formatted(err), compact(filepath.Join(dir, "absent-image.md")))

	// Recovery must not trigger: with the key unset the embedded default is used.
	got, err := NewLoader(reportConfig(dir)).Load("container", "image", ContainerDefaults())
	require.NoError(t, err)
	assert.NotEmpty(t, got)
}

func TestLoaderLoad_NonContainerOverrideMissingFileFallsBack(t *testing.T) {
	// Native plugin overrides keep their historical behavior: a missing file falls back to the
	// embedded default.
	cfg := reportConfig(t.TempDir())
	cfg.CI.Templates.Terraform = map[string]string{"plan": "absent-plan.md"}

	got, err := NewLoader(cfg).Load("terraform", "plan", testEmbeddedFS())
	require.NoError(t, err)
	assert.Contains(t, got, "Test Plan Template")
}

func TestRenderReport_MissingKeyNamesTheKey(t *testing.T) {
	dir := t.TempDir()
	writeReportTemplate(t, dir, "typo.md", "stack {{ .Stak }}")

	got, err := RenderReport(reportConfig(dir), "typo.md", map[string]any{"Stack": "dev"})
	require.ErrorIs(t, err, errUtils.ErrTemplateEvaluation)
	assert.Empty(t, got)
	assert.Contains(t, formatted(err), "Stak")
}

func TestRenderReport_PresentKeyWithEmptyValueStillRenders(t *testing.T) {
	dir := t.TempDir()
	writeReportTemplate(t, dir, "empty.md", "[{{ .Name }}]")

	got, err := RenderReport(reportConfig(dir), "empty.md", map[string]any{"Name": ""})
	require.NoError(t, err)
	assert.Equal(t, "[]", got)
}

func TestRenderReport_NotFoundReportsPathAndBasePath(t *testing.T) {
	dir := t.TempDir()

	_, err := RenderReport(reportConfig(dir), "nope.md", nil)
	require.ErrorIs(t, err, errUtils.ErrCITemplateNotFound)
	text := formatted(err)
	assert.Contains(t, text, compact(filepath.Join(dir, "nope.md")), "the resolved path must be named")
	assert.Contains(t, text, compact("ci.templates.base_path: "+dir), "the configured base_path must be named")
}

func TestRenderReport_NotFoundWithoutBasePathSaysItIsUnset(t *testing.T) {
	_, err := RenderReport(&schema.AtmosConfiguration{}, "nope.md", nil)
	require.ErrorIs(t, err, errUtils.ErrCITemplateNotFound)
	assert.Contains(t, formatted(err), compact("ci.templates.base_path: (unset)"))
}

func TestLoaderLoad_EmptyContainerOverrideDoesNotFallBack(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "empty.md"), nil, 0o600))
	cfg := reportConfig(dir)
	cfg.CI.Templates.Container = map[string]string{"image": "empty.md"}

	got, err := NewLoader(cfg).Load("container", "image", ContainerDefaults())
	require.NoError(t, err)
	assert.Empty(t, got, "an explicitly empty override takes precedence over the embedded template")
}
