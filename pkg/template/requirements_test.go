package template

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStaticFieldRefs(t *testing.T) {
	for _, tc := range []struct {
		input  string
		static bool
		paths  []FieldRef
	}{
		{`{{ (atmos.Component "vpc" .vars.stage).outputs.id }}`, true, []FieldRef{{Path: []string{"vars", "stage"}}}},
		{`{{ atmos.Resolve "!aws.account_id" }}`, true, nil},
		{`{{ index . .key }}`, false, []FieldRef{{Path: []string{"key"}}}},
		{`{{ range .vars.items }}{{ .name }}{{ end }}`, false, []FieldRef{{Path: []string{"vars", "items"}}, {Path: []string{"name"}}}},
		{`{{ broken`, false, nil},
	} {
		t.Run(tc.input, func(t *testing.T) {
			refs, static := StaticFieldRefs(tc.input)
			assert.Equal(t, tc.static, static)
			assert.Equal(t, tc.paths, refs)
		})
	}
}

func TestStaticFieldRefsCustomDelimiters(t *testing.T) {
	refs, static := StaticFieldRefs(`[[ .vars.name ]] {{ .unused }}`, "[[", "]]")
	assert.True(t, static)
	assert.Equal(t, []FieldRef{{Path: []string{"vars", "name"}}}, refs)
	_, static = StaticFieldRefs(`[[ index . .key ]]`, "[[", "]]")
	assert.False(t, static)
	_, static = StaticFieldRefs(`[[ .vars.name`, "[[", "]]")
	assert.False(t, static)
	_, static = StaticFieldRefs(`[[ .vars.name ]]`, "[[")
	assert.False(t, static)
}

func TestStaticFunctionNames(t *testing.T) {
	for _, tc := range []struct {
		input string
		names []string
		ok    bool
	}{
		{`{{ .vars.stage }}`, nil, true},
		{`{{ .vars.tenant }}-{{ .vars.stage | upper }}`, []string{"upper"}, true},
		{`{{ printf "%s-%s" .vars.a .vars.b | lower | lower }}`, []string{"printf", "lower"}, true},
		{`{{ env "HOME" }}`, []string{"env"}, true},
		{`{{ atmos.Resolve "!aws.account_id" }}`, []string{"atmos"}, true},
		{`{{ if eq .vars.stage "dev" }}d{{ end }}`, []string{"eq"}, true},
		{`{{ broken`, nil, false},
	} {
		t.Run(tc.input, func(t *testing.T) {
			names, ok := StaticFunctionNames(tc.input)
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.names, names)
		})
	}
}

func TestStaticFunctionNamesCustomDelimiters(t *testing.T) {
	names, ok := StaticFunctionNames(`<% .vars.name | upper %> {{ env "X" }}`, "<%", "%>")
	assert.True(t, ok)
	assert.Equal(t, []string{"upper"}, names)
	_, ok = StaticFunctionNames(`<% upper %>`, "<%")
	assert.False(t, ok)
}
