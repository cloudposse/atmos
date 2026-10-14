package step

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestScopedCommandOutput(t *testing.T) {
	for _, mode := range []OutputMode{OutputModeRaw, OutputModeNone} {
		t.Run(string(mode), func(t *testing.T) {
			var out, diagnostic bytes.Buffer
			writer := &OutputModeWriter{mode: mode, writers: OutputWriters{Stdout: &out, Stderr: &diagnostic}}
			stdout, stderr, err := writer.ExecuteWithIO(func(out, diagnostic io.Writer) error {
				_, _ = io.WriteString(out, "data\n")
				_, _ = io.WriteString(diagnostic, "details\n")
				return errUtils.ErrWorkflowStepFailed
			})
			require.ErrorIs(t, err, errUtils.ErrWorkflowStepFailed)
			assert.Equal(t, "data\n", stdout)
			assert.Equal(t, "details\n", stderr)
			if mode == OutputModeNone {
				assert.Empty(t, out.String())
				assert.Empty(t, diagnostic.String())
			} else {
				assert.Equal(t, "data\n", out.String())
				assert.Equal(t, "details\n", diagnostic.String())
			}
		})
	}
}

func TestScopedCommandProcess(t *testing.T) {
	executable, err := os.Executable()
	require.NoError(t, err)
	var output bytes.Buffer
	writer := &OutputModeWriter{mode: OutputModeRaw, writers: OutputWriters{Stdout: &output}}
	stdout, stderr, err := writer.Execute(exec.CommandContext(t.Context(), executable, "-test.run=^$"))
	require.NoError(t, err)
	assert.Contains(t, stdout, "PASS")
	assert.Equal(t, stdout, output.String())
	assert.Contains(t, stderr, "no tests to run")
}

func TestScopedContainerOutput(t *testing.T) {
	var out, diagnostic bytes.Buffer
	(&ContainerHandler{}).writeOutput(&schema.WorkflowStep{}, nil, "container data", "container diagnostic", OutputWriters{Stdout: &out, Stderr: &diagnostic})
	assert.Equal(t, "container data", out.String())
	assert.Equal(t, "container diagnostic", diagnostic.String())
}

func TestScopedAtmosEnvironment(t *testing.T) {
	vars := NewVariables()
	vars.SetEnv("AUTOMATION_VALUE", "parent")
	vars.OutputWriters.Stdout = io.Discard
	opts, err := (&AtmosHandler{}).prepareExecution(t.Context(), &schema.WorkflowStep{
		Command: "version", Env: map[string]string{"AUTOMATION_VALUE": "child"},
	}, vars)
	require.NoError(t, err)
	assert.Equal(t, io.Discard, opts.writers.Stdout)
	assert.Equal(t, "AUTOMATION_VALUE=child", opts.envVars[len(opts.envVars)-1])
	assert.Equal(t, "parent", vars.Env["AUTOMATION_VALUE"])
}
