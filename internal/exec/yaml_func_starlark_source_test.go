package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/function/starlarksource"
	"github.com/cloudposse/atmos/pkg/schema"
)

// starlarkTestSource returns code in the form the YAML loader's !starlark tag produces.
func starlarkTestSource(code string) string {
	return starlarksource.Source{Code: code, File: "test.yaml", Line: 1}.Encode()
}

func TestStarlarkYAMLOnlyEncodedSourceIsCode(t *testing.T) {
	cases := []struct {
		name  string
		value any
		want  any
	}{
		{"quoted tag stays a string", "!starlark return 1", "!starlark return 1"},
		{"multiline plain string stays a string", "!starlark\nreturn 1", "!starlark\nreturn 1"},
		{"encoded source is evaluated", starlarkTestSource("return 1"), int64(1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := map[string]any{"vars": map[string]any{"value": tc.value}}
			result, err := ProcessCustomYamlTags(&schema.AtmosConfiguration{}, input, "dev", nil, nil)
			require.NoError(t, err)
			assert.Equal(t, tc.want, result["vars"].(map[string]any)["value"])
		})
	}
}

func TestStarlarkYAMLListSourcesSurviveUnset(t *testing.T) {
	cases := []struct {
		name string
		list []any
		want []any
	}{
		{"unset before source", []any{"a", "!unset", starlarkTestSource(`return "c"`)}, []any{"a", "c"}},
		{"leading unset", []any{"!unset", starlarkTestSource(`return "c"`)}, []any{"c"}},
		{"unset after source", []any{"a", starlarkTestSource(`return "b"`), "!unset"}, []any{"a", "b"}},
		{"several unsets", []any{"!unset", "a", "!unset", starlarkTestSource(`return "c"`), "!unset", starlarkTestSource(`return "d"`)}, []any{"a", "c", "d"}},
		{"unset marker item", []any{"a", UnsetMarker{IsUnset: true}, starlarkTestSource(`return "c"`)}, []any{"a", "c"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			info := &schema.ConfigAndStacksInfo{ComponentSection: map[string]any{"vars": map[string]any{"list": tc.list}}}
			skip, finish := prepareConfigurationValues(&schema.AtmosConfiguration{}, info, nil, nil)
			var err error
			info.ComponentSection, err = ProcessCustomYamlTags(&schema.AtmosConfiguration{}, info.ComponentSection, "dev", skip, info)
			require.NoError(t, err)
			require.NoError(t, finish())
			got := info.ComponentSection["vars"].(map[string]any)["list"]
			assert.Equal(t, tc.want, got)
		})
	}
}
