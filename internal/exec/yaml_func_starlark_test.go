package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/degradation"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func TestStarlarkYAMLContextAndDependencies(t *testing.T) {
	source := `vars:
  final: !starlark |
    return ctx.vars["later"] + 1
  later: !starlark |
    return ctx.vars["count"] * 2
  count: 3
  tags: !starlark |
    return {"Environment": ctx.vars["stage"], "Owner": ctx.metadata.get("owner", "platform")}
  stage: prod
metadata:
  owner: team-a
`
	config := &schema.AtmosConfiguration{}
	input, err := u.UnmarshalYAMLFromFile[map[string]any](config, source, "catalog/service.yaml")
	require.NoError(t, err)
	info := &schema.ConfigAndStacksInfo{Component: "api", ComponentType: "terraform", ComponentSection: input}
	for range 10 {
		result, err := ProcessCustomYamlTags(config, input, "dev", nil, info)
		require.NoError(t, err)
		vars := result["vars"].(map[string]any)
		assert.Equal(t, int64(7), vars["final"])
		assert.Equal(t, map[string]any{"Environment": "prod", "Owner": "team-a"}, vars["tags"])
	}
	assert.IsType(t, "", input["vars"].(map[string]any)["later"], "input must remain unchanged")
}

func TestStarlarkYAMLCyclesAndSkip(t *testing.T) {
	config := &schema.AtmosConfiguration{}
	input := map[string]any{"vars": map[string]any{"a": starlarkTestSource(`return ctx.vars["b"]`), "b": starlarkTestSource(`return ctx.vars["a"]`)}}
	_, err := ProcessCustomYamlTags(config, input, "dev", nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "circular dependency")
	result, err := ProcessCustomYamlTags(config, input, "dev", []string{"starlark"}, nil)
	require.NoError(t, err)
	assert.Equal(t, input, result)
}

func TestStarlarkYAMLTemplatesAndSource(t *testing.T) {
	config := &schema.AtmosConfiguration{}
	input := `vars:
  stage: prod
  value: !starlark |
    return "{{ keep.this }}"
`
	rendered, err := ProcessTmpl(config, "stack.yaml", input, map[string]any{}, false)
	require.NoError(t, err)
	parsed, err := u.UnmarshalYAMLFromFile[map[string]any](config, rendered, "stack.yaml")
	require.NoError(t, err)
	result, err := ProcessCustomYamlTags(config, parsed, "dev", nil, nil)
	require.NoError(t, err)
	assert.Equal(t, "{{ keep.this }}", result["vars"].(map[string]any)["value"])
}

func TestStarlarkYAMLExistingFunctionDependency(t *testing.T) {
	t.Setenv("ATMOS_TEST_STARLARK_REGION", "us-east-2")
	input := map[string]any{"vars": map[string]any{
		"region": "!env ATMOS_TEST_STARLARK_REGION",
		"name":   starlarkTestSource(`return ctx.vars["region"] + "-app"`),
	}}
	result, err := ProcessCustomYamlTags(&schema.AtmosConfiguration{}, input, "dev", nil, nil)
	require.NoError(t, err)
	assert.Equal(t, "us-east-2-app", result["vars"].(map[string]any)["name"])
	result, err = ProcessCustomYamlTags(&schema.AtmosConfiguration{}, input, "dev", []string{"env"}, nil)
	require.NoError(t, err)
	assert.Equal(t, "!env ATMOS_TEST_STARLARK_REGION-app", result["vars"].(map[string]any)["name"])
}

func TestStarlarkYAMLPartialEvaluationContext(t *testing.T) {
	input := map[string]any{"vars": map[string]any{"owner": starlarkTestSource(`return ctx.metadata["owner"]`)}}
	info := &schema.ConfigAndStacksInfo{ComponentSection: map[string]any{"metadata": map[string]any{"owner": "team-a"}}}
	result, err := ProcessCustomYamlTags(&schema.AtmosConfiguration{}, input, "dev", nil, info)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"vars": map[string]any{"owner": "team-a"}}, result)
}

func TestStarlarkYAMLConcurrentComponents(t *testing.T) {
	t.Parallel()

	source := starlarkTestSource(`return {"stage": ctx.vars["stage"], "component": ctx.component}`)
	for _, stage := range []string{"dev", "prod"} {
		t.Run(stage, func(t *testing.T) {
			t.Parallel()
			input := map[string]any{"vars": map[string]any{"stage": stage, "tags": source}}
			info := &schema.ConfigAndStacksInfo{Component: stage, ComponentSection: input}
			for range 20 {
				result, err := ProcessCustomYamlTags(&schema.AtmosConfiguration{}, input, stage, nil, info)
				require.NoError(t, err)
				assert.Equal(t, map[string]any{"stage": stage, "component": stage}, result["vars"].(map[string]any)["tags"])
			}
		})
	}
}

func TestStarlarkYAMLSelectedFieldContext(t *testing.T) {
	input := map[string]any{"vars": map[string]any{"name": starlarkTestSource(`return ctx.vars["stage"] + ctx.vars["nested"]["suffix"]`)}}
	info := &schema.ConfigAndStacksInfo{ComponentSection: map[string]any{"vars": map[string]any{"stage": "prod", "nested": map[string]any{"suffix": "-api"}}}}
	result, err := ProcessCustomYamlTags(&schema.AtmosConfiguration{}, input, "prod", nil, info)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"vars": map[string]any{"name": "prod-api"}}, result)
	assert.NotContains(t, info.ComponentSection["vars"], "name")
}

func TestStarlarkYAMLUnsetContext(t *testing.T) {
	input := map[string]any{"vars": map[string]any{"copy": starlarkTestSource(`return ctx.settings`)}, "settings": map[string]any{"keep": "value", "drop": "!unset"}}
	result, err := ProcessCustomYamlTags(&schema.AtmosConfiguration{}, input, "dev", nil, nil)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"keep": "value"}, result["vars"].(map[string]any)["copy"])
}

func TestStarlarkYAMLComputedDependency(t *testing.T) {
	input := map[string]any{"vars": map[string]any{
		"unknown":     degradation.AtmosComputedValue{},
		"derived":     starlarkTestSource(`return ctx.vars["unknown"] + "-name"`),
		"independent": starlarkTestSource(`return 3`),
	}}
	result, err := ProcessCustomYamlTags(&schema.AtmosConfiguration{}, input, "dev", nil, nil)
	require.NoError(t, err)
	assert.Equal(t, degradation.AtmosComputedValue{}, result["vars"].(map[string]any)["derived"])
	assert.Equal(t, int64(3), result["vars"].(map[string]any)["independent"])
	input["vars"].(map[string]any)["independent"] = starlarkTestSource(`return missing`)
	_, err = ProcessCustomYamlTags(&schema.AtmosConfiguration{}, input, "dev", nil, nil)
	require.ErrorContains(t, err, "undefined: missing")
}
