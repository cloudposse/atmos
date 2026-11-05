package hooks

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/utils"
)

// Compile-time sentinel: a rename of the field these tests rely on must fail the build.
var _ = schema.WorkflowStep{LiteralFields: nil}

// failIfRendered is a Starlark body that fails when the template text inside it was rendered.
const failIfRendered = `text = "{{ x }}"
if text != "{" + "{ x }}":
    fail("the script was rendered: " + text)
`

// hookWith decodes a hook manifest exactly like stack processing does (so !literal is recorded as
// literal_fields) and returns the hook's `with` value.
func hookWith(t *testing.T, manifest string) any {
	t.Helper()
	out, err := utils.UnmarshalYAMLFromFile[map[string]any](&schema.AtmosConfiguration{}, manifest, "stack.yaml")
	require.NoError(t, err)
	hooksSection, ok := out["hooks"].(map[string]any)
	require.True(t, ok)
	hook, ok := hooksSection["h"].(map[string]any)
	require.True(t, ok)
	return hook["with"]
}

func TestStepHookLiteralScriptIsNotRendered(t *testing.T) {
	with := hookWith(t, "hooks:\n  h:\n    kind: step\n    with:\n      interpreter: starlark\n      output: none\n      script: !literal |\n        "+
		"text = \"{{ x }}\"\n        if text != \"{\" + \"{ x }}\":\n            fail(\"the script was rendered: \" + text)\n")
	m, ok := with.(map[string]any)
	require.True(t, ok)
	require.Equal(t, []any{"script"}, m[schema.LiteralFieldsKey])

	ctx := stepExecContext(&Hook{Kind: stepKindName, Type: "script", OnFailure: OnFailureFail, With: with})
	_, err := stepEngine{}.Run(ctx)
	require.NoError(t, err)

	t.Run("the decoded step carries the marker and no step parameter leaks", func(t *testing.T) {
		ws, err := stepFromHookWithVariables(ctx, stepVariables(ctx))
		require.NoError(t, err)
		assert.Equal(t, []string{"script"}, ws.LiteralFields)
		assert.Equal(t, failIfRendered, ws.Script)
		assert.NotContains(t, ws.With, schema.LiteralFieldsKey)
	})
}

func TestStepHookWithoutLiteralRendersTheScript(t *testing.T) {
	// Negative path: the same body without the marker is rendered as a template and fails.
	with := map[string]any{"interpreter": "starlark", "output": "none", "script": failIfRendered}
	ctx := stepExecContext(&Hook{Kind: stepKindName, Type: "script", OnFailure: OnFailureFail, With: with})
	_, err := stepEngine{}.Run(ctx)
	require.Error(t, err)
}

func TestStepHookLiteralFieldsInAnyKeyedPayload(t *testing.T) {
	// A hook's `with:` reaches the engine as map[any]any after the typed Hook round trips through
	// yaml.v2, so the renderer must honor the marker in that shape as well.
	with := map[any]any{
		"interpreter":           "starlark",
		"output":                "none",
		"script":                failIfRendered,
		schema.LiteralFieldsKey: []any{"script", "env.GREETING"},
		"env":                   map[any]any{"GREETING": "{{ g }}", "PLAIN": `{{ "p" }}`},
	}
	ctx := stepExecContext(&Hook{Kind: stepKindName, Type: "script", OnFailure: OnFailureFail, With: with})

	ws, err := stepFromHookWithVariables(ctx, stepVariables(ctx))
	require.NoError(t, err)
	assert.Equal(t, failIfRendered, ws.Script)
	assert.Equal(t, map[string]string{"GREETING": "{{ g }}", "PLAIN": "p"}, ws.Env)
	assert.Equal(t, []string{"script", "env.GREETING"}, ws.LiteralFields)
}

func TestStepsHookLiteralFieldsPerStep(t *testing.T) {
	with := hookWith(t, `hooks:
  h:
    kind: steps
    with:
      - type: script
        interpreter: starlark
        output: none
        script: !literal |
          text = "{{ x }}"
          if text != "{" + "{ x }}":
              fail("the script was rendered: " + text)
      - type: script
        interpreter: '{{ "star" }}{{ "lark" }}'
        output: none
        script: print("{{ "second" }}")
`)
	ctx := stepsExecContext(&Hook{Kind: stepsKindName, OnFailure: OnFailureFail, With: with})
	_, err := stepsEngine{}.Run(ctx)
	require.NoError(t, err, "the literal step keeps its braces and the plain step still renders")
}

func TestStepHookParallelChildKeepsLiteralScript(t *testing.T) {
	with := hookWith(t, `hooks:
  h:
    kind: step
    with:
      type: parallel
      steps:
        - name: child
          type: script
          interpreter: starlark
          output: none
          script: !literal |
            text = "{{ x }}"
            if text != "{" + "{ x }}":
                fail("the script was rendered: " + text)
`)
	ctx := stepExecContext(&Hook{Kind: stepKindName, Type: "parallel", With: with})

	ws, err := stepFromHookWithVariables(ctx, stepVariables(ctx))
	require.NoError(t, err)
	require.Len(t, ws.Steps, 1)
	assert.Equal(t, []string{"script"}, ws.Steps[0].LiteralFields)
	assert.Equal(t, failIfRendered, ws.Steps[0].Script)
}

func TestStepHookLiteralMarkerIsNotAStepParameter(t *testing.T) {
	m := map[string]any{"a": 1, schema.LiteralFieldsKey: []any{"script"}}
	got := withoutScriptSource(m)
	assert.Equal(t, map[string]any{"a": 1}, got)
	assert.Contains(t, m, schema.LiteralFieldsKey, "the input is not modified")

	plain := map[string]any{"a": 1}
	assert.Equal(t, plain, withoutScriptSource(plain))
}
