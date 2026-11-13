package templatesource

import (
	"bytes"
	"strings"
	"testing"
	"text/template"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// Valid templates without auth ownership must reach ordinary rendering unchanged.
// Each entry combines templated mapping keys with a different output-free action,
// the shape that the malformed-YAML guard previously rejected.
func TestProtectRendersValidTemplatesWithoutAuth(t *testing.T) {
	overrides := func(line string) string {
		return "vars:\n  packs:\n    {{- range $p := .packs }}\n    - name: {{ $p.name }}\n      " + line +
			"\n      overrides:\n        {{- range $k, $v := $p.overrides }}\n        {{ $k }}: {{ $v }}\n        {{- end }}\n    {{- end }}\n"
	}
	sources := map[string]string{
		"declaration":            overrides("{{- $o := $p.overrides }}"),
		"blank declaration":      overrides("{{- $_ := $p.name }}"),
		"multi-line declaration": overrides("{{- $o := printf \"%s\"\n        $p.name }}"),
		"declaration chain":      overrides("{{- $a := 1 }}{{ $b := 2 }}"),
		"break":                  overrides("{{- if eq $p.name \"stop\" }}{{ break }}{{ end }}"),
		"standalone break":       "vars:\n  packs:\n    {{- range $p := .packs }}\n    {{- if eq $p.name \"stop\" }}\n    {{- break }}\n    {{- end }}\n    - name: {{ $p.name }}\n      overrides:\n        {{- range $k, $v := $p.overrides }}\n        {{ $k }}: {{ $v }}\n        {{- end }}\n    {{- end }}\n",
		"standalone continue":    "vars:\n  packs:\n    {{- range $p := .packs }}\n    {{- if eq $p.name \"skip\" }}\n    {{- continue }}\n    {{- end }}\n    - name: {{ $p.name }}\n      overrides:\n        {{- range $k, $v := $p.overrides }}\n        {{ $k }}: {{ $v }}\n        {{- end }}\n    {{- end }}\n",
		"comment":                overrides("{{- /* overrides follow */}}"),
		"auth words in comments": "# environment specific identities\n" + overrides("{{- $o := $p.overrides }}"),
	}
	data := map[string]any{"packs": []map[string]any{
		{"name": "one", "overrides": map[string]string{"k": "v"}},
		{"name": "two", "overrides": map[string]string{"k": "w"}},
	}}
	for name, source := range sources {
		t.Run(name, func(t *testing.T) {
			protected, restore, err := Protect(source)
			require.NoError(t, err)
			require.Equal(t, source, protected)
			require.Equal(t, source, restore(protected))

			renderer, err := template.New("catalog").Parse(protected)
			require.NoError(t, err, "fixture must be a valid Go template")
			var output bytes.Buffer
			require.NoError(t, renderer.Execute(&output, data))
			var document struct {
				Vars struct {
					Packs []struct {
						Name      string            `yaml:"name"`
						Overrides map[string]string `yaml:"overrides"`
					} `yaml:"packs"`
				} `yaml:"vars"`
			}
			require.NoError(t, yaml.Unmarshal(output.Bytes(), &document), "fixture must render valid YAML")
			packs := document.Vars.Packs
			require.Len(t, packs, 2)
			require.Equal(t, "one", packs[0].Name)
			require.Equal(t, map[string]string{"k": "v"}, packs[0].Overrides)
			require.Equal(t, "two", packs[1].Name)
			require.Equal(t, map[string]string{"k": "w"}, packs[1].Overrides)
		})
	}
}

// Literal credentials beside output-free actions must still defer credential values.
func TestProtectDefersCredentialsBesideNonOutputActions(t *testing.T) {
	credentials := "auth:\n  identities:\n    service:\n      spec:\n        env:\n" +
		"          {{- $region := .region }}\n" +
		"          TOKEN: '{{ call .privileged }}'\n"
	application := "vars:\n  tags:\n    {{- range $k, $v := .tags }}\n    {{- $key := $k }}\n    {{ $key }}: {{ $v }}\n    {{- end }}\n"
	source := credentials + application

	protected, restore, err := Protect(source)
	require.NoError(t, err)
	require.NotContains(t, protected, ".privileged", "credential values must be deferred")
	require.Equal(t, source, restore(protected))

	calls := 0
	renderer, err := template.New("stack").Parse(protected)
	require.NoError(t, err)
	var output bytes.Buffer
	require.NoError(t, renderer.Execute(&output, map[string]any{
		"region":     "us-east-1",
		"tags":       map[string]string{"team": "platform"},
		"privileged": func() string { calls++; return "must-not-run" },
	}))
	require.Zero(t, calls, "general rendering must not execute credential templates")
	rendered := restore(output.String())
	require.Contains(t, rendered, "TOKEN: '{{ call .privileged }}'")
	var document map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(rendered), &document))
	require.Equal(t, "platform", document["vars"].(map[string]any)["tags"].(map[string]any)["team"])
}

func TestNonOutputLine(t *testing.T) {
	for line, want := range map[string]bool{
		"{{- $o := .x }}":                 true,
		"{{ $o = .x -}}":                  true,
		"{{- break }}":                    true,
		"{{ continue }}":                  true,
		"{{ if .x }}{{ break }}{{ end }}": true,
		"{{ $a := 1 }} {{ $b := 2 }}":     true,
		"{{ $o }}":                        false,
		"{{ .x }}":                        false,
		"{{ $o := .x }}{{ $o }}":          false,
		"{{ $o := .x }} key: value":       false,
		"key: {{ $o := .x }}":             false,
		"{{ breakfast }}":                 false,
	} {
		require.Equal(t, want, nonOutputLine(strings.TrimSpace(line)), line)
	}
}
