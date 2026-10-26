package exec

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	atmosYaml "github.com/cloudposse/atmos/pkg/yaml"
)

// Literal values must survive loading, repeated template passes, and YAML functions.
func TestLiteralScalarsSurviveComponentTemplates(t *testing.T) {
	t.Setenv("ATMOS_LITERAL_TEST", "must not resolve")
	for _, pin := range []string{"", "2026-10-19"} {
		t.Run(pin, func(t *testing.T) {
			config := &schema.AtmosConfiguration{Edition: pin}
			config.Templates.Settings.Enabled = true
			config.Templates.Settings.Sprig.Enabled = true
			config.Templates.Settings.Gomplate.Enabled = true
			config.Templates.Settings.Evaluations = 2
			input, _, err := u.UnmarshalYAMLFromFileWithPositions[map[string]any](config, `
regular: '{{ .name }}'
helm: !literal '{{ .Values.tpl.source }}'
invalid: !literal '{{ this is not valid Go template syntax }}'
function: !literal '  !env ATMOS_LITERAL_TEST  '
block: !literal |
  {{ .Values.tpl.source }}
  !starlark return 1
list:
  - !literal '{{ .Values.tpl.source }}'
`, "literal.yaml")
			require.NoError(t, err)
			serialized, err := atmosYaml.ConvertToYAMLPreservingDelimiters(input, nil)
			require.NoError(t, err)
			rendered, err := ProcessTmplWithDatasources(config, &schema.ConfigAndStacksInfo{}, schema.Settings{}, "literal", serialized, map[string]any{"name": "resolved"}, false)
			require.NoError(t, err)
			parsed, err := u.UnmarshalYAML[map[string]any](rendered)
			require.NoError(t, err)
			result, err := ProcessCustomYamlTags(config, parsed, "", nil, nil)
			require.NoError(t, err)
			require.Equal(t, map[string]any{
				"regular":  "resolved",
				"helm":     "{{ .Values.tpl.source }}",
				"invalid":  "{{ this is not valid Go template syntax }}",
				"function": "  !env ATMOS_LITERAL_TEST  ",
				"block":    "{{ .Values.tpl.source }}\n!starlark return 1\n",
				"list":     []any{"{{ .Values.tpl.source }}"},
			}, result)
		})
	}
}
