package exec

import (
	"fmt"
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

func TestLiteralScalarsWithTemplateControlFlow(t *testing.T) {
	config := &schema.AtmosConfiguration{}
	config.Templates.Settings.Enabled = true
	config.Templates.Settings.Evaluations = 2
	for _, value := range []string{
		"!literal '{{ .Values.tpl.source }}'",
		`!literal "{{ .Values.tpl.source }}"`,
		"!literal |-\n  {{ .Values.tpl.source }}",
		"!literal >-\n  {{ .Values.tpl.source }}",
	} {
		for _, enabled := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%t", value, enabled), func(t *testing.T) {
				input := "{{ if .enabled }}\nvalue: " + value + "\n{{ else }}\nvalue: disabled\n{{ end }}\n"
				rendered, err := ProcessTmplWithDatasources(config, &schema.ConfigAndStacksInfo{}, schema.Settings{}, "control", input, map[string]any{"enabled": enabled}, false)
				require.NoError(t, err)
				parsed, err := u.UnmarshalYAML[map[string]any](rendered)
				require.NoError(t, err)
				result, err := ProcessCustomYamlTags(config, parsed, "", nil, nil)
				require.NoError(t, err)
				want := "disabled"
				if enabled {
					want = "{{ .Values.tpl.source }}"
				}
				require.Equal(t, want, result["value"])
			})
		}
	}
}

func TestLiteralTemplateControlFlowContexts(t *testing.T) {
	cases := []struct {
		name, input string
		want        map[string]any
	}{
		{"tag on previous line", "{{ if .enabled }}\nvalue: !literal\n  '{{ unknown }}'\n{{ end }}\n", map[string]any{"value": "{{ unknown }}"}},
		{"CRLF", "{{ if .enabled }}\r\nvalue: !literal '{{ unknown }}'\r\n{{ end }}\r\n", map[string]any{"value": "{{ unknown }}"}},
		{"range", "values:\n{{ range .items }}\n  - !literal '{{ .Values.tpl.source }}'\n{{ end }}\n", map[string]any{"values": []any{"{{ .Values.tpl.source }}", "{{ .Values.tpl.source }}"}}},
		{"flow", "{{ if .enabled }}\nvalue: {literal: !literal '{{ .Values.tpl.source }}', rendered: '{{ .name }}'}\n{{ end }}\n", map[string]any{"value": map[string]any{"literal": "{{ .Values.tpl.source }}", "rendered": "resolved"}}},
		{"ordinary text", "{{ if .enabled }}\nvalue: 'mentions !literal {{ .name }}'\n# !literal {{ .name }}\n{{ end }}\n", map[string]any{"value": "mentions !literal resolved"}},
		{"trim markers", "{{- if .enabled }}\nvalue: !literal '{{ unknown }}'\n{{- end }}\n", map[string]any{"value": "{{ unknown }}"}},
		{"unicode and whitespace", "{{ if .enabled }}\nзначение: !literal '  {{ unknown }} café  ' # keep this comment\n{{ end }}\n", map[string]any{"значение": "  {{ unknown }} café  "}},
		{"literal newlines", "{{ if .enabled }}\nvalue: !literal |\n  {{ unknown }}\n  second\n{{ end }}\n", map[string]any{"value": "{{ unknown }}\nsecond\n"}},
		{"folded newlines", "{{ if .enabled }}\nvalue: !literal >-\n  {{ unknown }}\n  second\n{{ end }}\n", map[string]any{"value": "{{ unknown }} second"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			config := &schema.AtmosConfiguration{}
			config.Templates.Settings.Enabled = true
			config.Templates.Settings.Evaluations = 2
			data := map[string]any{"enabled": true, "items": []int{1, 2}, "name": "resolved"}
			rendered, err := ProcessTmplWithDatasources(config, &schema.ConfigAndStacksInfo{}, schema.Settings{}, "control", tc.input, data, false)
			require.NoError(t, err)
			parsed, err := u.UnmarshalYAML[map[string]any](rendered)
			require.NoError(t, err)
			result, err := ProcessCustomYamlTags(config, parsed, "", nil, nil)
			require.NoError(t, err)
			require.Equal(t, tc.want, result)
		})
	}
}
