package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestTestStepValidation(t *testing.T) {
	for _, tc := range []struct{ name, source string }{
		{"empty", `type: test`},
		{"output", `type: test
output: raw
steps: [{type: shell, command: echo ok}]`},
		{"concurrency", `type: test
max_concurrency: 2
steps: [{type: shell, command: echo ok}]`},
		{"nested test", `type: test
steps: [{type: test, steps: [{type: shell, command: echo ok}]}]`},
		{"exec", `type: test
steps: [{type: exec, command: echo ok}]`},
		{"terminal", `type: test
steps: [{type: shell, command: echo ok, tty: true}]`},
		{"duplicate", `type: test
steps: [{name: a, type: shell, command: echo ok}, {name: a, type: shell, command: echo ok}]`},
		{"cycle", `type: test
steps:
 - type: parallel
   steps:
    - {name: a, type: shell, command: echo ok, needs: [b]}
    - {name: b, type: shell, command: echo ok, needs: [a]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var s WorkflowStep
			require.NoError(t, yaml.Unmarshal([]byte(tc.source), &s))
			assert.Error(t, ValidateWorkflowSteps([]WorkflowStep{s}))
		})
	}
}

func TestTestStepTaskRoundTrip(t *testing.T) {
	var task Task
	require.NoError(t, yaml.Unmarshal([]byte(`type: test
output: all
fail: {mode: wait_all}
steps:
 - name: regions
   type: matrix
   matrix: {region: [east, west]}
   steps: [{name: probe, type: http, url: 'https://example.com/health'}]
`), &task))
	step := task.ToWorkflowStep()
	assert.Equal(t, "test", step.Type)
	assert.Equal(t, "all", step.Output)
	require.NoError(t, ValidateWorkflowSteps([]WorkflowStep{step}))
	assert.Equal(t, []string{"east", "west"}, step.Steps[0].Matrix["region"])
}
