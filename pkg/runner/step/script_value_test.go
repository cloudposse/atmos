package step

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestScriptStructuredOutputReachesLaterStepsAsJSONText pins the contract that a script step's
// dict or list `output` is exposed to later steps as its JSON text, never as Go map syntax.
func TestScriptStructuredOutputReachesLaterStepsAsJSONText(t *testing.T) {
	initShellTestIO(t)
	handler, ok := Get(schema.TaskTypeScript)
	require.True(t, ok)

	tests := []struct {
		name   string
		script string
		want   string
	}{
		{name: "dict", script: `output = {"n": 3}`, want: `{"n":3}`},
		{name: "nested dict and list", script: `output = {"items": [1, "two"], "ok": True}`, want: `{"items":[1,"two"],"ok":true}`},
		{name: "list", script: `output = [1, 2, 3]`, want: `[1,2,3]`},
		{name: "string output stays a plain string", script: `output = "abc"`, want: "abc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vars := NewVariables()
			result, err := handler.Execute(context.Background(), &schema.WorkflowStep{
				Name: "first", Type: schema.TaskTypeScript, Interpreter: "starlark", Output: "none", Script: tt.script,
			}, vars)
			require.NoError(t, err)
			require.NoError(t, vars.SetWithOutputs("first", result, nil))

			rendered, err := vars.Resolve(`{{ .steps.first.value }}`)

			require.NoError(t, err)
			assert.Equal(t, tt.want, rendered)
			assert.NotContains(t, rendered, "map[")
		})
	}
}
