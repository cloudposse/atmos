package create

import (
	"bytes"
	stdio "io"
	"path/filepath"
	"strings"
	"testing"

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
type splitStreams struct{ out, err *bytes.Buffer }

func (s splitStreams) Input() stdio.Reader     { return bytes.NewReader(nil) }
func (s splitStreams) Output() stdio.Writer    { return s.out }
func (s splitStreams) Error() stdio.Writer     { return s.err }
func (s splitStreams) RawOutput() stdio.Writer { return s.out }
func (s splitStreams) RawError() stdio.Writer  { return s.err }

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
