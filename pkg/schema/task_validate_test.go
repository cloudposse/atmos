package schema

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateExecTasks(t *testing.T) {
	maxAttempts := 3

	tests := []struct {
		name    string
		tasks   Tasks
		wantErr error
	}{
		{
			name:    "no steps",
			tasks:   Tasks{},
			wantErr: nil,
		},
		{
			name: "no exec steps",
			tasks: Tasks{
				{Type: TaskTypeShell, Command: "echo one"},
				{Type: TaskTypeAtmos, Command: "terraform plan vpc"},
			},
			wantErr: nil,
		},
		{
			name: "exec as last step",
			tasks: Tasks{
				{Type: TaskTypeShell, Command: "echo preparing"},
				{Type: TaskTypeExec, Command: "aws ssm start-session"},
			},
			wantErr: nil,
		},
		{
			name: "exec as only step",
			tasks: Tasks{
				{Type: TaskTypeExec, Command: "aws ssm start-session"},
			},
			wantErr: nil,
		},
		{
			name: "exec not last",
			tasks: Tasks{
				{Type: TaskTypeExec, Command: "aws ssm start-session"},
				{Type: TaskTypeShell, Command: "echo never runs"},
			},
			wantErr: ErrExecStepNotLast,
		},
		{
			name: "exec with tty",
			tasks: Tasks{
				{Type: TaskTypeExec, Command: "top", Tty: true},
			},
			wantErr: ErrExecStepInvalidField,
		},
		{
			name: "exec with interactive",
			tasks: Tasks{
				{Type: TaskTypeExec, Command: "top", Interactive: true},
			},
			wantErr: ErrExecStepInvalidField,
		},
		{
			name: "exec with retry",
			tasks: Tasks{
				{Type: TaskTypeExec, Command: "top", Retry: &RetryConfig{MaxAttempts: &maxAttempts}},
			},
			wantErr: ErrExecStepInvalidField,
		},
		{
			name: "exec with timeout",
			tasks: Tasks{
				{Type: TaskTypeExec, Command: "top", Timeout: 30 * time.Second},
			},
			wantErr: ErrExecStepInvalidField,
		},
		{
			name: "exec with output",
			tasks: Tasks{
				{Type: TaskTypeExec, Command: "top", Output: "raw"},
			},
			wantErr: ErrExecStepInvalidField,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateExecTasks(tt.tasks)
			if tt.wantErr == nil {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestValidateExecTasks_ErrorNamesStep(t *testing.T) {
	err := ValidateExecTasks(Tasks{
		{Type: TaskTypeExec, Name: "session", Command: "ssh host"},
		{Type: TaskTypeShell, Command: "echo never runs"},
	})
	require.ErrorIs(t, err, ErrExecStepNotLast)
	assert.Contains(t, err.Error(), `"session"`)
}

func TestValidateExecWorkflowSteps(t *testing.T) {
	maxAttempts := 3

	tests := []struct {
		name    string
		steps   []WorkflowStep
		wantErr error
	}{
		{
			name: "exec with tty",
			steps: []WorkflowStep{
				{Type: TaskTypeExec, Command: "psql", Tty: true},
			},
			wantErr: ErrExecStepInvalidField,
		},
		{
			name: "exec with interactive",
			steps: []WorkflowStep{
				{Type: TaskTypeExec, Command: "psql", Interactive: true},
			},
			wantErr: ErrExecStepInvalidField,
		},
		{
			name: "exec with retry",
			steps: []WorkflowStep{
				{Type: TaskTypeExec, Command: "psql", Retry: &RetryConfig{MaxAttempts: &maxAttempts}},
			},
			wantErr: ErrExecStepInvalidField,
		},
		{
			name: "exec as last step",
			steps: []WorkflowStep{
				{Type: TaskTypeShell, Command: "echo preparing"},
				{Type: TaskTypeExec, Command: "psql $DATABASE_URL"},
			},
			wantErr: nil,
		},
		{
			name: "exec not last",
			steps: []WorkflowStep{
				{Type: TaskTypeExec, Command: "psql $DATABASE_URL"},
				{Type: TaskTypeShell, Command: "echo never runs"},
			},
			wantErr: ErrExecStepNotLast,
		},
		{
			name: "exec with timeout string",
			steps: []WorkflowStep{
				{Type: TaskTypeExec, Command: "psql", Timeout: "30s"},
			},
			wantErr: ErrExecStepInvalidField,
		},
		{
			name: "exec with output mode",
			steps: []WorkflowStep{
				{Type: TaskTypeExec, Command: "psql", Output: "viewport"},
			},
			wantErr: ErrExecStepInvalidField,
		},
		{
			name: "non-exec steps unaffected",
			steps: []WorkflowStep{
				{Type: TaskTypeShell, Command: "echo one", Output: "raw", Tty: true, Interactive: true},
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateExecWorkflowSteps(tt.steps)
			if tt.wantErr == nil {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}
