package create

import (
	"bytes"
	"errors"
	stdio "io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/condition"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/generator/engine"
	generatorui "github.com/cloudposse/atmos/pkg/generator/ui"
	"github.com/cloudposse/atmos/pkg/hooks"
	ioLayer "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
)

// splitStreams keeps stdout (data) and stderr (UI) apart so a test can assert which channel carried what.
// The splitStreams type captures stdout and stderr separately. Captured text is
// stored without ANSI escapes because CI runners can advertise color support.
type splitStreams struct{ out, err *bytes.Buffer }

// plainWriter strips ANSI escape sequences before writing to the buffer.
type plainWriter struct{ buffer *bytes.Buffer }

func (w plainWriter) Write(p []byte) (int, error) {
	if _, err := w.buffer.WriteString(ansi.Strip(string(p))); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (s splitStreams) Input() stdio.Reader     { return bytes.NewReader(nil) }
func (s splitStreams) Output() stdio.Writer    { return plainWriter{s.out} }
func (s splitStreams) Error() stdio.Writer     { return plainWriter{s.err} }
func (s splitStreams) RawOutput() stdio.Writer { return plainWriter{s.out} }
func (s splitStreams) RawError() stdio.Writer  { return plainWriter{s.err} }

func captureChannels(t *testing.T) (stdout, stderr *bytes.Buffer) {
	t.Helper()
	stdout, stderr = &bytes.Buffer{}, &bytes.Buffer{}
	ctx, err := ioLayer.NewContext(ioLayer.WithStreams(splitStreams{out: stdout, err: stderr}))
	require.NoError(t, err)
	data.InitWriter(ctx)
	ui.InitFormatter(ctx)
	t.Cleanup(data.Reset)
	t.Cleanup(ui.Reset)
	return stdout, stderr
}

func previewPlan(hookMap map[string]hooks.Hook) *Plan {
	record := filepath.Join(recordDirectory, "abc.yaml")
	return &Plan{
		Manifest: filepath.Join("stacks", "dev.yaml"),
		Hooks:    hookMap,
		Generation: &generatorui.CreationPlan{Files: []engine.File{
			{Path: filepath.Join("stacks", "dev.yaml"), Content: "vars:\n  stage: dev\n"},
			{Path: filepath.Join("stacks", "extra.yaml"), Content: "vars:\n  stage: extra\n"},
			{Path: record, Content: "scaffold:\n  values:\n    secret_answer: hunter2\n"},
		}},
	}
}

func TestPreviewSeparatesFilesAndOmitsTheRecordContent(t *testing.T) {
	stdout, stderr := captureChannels(t)
	require.NoError(t, previewPlan(nil).Preview(nil))
	assert.Equal(t, "# stacks/dev.yaml\nvars:\n  stage: dev\n---\n# stacks/extra.yaml\nvars:\n  stage: extra\n", stdout.String())
	assert.NotContains(t, stdout.String(), "hunter2")
	assert.Contains(t, stderr.String(), "Would create stacks/dev.yaml")
	assert.Contains(t, stderr.String(), "Would record the generation in .atmos/stack-generations/abc.yaml")
	assert.NotContains(t, stderr.String(), "hunter2")
}

func TestPreviewOrdersHooksShowsWhenAsWrittenAndReportsIgnoredHooks(t *testing.T) {
	stdout, stderr := captureChannels(t)
	step := func(event string) hooks.Hook {
		return hooks.Hook{Kind: "step", Type: "shell", Events: []string{event}, With: map[string]any{"command": "echo " + event}}
	}
	guarded := step("after.scaffold.generate")
	guarded.When = condition.Must("answers.fail_before == true")
	skipped := step("before.scaffold.generate")
	plan := previewPlan(map[string]hooks.Hook{
		"z-before": step("before.scaffold.generate"),
		"b-after":  guarded,
		"a-after":  step("after.scaffold.generate"),
		"m-ignore": step("after.terraform.apply"),
		"skipped":  skipped,
	})
	require.NoError(t, plan.Preview(func(name string) bool { return name == "skipped" }))

	order := []string{"Would skip scaffold hook skipped", "Scaffold hook z-before would", "Scaffold hook a-after would", "Scaffold hook b-after would", "Scaffold hook m-ignore ignored: not a scaffold event"}
	last := -1
	for _, line := range order {
		index := strings.Index(stderr.String(), line)
		require.GreaterOrEqual(t, index, 0, "missing %q in %q", line, stderr.String())
		assert.Greater(t, index, last, "%q is out of order", line)
		last = index
	}
	assert.Contains(t, stdout.String(), "when: answers.fail_before == true")
	assert.NotContains(t, stdout.String()+stderr.String(), "!cel")
	assert.NotContains(t, stdout.String(), "echo after.terraform.apply")
}

func TestOutputsClassifyEveryFile(t *testing.T) {
	assert.Equal(t, []Output{
		{Path: "stacks/dev.yaml", Kind: OutputManifest},
		{Path: "stacks/extra.yaml", Kind: OutputFile},
		{Path: ".atmos/stack-generations/abc.yaml", Kind: OutputRecord},
	}, previewPlan(nil).Outputs())
}

// failingStdout builds streams whose stdout accepts a number of writes and then fails.
func failingStdout(t *testing.T, okWrites int, failure error) {
	t.Helper()
	writer := &failAfterWriter{remaining: okWrites, err: failure}
	failing, err := ioLayer.NewContext(ioLayer.WithStreams(failingStreams{out: writer, err: &bytes.Buffer{}}))
	require.NoError(t, err)
	data.InitWriter(failing)
	ui.InitFormatter(failing)
	t.Cleanup(data.Reset)
	t.Cleanup(ui.Reset)
}

type failingStreams struct{ out, err stdio.Writer }

func (s failingStreams) Input() stdio.Reader     { return bytes.NewReader(nil) }
func (s failingStreams) Output() stdio.Writer    { return s.out }
func (s failingStreams) Error() stdio.Writer     { return s.err }
func (s failingStreams) RawOutput() stdio.Writer { return s.out }
func (s failingStreams) RawError() stdio.Writer  { return s.err }

// failAfterWriter accepts a number of writes, then fails with err.
type failAfterWriter struct {
	remaining int
	err       error
}

func (w *failAfterWriter) Write(p []byte) (int, error) {
	if w.remaining <= 0 {
		return 0, w.err
	}
	w.remaining--
	return len(p), nil
}

func TestPreviewStopsAtTheFirstOutputFailure(t *testing.T) {
	broken := errors.New("stdout closed")
	// Each file takes two stdout writes (path comment, content) plus a separator before every file but the first.
	for _, tc := range []struct {
		name     string
		okWrites int
	}{
		{"first path comment", 0},
		{"first file content", 1},
		{"separator before the second file", 2},
		{"second path comment", 3},
		{"second file content", 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			failingStdout(t, tc.okWrites, broken)
			require.ErrorIs(t, previewPlan(nil).Preview(nil), broken)
		})
	}
	t.Run("every write succeeds", func(t *testing.T) {
		failingStdout(t, 5, broken)
		require.NoError(t, previewPlan(nil).Preview(nil))
	})
}

