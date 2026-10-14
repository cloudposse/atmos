package step

import (
	"context"
	"strings"
	"testing"

	cerrors "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestHandlersRejectUnknownOutputMode(t *testing.T) {
	tests := []struct {
		name string
		step schema.WorkflowStep
	}{
		{name: "shell", step: schema.WorkflowStep{Type: schema.TaskTypeShell, Command: "echo hi"}},
		{name: "script", step: schema.WorkflowStep{Type: schema.TaskTypeScript, Interpreter: "starlark", Script: "x = 1"}},
		{name: "atmos", step: schema.WorkflowStep{Type: schema.TaskTypeAtmos, Command: "version"}},
		{name: "container", step: schema.WorkflowStep{Type: "container"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, ok := Get(tt.step.Type)
			require.True(t, ok)

			valid := tt.step
			valid.Name = "ok"
			valid.Output = "log"
			require.NotErrorIs(t, handler.Validate(&valid), errUtils.ErrStepInvalidOutputMode)

			invalid := tt.step
			invalid.Name = "bad"
			invalid.Output = "capture"
			err := handler.Validate(&invalid)

			require.ErrorIs(t, err, errUtils.ErrStepInvalidOutputMode)
			hints := strings.Join(cerrors.GetAllHints(err), "\n")
			assert.Contains(t, hints, "raw, log, viewport, none")
		})
	}
}

func TestExecutorFailsBeforeRunningAStepWithAnUnknownOutputMode(t *testing.T) {
	initShellTestIO(t)

	_, err := NewStepExecutor().Execute(context.Background(), &schema.WorkflowStep{
		Name: "typo", Type: schema.TaskTypeScript, Interpreter: "starlark",
		Script: "fail(\"must not run\")", Output: "capture",
	})

	require.ErrorIs(t, err, errUtils.ErrStepInvalidOutputMode)
	assert.NotErrorIs(t, err, errUtils.ErrStarlark, "the script must not have run")
}
