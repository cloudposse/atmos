package deferred

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExpandStarlarkEvaluationPaths(t *testing.T) {
	data := map[string]any{"vars": map[string]any{
		"name":  `!starlark return ctx.vars["stage"]`,
		"stage": "prod",
	}}
	assert.Nil(t, ExpandEvaluationPaths(data, [][]string{{"vars", "name"}}))
	assert.Equal(t, [][]string{{"vars", "stage"}}, ExpandEvaluationPaths(data, [][]string{{"vars", "stage"}}))
}
