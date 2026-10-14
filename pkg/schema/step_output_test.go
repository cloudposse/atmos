package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateOutputMode(t *testing.T) {
	tests := []struct {
		name    string
		mode    string
		wantErr error
	}{
		{name: "empty uses the default", mode: ""},
		{name: "raw", mode: "raw"},
		{name: "log", mode: "log"},
		{name: "viewport", mode: "viewport"},
		{name: "none", mode: "none"},
		{name: "surrounding space is ignored", mode: " raw "},
		{name: "a templated mode is checked after rendering", mode: "{{ .flags.mode }}"},
		{name: "capture is not a mode", mode: "capture", wantErr: ErrStepInvalidOutputMode},
		{name: "modes are case sensitive", mode: "Raw", wantErr: ErrStepInvalidOutputMode},
		{name: "unknown", mode: "pretty", wantErr: ErrStepInvalidOutputMode},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateOutputMode("step \"s\"", tt.mode)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Contains(t, err.Error(), "raw, log, viewport, none")
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestValidateStepOutputOnlyChecksTypesThatAcceptOutputModes(t *testing.T) {
	tests := []struct {
		name     string
		stepType string
		output   string
		wantErr  bool
	}{
		{name: "default type", stepType: "", output: "capture", wantErr: true},
		{name: "shell", stepType: TaskTypeShell, output: "capture", wantErr: true},
		{name: "script", stepType: TaskTypeScript, output: "capture", wantErr: true},
		{name: "atmos", stepType: TaskTypeAtmos, output: "capture", wantErr: true},
		{name: "container", stepType: "container", output: "capture", wantErr: true},
		{name: "shell with a valid mode", stepType: TaskTypeShell, output: "log"},
		{name: "parallel has its own modes", stepType: TaskTypeParallel, output: "grouped"},
		{name: "matrix has its own modes", stepType: TaskTypeMatrix, output: "prefixed"},
		{name: "test steps report failures or all", stepType: TaskTypeTest, output: "all"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateStepOutput(&WorkflowStep{Name: "s", Type: tt.stepType, Output: tt.output})
			if tt.wantErr {
				require.ErrorIs(t, err, ErrStepInvalidOutputMode)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestValidateWorkflowStepsRejectsUnknownOutputMode(t *testing.T) {
	t.Run("top-level step", func(t *testing.T) {
		err := ValidateWorkflowSteps([]WorkflowStep{{Name: "c", Type: TaskTypeScript, Interpreter: "starlark", Script: "x", Output: "capture"}})
		require.ErrorIs(t, err, ErrStepInvalidOutputMode)
		assert.Contains(t, err.Error(), `step "c"`)
	})

	t.Run("a child of a parallel step", func(t *testing.T) {
		err := ValidateWorkflowSteps([]WorkflowStep{{
			Name: "group", Type: TaskTypeParallel,
			Steps: []WorkflowStep{{Name: "child", Type: TaskTypeShell, Command: "echo", Output: "capture"}},
		}})
		require.ErrorIs(t, err, ErrStepInvalidOutputMode)
		assert.Contains(t, err.Error(), `step "child"`)
	})

	t.Run("valid modes pass", func(t *testing.T) {
		for _, mode := range StepOutputModes() {
			require.NoError(t, ValidateWorkflowSteps([]WorkflowStep{{Name: "s", Type: TaskTypeShell, Command: "echo", Output: mode}}), mode)
		}
	})
}
