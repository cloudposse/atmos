package ui

import (
	"bytes"
	stdio "io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/generator/templates"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/terminal"
	atmosui "github.com/cloudposse/atmos/pkg/ui"
)

// planStreams is an in-memory iolib.Streams so a test can read what the UI wrote.
type planStreams struct {
	stdout, stderr *bytes.Buffer
}

func (s *planStreams) Input() stdio.Reader     { return &bytes.Buffer{} }
func (s *planStreams) Output() stdio.Writer    { return s.stdout }
func (s *planStreams) Error() stdio.Writer     { return s.stderr }
func (s *planStreams) RawOutput() stdio.Writer { return s.stdout }
func (s *planStreams) RawError() stdio.Writer  { return s.stderr }

// newCapturingUI returns an InitUI whose UI-channel output lands in the returned buffer.
func newCapturingUI(t *testing.T) (*InitUI, *bytes.Buffer) {
	t.Helper()
	streams := &planStreams{stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}
	ioCtx, err := iolib.NewContext(iolib.WithStreams(streams))
	require.NoError(t, err)
	atmosui.InitFormatter(ioCtx)
	t.Cleanup(atmosui.Reset)
	term := terminal.New(terminal.WithIO(iolib.NewTerminalWriter(ioCtx)))
	return NewInitUI(ioCtx, term), streams.stderr
}

func singleFileTemplate() *templates.Configuration {
	return &templates.Configuration{
		Name:   "one",
		README: "# Readme heading\n",
		Files: []templates.File{
			{Path: "scaffold.yaml", Content: `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: one
spec:
  fields: []
`},
			{Path: "stack.yaml", Content: "name: one\n", Permissions: 0o644},
		},
	}
}

func TestPlanCreation_PrintsDryRunReportByDefault(t *testing.T) {
	ui, stderr := newCapturingUI(t)
	plan, err := ui.PlanCreation(singleFileTemplate(), t.TempDir(), true, nil)
	require.NoError(t, err)
	require.Len(t, plan.Files, 1)

	assert.Contains(t, stderr.String(), "(would create)")
	assert.Contains(t, stderr.String(), "Would generate 1 file.")
	assert.NotContains(t, stderr.String(), "1 files")
}

func TestPlanCreationQuiet_SuppressesDryRunReport(t *testing.T) {
	ui, stderr := newCapturingUI(t)
	root := t.TempDir()
	plan, err := ui.PlanCreationQuiet(singleFileTemplate(), root, true, nil)
	require.NoError(t, err)

	// The plan is identical to the loud one; only the report is gone.
	require.Len(t, plan.Files, 1)
	assert.Equal(t, "stack.yaml", filepath.ToSlash(plan.Files[0].Path))
	assert.Equal(t, "name: one\n", plan.Files[0].Content)
	assert.NoFileExists(t, filepath.Join(root, "stack.yaml"))

	out := stderr.String()
	assert.NotContains(t, out, "would create")
	assert.NotContains(t, out, "Would generate")
	assert.NotContains(t, out, "Readme heading")

	// Quiet mode does not leak into later runs on the same UI.
	assert.False(t, ui.quietPlan)
	stderr.Reset()
	_, err = ui.PlanCreation(singleFileTemplate(), t.TempDir(), true, nil)
	require.NoError(t, err)
	assert.Contains(t, stderr.String(), "Would generate 1 file.")
}

func TestSummaryLines_PluralizeFiles(t *testing.T) {
	assert.Equal(t, "Would generate 1 file.\n", generationSummaryLine(true, 1, 0))
	assert.Equal(t, "Would generate 0 files.\n", generationSummaryLine(true, 0, 0))
	assert.Equal(t, "Generated 1 file.\n", generationSummaryLine(false, 1, 0))
	assert.Equal(t, "Generated 1 file. Failed to generate 1 file.\n", generationSummaryLine(false, 1, 1))
	assert.Equal(t, "Generated 2 files. Failed to generate 3 files.\n", generationSummaryLine(false, 2, 3))
	assert.Equal(t, "Would initialize 1 file.\n", initializationSummaryLine(true, 1, 0))
	assert.Equal(t, "Initialized 4 files.\n", initializationSummaryLine(false, 4, 0))
}

func TestCreationPlanUsesMatricesWithoutWritingOrProjectHistory(t *testing.T) {
	ui := createTestUI(t)
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".atmos"), 0o755))
	// A malformed unrelated record must not affect a new generation.
	require.NoError(t, os.WriteFile(filepath.Join(root, ".atmos", "scaffold.yaml"), []byte("unrelated"), 0o644))
	template := &templates.Configuration{Name: "matrix", Files: []templates.File{
		{Path: "scaffold.yaml", Content: `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: matrix
spec:
  fields:
    - name: envs
      type: multiselect
      options: [dev, prod]
      default: [dev, prod]
  files:
    - path: stack.yaml
      target: 'stacks/{{ .matrix.env }}.yaml'
      matrix:
        env: answers.envs
`},
		{Path: "stack.yaml", Content: "name: {{ .matrix.env }}\n", IsTemplate: true, Permissions: 0o644},
	}}
	plan, err := ui.PlanCreation(template, root, true, nil)
	require.NoError(t, err)
	require.Len(t, plan.Files, 2)
	assert.Equal(t, "stacks/dev.yaml", filepath.ToSlash(plan.Files[0].Path))
	assert.Equal(t, "name: dev\n", plan.Files[0].Content)
	assert.Equal(t, "stacks/prod.yaml", filepath.ToSlash(plan.Files[1].Path))
	assert.Equal(t, "name: prod\n", plan.Files[1].Content)
	assert.NoDirExists(t, filepath.Join(root, "stacks"))
	assert.Equal(t, []any{"dev", "prod"}, plan.Values["envs"])
	assert.False(t, ui.processor.DryRun)
	assert.Nil(t, ui.processor.PreviewFile)
}
