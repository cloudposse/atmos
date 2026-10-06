package starlarksource

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestSourceRoundTrip(t *testing.T) {
	source := Source{Code: "\nreturn {'braces': '{{value}}'}\n", File: "catalog/shared.yaml", Line: 12}
	assert.Equal(t, source, Decode(source.Encode()))
	assert.Equal(t, "return 3", Decode("!starlark return 3").Code)
	assert.False(t, Is("!starlark.extra"))
	for _, code := range []string{"!starlark\n# atmos-starlark-source: %bad\nreturn 1", "!starlark\n# atmos-starlark-source: invalid"} {
		assert.NotEmpty(t, Decode(code).Code)
	}
}

func TestProtectSourceThroughYAML(t *testing.T) {
	input := `vars:
  untouched: "{{ .name }}"
  tags: !starlark |
    return {"literal": "{{ value }}", "other": "<< value >>"}
`
	protected, restore, err := Protect(input, "catalog/shared.yaml")
	require.NoError(t, err)
	assert.NotContains(t, protected, "{{ value }}")
	assert.Contains(t, protected, "{{ .name }}")
	rendered := restore(strings.ReplaceAll(protected, "{{ .name }}", "api"))
	var values map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(rendered), &values))
	vars := values["vars"].(map[string]any)
	assert.Equal(t, "api", vars["untouched"])
	source := Decode(vars["tags"].(string))
	assert.Equal(t, int32(4), source.Line)
	assert.Equal(t, "catalog/shared.yaml", source.File)
	assert.Contains(t, source.Code, "{{ value }}")
	assert.Contains(t, source.Code, "<< value >>")
	protected, restore, err = Protect(rendered, "later-pass")
	require.NoError(t, err)
	var again map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(restore(protected)), &again))
	assert.Equal(t, values, again)
}

func TestProtectLeavesOtherTagsAndOrdinaryText(t *testing.T) {
	for _, input := range []string{
		"value: !literal '!starlark return 1'\n",
		"value: this mentions !starlark in prose\n",
		"value: no tags\n",
	} {
		protected, restore, err := Protect(input, "stack.yaml")
		require.NoError(t, err)
		assert.Equal(t, input, protected)
		assert.Equal(t, input, restore(protected))
	}
}

func TestProtectCollisionAndInvalidYAML(t *testing.T) {
	input := "name: ATMOS_STARLARK_SOURCE_0\nvalue: !starlark return 1\n"
	protected, restore, err := Protect(input, "stack.yaml")
	require.NoError(t, err)
	var result map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(restore(protected)), &result))
	assert.Equal(t, "ATMOS_STARLARK_SOURCE_0", result["name"])
	assert.Equal(t, "return 1", Decode(result["value"].(string)).Code)
	_, _, err = Protect("value: [!starlark", "broken.yaml")
	require.ErrorContains(t, err, "broken.yaml")
}

func TestProtectManyExpressions(t *testing.T) {
	var input strings.Builder
	for i := range 15 {
		fmt.Fprintf(&input, "field%d: !starlark return %d\n", i, i)
	}
	protected, restore, err := Protect(input.String(), "many.yaml")
	require.NoError(t, err)
	var result map[string]string
	require.NoError(t, yaml.Unmarshal([]byte(restore(protected)), &result))
	for i := range 15 {
		assert.Equal(t, fmt.Sprintf("return %d", i), Decode(result[fmt.Sprintf("field%d", i)]).Code)
	}
}
