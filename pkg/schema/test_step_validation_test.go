package schema

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestTestStepRejectsInvalidPoliciesAndStructure(t *testing.T) {
	for _, tc := range []struct{ name, source, message string }{
		{"failure policy", "fail: {mode: invalid}\nsteps: [{type: require, files: [check]}]", "fail.mode"},
		{"sequential dependency", "steps: [{name: first, type: require, files: [check]}, {name: next, type: require, files: [check], needs: [first]}]", "needs requires"},
		{"leaf children", "steps: [{name: leaf, type: shell, steps: [{type: require, files: [check]}]}]", "cannot contain steps"},
		{"empty group", "steps: [{type: parallel, steps: []}]", "at least"},
		{"unknown dependency", "steps: [{name: first, type: require, files: [check], needs: [missing]}]", "missing"},
		{"detached child", "steps: [{type: shell, command: echo, background: true}]", "foreground"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var s WorkflowStep
			require.NoError(t, yaml.Unmarshal([]byte("type: test\n"+tc.source), &s))
			err := ValidateTestStep(&s)
			require.ErrorIs(t, err, ErrWorkflowControlStepInvalid)
			require.ErrorContains(t, err, tc.message)
		})
	}
}
