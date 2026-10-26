package starlarksource

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/function/yamlsource"
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
		"value: !literal\n  nested: !starlark |\n    return \"{{ literal }}\"\n",
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

func TestIsEncoded(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{"loader-encoded source", Source{Code: "return 1", File: "stack.yaml", Line: 3}.Encode(), true},
		{"encoded empty body", Source{File: "stack.yaml", Line: 1}.Encode(), true},
		{"quoted single line", "!starlark return 1", false},
		{"plain multiline", "!starlark\nreturn 1", false},
		{"similar tag", "!starlarkish", false},
		{"wrong case", "!STARLARK x", false},
		{"bare tag", "!starlark", false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsEncoded(tt.value))
		})
	}
}

func TestAuthoredSourceHeaderIsLiteral(t *testing.T) {
	assert.False(t, IsEncoded("!starlark\n# atmos-starlark-source: eyJmaWxlIjoic3RhY2sueWFtbCIsImxpbmUiOjF9\nreturn 1"))
}

func TestDecodeSharedSourceLineBounds(t *testing.T) {
	for _, tc := range []struct {
		line int
		want int32
	}{
		{0, 1},
		{-1, 1},
		{12, 12},
		{math.MaxInt32, math.MaxInt32},
		{math.MaxInt32 + 1, 1},
	} {
		source := yamlsource.Source{Tag: Tag, Argument: "return 1", File: "stack.yaml", Line: tc.line, Column: 7}
		decoded := Decode(source.Encode())
		assert.Equal(t, tc.want, decoded.Line)
		assert.Equal(t, source.Argument, decoded.Code)
		assert.Equal(t, source.File, decoded.File)
		assert.Equal(t, source.Column, decoded.Column)
	}
}

func TestProtectLiteralScalars(t *testing.T) {
	for _, value := range []string{"{{ .Values.tpl.source }}", "<< .Values.tpl.source >>", "!starlark return 1", "  !env SECRET  ", "first\n{{ invalid }}\n"} {
		t.Run(value, func(t *testing.T) {
			input := fmt.Sprintf("value: !literal %q\n", value)
			for range 2 {
				protected, restore, err := Protect(input, "literal.yaml")
				require.NoError(t, err)
				assert.NotContains(t, protected, value)
				input = restore(protected)
				var values map[string]string
				require.NoError(t, yaml.Unmarshal([]byte(input), &values))
				source, ok := yamlsource.Decode(values["value"])
				require.True(t, ok)
				assert.Equal(t, "!literal", source.Tag)
				assert.Equal(t, value, source.Argument)
			}
		})
	}
}
