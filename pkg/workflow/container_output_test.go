package workflow

import (
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// captureStreams runs fn with the process stdout and stderr redirected and returns what was
// written to each. The I/O layer is re-initialized on both sides so later tests keep working.
func captureStreams(t *testing.T, fn func()) (stdout, stderr string) {
	t.Helper()

	oldStdout, oldStderr := os.Stdout, os.Stderr
	outR, outW, err := os.Pipe()
	require.NoError(t, err)
	errR, errW, err := os.Pipe()
	require.NoError(t, err)
	outDone, errDone := make(chan string, 1), make(chan string, 1)
	drain := func(r *os.File, done chan<- string) {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}
	go drain(outR, outDone)
	go drain(errR, errDone)

	initIO := func() {
		iolib.Reset()
		ui.Reset()
		data.Reset()
		require.NoError(t, iolib.Initialize())
		ioCtx := iolib.GetContext()
		ui.InitFormatter(ioCtx)
		data.InitWriter(ioCtx)
	}
	os.Stdout, os.Stderr = outW, errW
	initIO()
	fn()
	_ = outW.Close()
	_ = errW.Close()
	stdout, stderr = <-outDone, <-errDone
	os.Stdout, os.Stderr = oldStdout, oldStderr
	initIO()
	return stdout, stderr
}

// TestContainerStepOutputMatchesOtherCommandSteps checks that a container step renders output the
// way shell, script, and atmos command steps do: raw, with no step label unless the step or the
// workflow asks for one.
func TestContainerStepOutputMatchesOtherCommandSteps(t *testing.T) {
	result := &container.EphemeralResult{Stdout: "container-out\n", Stderr: "container-err\n"}
	labelsOn := true

	tests := []struct {
		name       string
		step       schema.WorkflowStep
		workflow   schema.WorkflowDefinition
		wantLabels bool
	}{
		{name: "default is raw without labels", step: schema.WorkflowStep{Name: "build"}},
		{name: "log mode alone adds no labels", step: schema.WorkflowStep{Name: "build", Output: "log"}},
		{name: "show.labels on the step adds labels", step: schema.WorkflowStep{Name: "build", Output: "log", Show: &schema.ShowConfig{Labels: &labelsOn}}, wantLabels: true},
		{name: "show.labels on the workflow adds labels", step: schema.WorkflowStep{Name: "build"}, workflow: schema.WorkflowDefinition{Show: &schema.ShowConfig{Labels: &labelsOn}}, wantLabels: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := &ContainerStepParams{Step: &tt.step, WorkflowDef: &tt.workflow}

			stdout, stderr := captureStreams(t, func() { writeEphemeralResult(params, result, nil) })

			assert.Contains(t, stdout, "container-out")
			assert.Contains(t, stderr, "container-err")
			if tt.wantLabels {
				assert.Contains(t, stderr, "[build]")
			} else {
				assert.NotContains(t, stderr, "[build]")
				assert.NotContains(t, stdout, "[build]")
			}
		})
	}
}
