package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestStarlarkYAMLInheritedValues(t *testing.T) {
	config := &schema.AtmosConfiguration{}
	expression := starlarkTestSource(`return {"Environment": ctx.vars["stage"]}`)
	cases := []struct {
		name           string
		base, override any
		want           map[string]any
	}{
		{"inherited expression", expression, nil, map[string]any{"Environment": "prod"}},
		{"expression overrides map", map[string]any{"Old": "tag"}, expression, map[string]any{"Environment": "prod"}},
		{"map overrides expression", expression, map[string]any{"Owner": "team"}, map[string]any{"Owner": "team"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base := map[string]any{"stage": "dev", "tags": tc.base}
			override := map[string]any{"stage": "prod"}
			if tc.override != nil {
				override["tags"] = tc.override
			}
			vars, _, err := merge.MergeWithDeferred(config, []map[string]any{base, override})
			require.NoError(t, err)
			input := map[string]any{"vars": vars}
			result, err := ProcessCustomYamlTags(config, input, "prod", nil, nil)
			require.NoError(t, err)
			assert.Equal(t, tc.want, result["vars"].(map[string]any)["tags"])
			assert.Equal(t, tc.base, base["tags"])
			assert.Equal(t, "dev", base["stage"])
		})
	}
}
