package step

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/automation"
)

func TestAutomationLibraryGoAPI(t *testing.T) {
	parent := NewVariables()
	parent.SetEnv("APP", "parent")
	library := NewAutomationLibrary(parent, nil)
	require.Contains(t, library.Names(), "join")
	call := &automation.StepCall{Type: "join", Configuration: map[string]any{
		"name": "services", "options": []string{"api", "worker"}, "separator": ",",
		"outputs": map[string]string{"names": "{{ .value }}"},
	}}
	require.NoError(t, library.Validate(call))
	assert.Empty(t, library.vars.Steps, "validation must not execute")
	result, err := library.Run(t.Context(), call)
	require.NoError(t, err)
	assert.Equal(t, "api,worker", result.Value)
	assert.Equal(t, map[string]string{"names": "api,worker"}, result.Outputs)
	assert.Empty(t, parent.Steps, "library state belongs to its invocation")
	assert.NotContains(t, call.Configuration, "type", "decoding must not mutate caller configuration")

	child := library.Fork()
	_, err = child.Run(t.Context(), &automation.StepCall{Type: "env", Configuration: map[string]any{"vars": map[string]string{"APP": "child"}}})
	require.NoError(t, err)
	_, err = library.Run(t.Context(), &automation.StepCall{Type: "env", Configuration: map[string]any{"vars": map[string]string{"APP": "updated-parent"}}})
	require.NoError(t, err)
	for _, tc := range []struct {
		library automation.StepLibrary
		want    string
	}{{child, "child:api,worker"}, {library, "updated-parent:api,worker"}} {
		result, err := tc.library.Run(t.Context(), &automation.StepCall{Type: "join", Configuration: map[string]any{"content": "{{ .env.APP }}:{{ .steps.services.outputs.names }}"}})
		require.NoError(t, err)
		assert.Equal(t, tc.want, result.Value)
	}
	assert.Equal(t, "parent", parent.Env["APP"])
}

func TestAutomationLibraryValidation(t *testing.T) {
	library := NewAutomationLibrary(nil, nil)
	for _, tc := range []struct {
		call    *automation.StepCall
		message string
	}{
		{nil, "request is required"},
		{&automation.StepCall{Type: "unknown"}, "unknown step type"},
		{&automation.StepCall{Type: "input"}, "required field"},
		{&automation.StepCall{Type: "join", Configuration: map[string]any{"missing": "value"}}, "unknown step field"},
		{&automation.StepCall{Type: "join", Configuration: map[string]any{"count": "invalid"}}, "cannot unmarshal"},
		{&automation.StepCall{Type: "parallel", Configuration: map[string]any{"steps": []any{map[string]any{"typo": true}}}}, "unknown step field"},
	} {
		require.ErrorContains(t, library.Validate(tc.call), tc.message)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := library.Run(ctx, &automation.StepCall{Type: "sleep"})
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, library.vars.Steps)
}

func TestAutomationLibraryNestingGuard(t *testing.T) {
	vars := NewVariables()
	vars.automationDepth = 64
	_, err := NewAutomationLibrary(vars, nil).Run(t.Context(), &automation.StepCall{Type: "join", Configuration: map[string]any{"content": "unused"}})
	require.ErrorIs(t, err, errUtils.ErrAutomation)
	require.ErrorContains(t, err, "nesting")
	vars.automationDepth = 0
	vars.automationParallel = true
	err = NewAutomationLibrary(vars, nil).Validate(&automation.StepCall{Type: "input", Configuration: map[string]any{"prompt": "nested", "default": "no"}})
	require.ErrorContains(t, err, "exclusive terminal")
}
