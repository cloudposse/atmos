package literalsource

import (
	"bytes"
	"strings"
	"testing"
	"text/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestPreserveExactScalar(t *testing.T) {
	for _, value := range []string{"", "  {{ external }}\n\n", "!env HOME", "<< .vars.foo >>", Encode("nested")} {
		decoded, ok := Decode(Encode(value))
		require.True(t, ok)
		assert.Equal(t, value, decoded)
		assert.Equal(t, value, Restore(Encode(value)))
	}
	_, ok := Decode(prefix + "bad%:END")
	assert.False(t, ok)
}

func TestProtectYAMLScalarsAndCollections(t *testing.T) {
	input := "value: !literal |+\n  {{ external }}\n\nlist: [!literal '{{ other }}', normal]\n"
	got, restore, err := ProtectYAML(input)
	require.NoError(t, err)
	var node yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte(restore(got)), &node))
	root := node.Content[0]
	assert.Equal(t, "!literal", root.Content[1].Tag)
	assert.Equal(t, "{{ external }}\n\n", root.Content[1].Value)
	assert.NotContains(t, got, "{{")
}

func TestProtectYAMLTemplateControlLines(t *testing.T) {
	got, _, err := ProtectYAML("vars:\n{{ if true }}\n  value: !literal '{{ external }}'\n{{ end }}\n")
	require.NoError(t, err)
	assert.Contains(t, got, "{{ if true }}")
	assert.Contains(t, got, "{{ end }}")
	assert.NotContains(t, got, "{{ external }}")
}

func TestProtectEncodedCustomDelimiters(t *testing.T) {
	input := "one: " + Encode("{{ first }}") + "\ntwo: " + Encode("!env HOME") + "\n"
	protected, restore := ProtectEncoded(input, "ATMOS", "END")
	assert.NotContains(t, protected, "ATMOS")
	assert.Equal(t, input, restore(protected))
}

func TestProtectYAMLArbitraryTemplateSource(t *testing.T) {
	for _, input := range []string{
		"vars:\n  {{ if true }}value: !literal &value '{{ external }}'{{ end }}\n",
		"vars:\n{{ if true }}\n  value: !literal\n    '{{ external }}'\n{{ end }}\n",

		"vars:\n  {{ if true }}value: !literal '{{ external }}'{{ end }}\n  {{ printf `key` }}: {{ .value }}\n",
		"vars:\n  << if true >>value: !literal '<< external >>'<< end >>\n  << .key >>: << .value >>\n",
		"vars:\n  {{ if true }}value: !literal |+\n    {{ external }}\n\n{{ end }}\n",
		"vars:\n{{ if true }}\n  list: [!literal '{{ external }}', normal]\n{{ end }}\n",
		"vars:\n  {{ if true }}value: !literal 'it''s {{ external }}'{{ end }}\n",
		"vars:\n  {{ if true }}value: !literal \"escaped \\\" {{ external }}\"{{ end }}\n",
	} {
		t.Run(input, func(t *testing.T) {
			masked, restore, err := ProtectYAML(input)
			require.NoError(t, err)
			assert.NotContains(t, masked, "external")
			assert.Equal(t, input, restore(masked))
		})
	}
}

func TestProtectYAMLTemplateSourceIgnoresTagText(t *testing.T) {
	input := "{{ if true }}\nvars:\n  quoted: 'a !literal {{ ordinary }}'\n  block: |\n    !literal {{ ordinary }}\n  # !literal {{ ordinary }}\n  value: !literal '{{ external }}'\n{{ end }}\n"
	masked, restore, err := ProtectYAML(input)
	require.NoError(t, err)
	assert.Equal(t, 3, strings.Count(masked, "{{ ordinary }}"))
	assert.NotContains(t, masked, "{{ external }}")
	assert.Equal(t, input, restore(masked))
}

func TestProtectYAMLPreservesUntaggedTemplateSource(t *testing.T) {
	input := "# preserve source spelling\na: {{ .vars.x }}\nb: !literal '{{ external.email }}'\n"
	masked, restore, err := ProtectYAML(input)
	require.NoError(t, err)
	require.Contains(t, masked, "a: {{ .vars.x }}")
	require.Equal(t, input, restore(masked))
	tmpl, err := template.New("source").Parse(masked)
	require.NoError(t, err)
	var output bytes.Buffer
	require.NoError(t, tmpl.Execute(&output, map[string]any{"vars": map[string]any{"x": "rendered"}}))
	require.Equal(t, "# preserve source spelling\na: rendered\nb: !literal '{{ external.email }}'\n", restore(output.String()))
}

func TestProtectYAMLPlainScalarSeparators(t *testing.T) {
	for _, tc := range []struct{ source, want string }{
		{"message: !literal hello, {{ .vars.stage }}\n", "message: !literal hello, {{ .vars.stage }}\n"},
		{"message: !literal hello] } {{ .vars.stage }}\n", "message: !literal hello] } {{ .vars.stage }}\n"},
		{"- !literal hello, {{ .vars.stage }}\n", "- !literal hello, {{ .vars.stage }}\n"},
		{"items: [!literal hello, '{{ .vars.stage }}']\n", "items: [!literal hello, 'rendered']\n"},
		{"items: {literal: !literal hello, normal: '{{ .vars.stage }}'}\n", "items: {literal: !literal hello, normal: 'rendered'}\n"},
		{"items: &items [!literal hello, '{{ .vars.stage }}']\n", "items: &items [!literal hello, 'rendered']\n"},
		{"items: !!seq &items [!literal hello, '{{ .vars.stage }}']\n", "items: !!seq &items [!literal hello, 'rendered']\n"},
		{"items: [{literal: !literal hello, normal: '{{ .vars.stage }}'}]\n", "items: [{literal: !literal hello, normal: 'rendered'}]\n"},
		{"items: [\n  !literal hello,\n  '{{ .vars.stage }}'\n]\n", "items: [\n  !literal hello,\n  'rendered'\n]\n"},
		{"previous: plain [ bracket\nmessage: !literal hello, {{ .vars.stage }}\n", "previous: plain [ bracket\nmessage: !literal hello, {{ .vars.stage }}\n"},
		{"previous: plain, [ bracket\nmessage: !literal hello, {{ .vars.stage }}\n", "previous: plain, [ bracket\nmessage: !literal hello, {{ .vars.stage }}\n"},
		{"{{ if true }}\nmessage: !literal hello, {{ .vars.stage }}\n{{ end }}\n", "\nmessage: !literal hello, {{ .vars.stage }}\n\n"},
	} {
		t.Run(tc.source, func(t *testing.T) {
			masked, restore, err := ProtectYAML(tc.source)
			require.NoError(t, err)
			require.Equal(t, tc.source, restore(masked))
			tmpl, err := template.New("plain").Parse(masked)
			require.NoError(t, err)
			var rendered bytes.Buffer
			require.NoError(t, tmpl.Execute(&rendered, map[string]any{"vars": map[string]any{"stage": "rendered"}}))
			require.Equal(t, tc.want, restore(rendered.String()))
		})
	}
}
