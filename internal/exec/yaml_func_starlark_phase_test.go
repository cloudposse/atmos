package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestStarlarkYAMLAfterDeferredMerges(t *testing.T) {
	config := &schema.AtmosConfiguration{}
	vars, dctx, err := merge.MergeWithDeferred(config, []map[string]any{
		{"shared": `!template {"from_base":"catalog"}`},
		{"shared": map[string]any{"from_override": "prod"}, "copy": `!starlark return ctx.vars["shared"]`},
	})
	require.NoError(t, err)
	info := &schema.ConfigAndStacksInfo{Stack: "prod", ComponentSection: map[string]any{"vars": vars}, DeferredMergeContexts: ComponentDeferredContexts{"vars": dctx}}
	skip, finish := prepareConfigurationValues(config, info, nil, nil)
	info.ComponentSection, err = ProcessCustomYamlTags(config, info.ComponentSection, info.Stack, skip, info)
	require.NoError(t, err)
	require.NoError(t, resolveDeferredYamlFunctions(config, info, &schema.Settings{}, info.ComponentSection, nil, nil))
	require.NoError(t, finish())
	resolved := info.ComponentSection["vars"].(map[string]any)
	assert.Equal(t, map[string]any{"from_base": "catalog", "from_override": "prod"}, resolved["shared"])
	assert.Equal(t, resolved["shared"], resolved["copy"])
	assert.Equal(t, `!starlark return ctx.vars["shared"]`, vars["copy"], "cached source must stay unchanged")
}

func TestStarlarkYAMLFinalPhaseOnlyEvaluatesOriginalSources(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{ComponentSection: map[string]any{"vars": map[string]any{
		"original": `!starlark return "!starlark return 99"`,
		"external": "!template external-data",
		"nested":   []any{`!starlark return 3`},
	}}}
	_, finish := prepareConfigurationValues(&schema.AtmosConfiguration{}, info, nil, nil)
	info.ComponentSection["vars"].(map[string]any)["external"] = "!starlark return 99"
	require.NoError(t, finish())
	vars := info.ComponentSection["vars"].(map[string]any)
	assert.Equal(t, "!starlark return 99", vars["original"])
	assert.Equal(t, "!starlark return 99", vars["external"])
	assert.Equal(t, []any{int64(3)}, vars["nested"])
}

func TestStarlarkYAMLFinalPhaseRespectsSelectionAndSkip(t *testing.T) {
	for _, tc := range []struct{ skip, sections []string }{
		{[]string{"starlark"}, nil}, {nil, []string{"metadata"}},
	} {
		original := map[string]any{"vars": map[string]any{"value": `!starlark return unknown`}}
		info := &schema.ConfigAndStacksInfo{ComponentSection: original}
		_, finish := prepareConfigurationValues(&schema.AtmosConfiguration{}, info, tc.skip, tc.sections)
		require.NoError(t, finish())
		assert.Equal(t, original, info.ComponentSection)
	}
}
