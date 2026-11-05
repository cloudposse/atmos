package engine

import (
	"bytes"
	"errors"
	stdio "io"
	"os"
	"path/filepath"
	"testing"

	cockroach "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	iolib "github.com/cloudposse/atmos/pkg/io"
	atmosui "github.com/cloudposse/atmos/pkg/ui"
)

// warnStreams is an in-memory iolib.Streams so a test can read what the UI wrote.
type warnStreams struct {
	stdout, stderr *bytes.Buffer
}

func (s *warnStreams) Input() stdio.Reader     { return &bytes.Buffer{} }
func (s *warnStreams) Output() stdio.Writer    { return s.stdout }
func (s *warnStreams) Error() stdio.Writer     { return s.stderr }
func (s *warnStreams) RawOutput() stdio.Writer { return s.stdout }
func (s *warnStreams) RawError() stdio.Writer  { return s.stderr }

// captureUIStderr routes the UI channel into the returned buffer for the test's duration.
func captureUIStderr(t *testing.T) *bytes.Buffer {
	t.Helper()
	streams := &warnStreams{stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}
	ioCtx, err := iolib.NewContext(iolib.WithStreams(streams))
	require.NoError(t, err)
	atmosui.InitFormatter(ioCtx)
	t.Cleanup(atmosui.Reset)
	return streams.stderr
}

func TestContainsTemplateActions(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		delimiters []string
		want       bool
	}{
		{name: "config action", content: "vars:\n  environment: {{ .Config.environment | quote }}\n", want: true},
		{name: "trim marker", content: "{{- .Config.name }}", want: true},
		{name: "no space", content: "{{.Config.name}}", want: true},
		{name: "matrix root", content: "{{ .matrix.region }}", want: true},
		{name: "plain content", content: "vars:\n  environment: dev\n", want: false},
		{name: "other tool template is not flagged", content: "image: {{ .Values.image }}", want: false},
		{name: "github expression is not flagged", content: "token: ${{ secrets.TOKEN }}", want: false},
		{name: "config prefix only is not a root", content: "{{ .ConfigMap.name }}", want: false},
		{name: "custom delimiters match", content: "name: [[ .Config.name ]]", delimiters: []string{"[[", "]]"}, want: true},
		{name: "custom delimiters ignore default pair", content: "name: {{ .Config.name }}", delimiters: []string{"[[", "]]"}, want: false},
		{name: "malformed delimiters fall back to default", content: "{{ .Config.x }}", delimiters: []string{"[["}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ContainsTemplateActions(tt.content, tt.delimiters))
		})
	}
}

func TestUnmarkedTemplateWarning(t *testing.T) {
	msg := UnmarkedTemplateWarning("stack.yaml", "stacks/x.yaml", "a: {{ .Config.a }}", nil)
	assert.Contains(t, msg, "`stack.yaml` (copied to `stacks/x.yaml`)")
	assert.Contains(t, msg, "contains template syntax but is not marked as a template")
	assert.Contains(t, msg, "# atmos:template")
	assert.Contains(t, msg, ".tmpl")

	// A destination equal to the source is not repeated.
	assert.NotContains(t, UnmarkedTemplateWarning("stack.yaml", "stack.yaml", "{{ .Config.a }}", nil), "copied to")

	assert.Empty(t, UnmarkedTemplateWarning("stack.yaml", "stack.yaml", "a: b", nil))
}

func TestExplainUnmarkedTemplate(t *testing.T) {
	content := "vars:\n  environment: {{ .Config.environment | quote }}\n"
	var doc map[string]any
	parseErr := yaml.Unmarshal([]byte(content), &doc)
	require.Error(t, parseErr, "the raw template must not parse as YAML")

	err := ExplainUnmarkedTemplate("stacks/x.yaml", content, nil, parseErr)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrUnprocessedTemplate)
	assert.ErrorIs(t, err, parseErr, "the original parse failure stays in the chain")

	details := cockroach.FlattenDetails(err)
	assert.Contains(t, details, "stacks/x.yaml")
	assert.Contains(t, details, "contains template syntax but is not marked as a template")
	hints := cockroach.FlattenHints(err)
	assert.Contains(t, hints, "# atmos:template")

	// Content without template actions leaves the cause untouched.
	plain := errors.New("yaml: line 3: could not find expected ':'")
	assert.Same(t, plain, ExplainUnmarkedTemplate("stacks/x.yaml", "a: [", nil, plain))
	assert.NoError(t, ExplainUnmarkedTemplate("stacks/x.yaml", content, nil, nil))
}

func TestProcessFile_CopiedFileWithTemplateActionsIsCopiedVerbatim(t *testing.T) {
	processor := NewProcessor()
	dir := t.TempDir()
	content := "environment: {{ .Config.environment }}\n"

	file := File{Path: "stack.yaml", Content: content, IsTemplate: false, Permissions: 0o644}
	require.NoError(t, processor.ProcessFile(file, dir, false, false, nil, map[string]any{"environment": "dev"}))

	got, err := os.ReadFile(filepath.Join(dir, "stack.yaml"))
	require.NoError(t, err)
	assert.Equal(t, content, string(got), "an unmarked file is still copied as written")
}

func TestHandleExistingFile_ErrorNamesRenderedPath(t *testing.T) {
	processor := NewProcessor()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "stacks"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stacks", "x.yaml"), []byte("existing"), 0o644))

	file := File{Path: "{{ .Config.stack_file }}", Content: "new", Permissions: 0o644}
	err := processor.ProcessFile(file, dir, false, false, nil, map[string]any{"stack_file": "stacks/x.yaml"})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrFileExists)

	details := cockroach.FlattenDetails(err)
	assert.Contains(t, details, "File already exists: `stacks/x.yaml`")
	assert.NotContains(t, details, ".Config.stack_file")
}

func TestRenderedDisplayPath(t *testing.T) {
	dir := t.TempDir()
	assert.Equal(t, "a/b.yaml", renderedDisplayPath(dir, filepath.Join(dir, "a", "b.yaml"), "fallback"))
	assert.Equal(t, "fallback", renderedDisplayPath(dir, "", "fallback"))
}

func TestProcessFile_WarnsOnUnmarkedTemplate(t *testing.T) {
	stderr := captureUIStderr(t)
	processor := NewProcessor()
	dir := t.TempDir()

	unmarked := File{
		Path:               "stacks/{{ .Config.env }}.yaml",
		OriginalSourcePath: "stack.yaml",
		Content:            "environment: {{ .Config.env }}\n",
		Permissions:        0o644,
	}
	require.NoError(t, processor.ProcessFile(unmarked, dir, false, false, nil, map[string]any{"env": "dev"}))

	out := stderr.String()
	assert.Contains(t, out, "stack.yaml")
	assert.Contains(t, out, "stacks/dev.yaml")
	assert.Contains(t, out, "contains template syntax but is not marked as a template")
	assert.Contains(t, out, "# atmos:template")

	// A properly marked template and a plain file produce no warning.
	stderr.Reset()
	marked := File{Path: "marked.yaml", Content: "environment: {{ .Config.env }}\n", IsTemplate: true, Permissions: 0o644}
	require.NoError(t, processor.ProcessFile(marked, dir, false, false, nil, map[string]any{"env": "dev"}))
	plain := File{Path: "plain.yaml", Content: "environment: dev\n", Permissions: 0o644}
	require.NoError(t, processor.ProcessFile(plain, dir, false, false, nil, nil))
	assert.NotContains(t, stderr.String(), "not marked as a template")
}
