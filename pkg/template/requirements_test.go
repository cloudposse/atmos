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