func TestPreviewHookStopsAtTheFirstOutputFailure(t *testing.T) {
	broken := errors.New("stdout closed")
	declared := map[string]hooks.Hook{"notify": {Kind: "step", Type: "shell", Events: []string{"after.scaffold.generate"}, With: map[string]any{"command": "echo done"}}}
	plan := &Plan{Generation: &generatorui.CreationPlan{}, Hooks: declared}
	// A hook is written as a separator, a name comment and its YAML.
	for okWrites, name := range []string{"separator", "name comment", "hook body"} {
		t.Run(name, func(t *testing.T) {
			failingStdout(t, okWrites, broken)
			require.ErrorIs(t, plan.Preview(nil), broken)
		})
	}
}

func TestHookPreviewShowsEveryEntryOfAListCondition(t *testing.T) {
	stdout, _ := captureChannels(t)
	hook := hooks.Hook{Kind: "step", Type: "shell", Events: []string{"after.scaffold.generate"}, With: map[string]any{"command": "echo done"}}
	hook.When = condition.Must([]any{"failure", "answers.deploy == true"})
	plan := &Plan{Generation: &generatorui.CreationPlan{}, Hooks: map[string]hooks.Hook{"notify": hook}}

	require.NoError(t, plan.Preview(nil))
	assert.Contains(t, stdout.String(), "failure")
	assert.Contains(t, stdout.String(), "answers.deploy == true")
	assert.NotContains(t, stdout.String(), "!cel", "the internal expression marker is hidden at every nesting level")
}
