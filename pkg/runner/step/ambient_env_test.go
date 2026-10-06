package step

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinel: a rename of the field the tests rely on fails the build.
var _ = schema.WorkflowStep{LiteralFields: []string{"env.X"}}

func TestApplyAmbientEnv(t *testing.T) {
	tests := []struct {
		name         string
		ambient      map[string]string
		declared     map[string]string
		existing     []string
		wantEnv      map[string]string
		wantLiteral  []string
		wantRendered []string
	}{
		{
			name:         "ambient entries are literal and declared entries are rendered",
			ambient:      map[string]string{"HOME": "/root", "FOO": "{{bad"},
			declared:     map[string]string{"GREETING": `{{ "hi" }}`},
			wantEnv:      map[string]string{"HOME": "/root", "FOO": "{{bad", "GREETING": `{{ "hi" }}`},
			wantLiteral:  []string{"HOME", "FOO"},
			wantRendered: []string{"GREETING"},
		},
		{
			name:         "a declared entry overrides the ambient value and is rendered",
			ambient:      map[string]string{"FOO": "ambient"},
			declared:     map[string]string{"FOO": `{{ "declared" }}`},
			wantEnv:      map[string]string{"FOO": `{{ "declared" }}`},
			wantRendered: []string{"FOO"},
		},
		{
			name:         "declared keys match ambient keys case-insensitively",
			ambient:      map[string]string{"Path": "/bin"},
			declared:     map[string]string{"PATH": "/usr/bin"},
			wantEnv:      map[string]string{"Path": "/bin", "PATH": "/usr/bin"},
			wantRendered: []string{"Path", "PATH"},
		},
		{
			name:        "an existing literal marker is kept",
			ambient:     map[string]string{"FOO": "x"},
			declared:    map[string]string{"KEEP": "{{ raw }}"},
			existing:    []string{"script", "env.KEEP"},
			wantEnv:     map[string]string{"FOO": "x", "KEEP": "{{ raw }}"},
			wantLiteral: []string{"FOO", "KEEP"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			step := &schema.WorkflowStep{LiteralFields: tt.existing}

			ApplyAmbientEnv(step, tt.ambient, tt.declared)

			assert.Equal(t, tt.wantEnv, step.Env)
			for _, key := range tt.wantLiteral {
				assert.True(t, step.IsLiteralEnv(key), "%s must be literal", key)
			}
			for _, key := range tt.wantRendered {
				assert.False(t, step.IsLiteralEnv(key), "%s must be rendered", key)
			}
			for _, field := range tt.existing {
				assert.Contains(t, step.LiteralFields, field)
			}
		})
	}
}

func TestApplyAmbientEnvDoesNotMutateItsInputs(t *testing.T) {
	ambient := map[string]string{"A": "1"}
	declared := map[string]string{"B": "2"}
	existing := []string{"script"}
	step := &schema.WorkflowStep{LiteralFields: existing}

	ApplyAmbientEnv(step, ambient, declared)

	assert.Equal(t, map[string]string{"A": "1"}, ambient)
	assert.Equal(t, map[string]string{"B": "2"}, declared)
	assert.Equal(t, []string{"script"}, existing)
}

// scriptAmbientEnvStep returns a starlark script step that prints the value the test binary reads
// from the named process environment variable.
func scriptAmbientEnvStep(t *testing.T, vars *Variables, name string, ambient, declared map[string]string) *schema.WorkflowStep {
	t.Helper()

	exe, err := os.Executable()
	require.NoError(t, err)
	vars.SetEnv("_ATMOS_STEP_FAKE", "printenv")
	vars.SetEnv(atmosStepFakeEnvNameEnv, name)
	declared["FAKE_EXE"] = exe
	step := &schema.WorkflowStep{
		Name: "read-env", Type: schema.TaskTypeScript, Interpreter: "starlark", Output: "none",
		Script: "r = exec.run([env[\"FAKE_EXE\"]], output=\"capture\")\noutput = r.stdout\n",
	}
	ApplyAmbientEnv(step, ambient, declared)
	step.ScriptEnv = declared
	return step
}

func TestScriptHandlerPassesAmbientEnvVerbatim(t *testing.T) {
	initShellTestIO(t)
	handler, ok := Get(schema.TaskTypeScript)
	require.True(t, ok)

	tests := []struct {
		name  string
		value string
	}{
		{name: "an unparsable template", value: "{{bad"},
		{name: "a template that would evaluate", value: `{{ "injected" }}`},
		{name: "a plain value", value: "plain"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vars := NewVariables()
			vars.SetEnv("AMBIENT_VALUE", tt.value)
			step := scriptAmbientEnvStep(t, vars, "AMBIENT_VALUE", map[string]string{"AMBIENT_VALUE": tt.value}, map[string]string{})

			result, err := handler.Execute(context.Background(), step, vars)

			require.NoError(t, err)
			assert.Equal(t, tt.value, result.Value)
		})
	}
}

func TestScriptHandlerStillRendersDeclaredEnv(t *testing.T) {
	initShellTestIO(t)
	handler, ok := Get(schema.TaskTypeScript)
	require.True(t, ok)

	vars := NewVariables()
	step := scriptAmbientEnvStep(t, vars, "DECLARED_VALUE", map[string]string{}, map[string]string{"DECLARED_VALUE": `{{ "rendered" }}`})

	result, err := handler.Execute(context.Background(), step, vars)

	require.NoError(t, err)
	assert.Equal(t, "rendered", result.Value)
}

func TestScriptHandlerDeclaredEnvWithBadTemplateStillFails(t *testing.T) {
	initShellTestIO(t)
	handler, ok := Get(schema.TaskTypeScript)
	require.True(t, ok)

	vars := NewVariables()
	step := scriptAmbientEnvStep(t, vars, "DECLARED_VALUE", map[string]string{}, map[string]string{"DECLARED_VALUE": "{{bad"})

	_, err := handler.Execute(context.Background(), step, vars)

	require.ErrorIs(t, err, errUtils.ErrTemplateEvaluation)
}
