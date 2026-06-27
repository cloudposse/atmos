package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateBackgroundSteps(t *testing.T) {
	tests := []struct {
		name    string
		steps   []WorkflowStep
		wantErr string
	}{
		{
			name: "valid background container + wait + cancel",
			steps: []WorkflowStep{
				{Name: "emulator", Type: "container", BackgroundAsync: true, Run: &ContainerRunStep{Image: "floci"}},
				{Name: "apply", Type: "atmos", Command: "terraform apply vpc"},
				{Name: "gate", Type: TaskTypeWait, For: []string{"emulator"}},
				{Name: "teardown", Type: TaskTypeCancel, For: []string{"emulator"}},
			},
		},
		{
			name: "wait-all needs no targets",
			steps: []WorkflowStep{
				{Name: "emulator", Type: "container", BackgroundAsync: true, Run: &ContainerRunStep{Image: "floci"}},
				{Name: "gate", Type: TaskTypeWaitAll},
			},
		},
		{
			name: "background on a shell step is rejected in v1",
			steps: []WorkflowStep{
				{Name: "server", Type: "shell", BackgroundAsync: true, Command: "npm run dev"},
			},
			wantErr: "supports background only for `type: container`",
		},
		{
			name: "background container cannot be interactive",
			steps: []WorkflowStep{
				{Name: "emulator", Type: "container", BackgroundAsync: true, Interactive: true, Run: &ContainerRunStep{Image: "floci"}},
			},
			wantErr: "cannot set tty or interactive",
		},
		{
			name: "wait targeting an unknown step is rejected",
			steps: []WorkflowStep{
				{Name: "gate", Type: TaskTypeWait, For: []string{"nope"}},
			},
			wantErr: "references unknown or already-stopped background step",
		},
		{
			name: "wait without for is rejected",
			steps: []WorkflowStep{
				{Name: "emulator", Type: "container", BackgroundAsync: true, Run: &ContainerRunStep{Image: "floci"}},
				{Name: "gate", Type: TaskTypeWait},
			},
			wantErr: "requires `for:`",
		},
		{
			name: "double cancel is rejected (target retired after first cancel)",
			steps: []WorkflowStep{
				{Name: "emulator", Type: "container", BackgroundAsync: true, Run: &ContainerRunStep{Image: "floci"}},
				{Name: "t1", Type: TaskTypeCancel, For: []string{"emulator"}},
				{Name: "t2", Type: TaskTypeCancel, For: []string{"emulator"}},
			},
			wantErr: "already-stopped",
		},
		{
			name: "wait may not reference a step declared later",
			steps: []WorkflowStep{
				{Name: "gate", Type: TaskTypeWait, For: []string{"emulator"}},
				{Name: "emulator", Type: "container", BackgroundAsync: true, Run: &ContainerRunStep{Image: "floci"}},
			},
			wantErr: "references unknown",
		},
	}

	// Compile-time sentinel: validation keys off these struct fields.
	_ = WorkflowStep{BackgroundAsync: true, For: []string{"x"}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBackgroundSteps(tt.steps)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrWorkflowControlStepInvalid)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
