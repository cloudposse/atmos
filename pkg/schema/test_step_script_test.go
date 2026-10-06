package schema

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTestStepValidatesScriptChildren(t *testing.T) {
	for _, scope := range []string{"direct", TaskTypeParallel, TaskTypeMatrix} {
		t.Run(scope, func(t *testing.T) {
			for _, tc := range []struct {
				name        string
				interpreter string
				script      string
				command     string
				err         error
				field       string
			}{
				{name: "valid", interpreter: "python3", script: "print('ok')"},
				{name: "missing interpreter", script: "print('ok')", err: ErrScriptStepFieldRequired, field: "interpreter"},
				{name: "missing script", interpreter: "python3", err: ErrScriptStepFieldRequired, field: "script"},
				{name: "conflicting command", interpreter: "python3", script: "print('ok')", command: "print('bad')", err: ErrScriptStepInvalidField, field: "command"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					children := []WorkflowStep{{Name: "check", Type: TaskTypeScript, Interpreter: tc.interpreter, Script: tc.script, Command: tc.command}}
					if scope != "direct" {
						group := WorkflowStep{Name: "group", Type: scope, Steps: children}
						if scope == TaskTypeMatrix {
							group.Matrix = map[string][]string{"region": {"east", "west"}}
						}
						children = []WorkflowStep{group}
					}
					err := ValidateWorkflowSteps([]WorkflowStep{{Type: TaskTypeTest, Steps: children}})
					if tc.err == nil {
						require.NoError(t, err)
						return
					}
					require.ErrorIs(t, err, tc.err)
					require.ErrorContains(t, err, tc.field)
				})
			}
		})
	}
}
