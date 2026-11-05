package schema

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestValidateScriptStepSource covers the script step rules that depend on a script file: the
// interpreter is optional because it is inferred from the file, and source can replace script.
func TestValidateScriptStepSource(t *testing.T) {
	tests := []struct {
		name    string
		step    WorkflowStep
		wantErr error
		field   string
	}{
		{name: "source only", step: WorkflowStep{Source: "scripts/check.star"}},
		{name: "source with interpreter", step: WorkflowStep{Source: "scripts/check.rb", Interpreter: "ruby"}},
		{name: "included script without interpreter", step: WorkflowStep{Script: "print(1)", ScriptSource: "/p/check.star"}},
		{name: "inline script without interpreter", step: WorkflowStep{Script: "print(1)"}, wantErr: ErrScriptStepFieldRequired, field: "interpreter"},
		{name: "neither source nor script", step: WorkflowStep{Interpreter: "python3"}, wantErr: ErrScriptStepFieldRequired, field: "source"},
		{name: "command", step: WorkflowStep{Source: "scripts/check.star", Command: "ls"}, wantErr: ErrScriptStepInvalidField, field: "command"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			step := tt.step
			step.Name, step.Type = "check", TaskTypeScript

			tasks := Tasks{{Name: step.Name, Type: step.Type, Script: step.Script, Interpreter: step.Interpreter, Command: step.Command, Source: step.Source, ScriptSource: step.ScriptSource}}
			for label, err := range map[string]error{
				"workflow steps": ValidateExecWorkflowSteps([]WorkflowStep{step}),
				"command tasks":  ValidateExecTasks(tasks),
			} {
				t.Run(label, func(t *testing.T) {
					if tt.wantErr == nil {
						require.NoError(t, err)
						return
					}
					require.ErrorIs(t, err, tt.wantErr)
					require.ErrorContains(t, err, tt.field)
				})
			}
		})
	}
}
