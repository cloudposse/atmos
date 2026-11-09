package templatesource

import (
	"bytes"
	"testing"
	"text/template"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
)

// Regression for https://github.com/cloudposse/atmos/issues/3650: a standalone
// variable assignment emits no output, so the malformed-YAML fallback must not
// treat it as a scalar when templated application keys share the file.
func TestProtectRendersTemplatedKeysBesideVariableAssignments(t *testing.T) {
	for name, assignment := range map[string]string{
		"declaration":  "{{- $o := $p.overrides }}",
		"reassignment": "{{- $o = $p.overrides }}",
	} {
		t.Run(name, func(t *testing.T) {
			declare := ""
			if name == "reassignment" {
				declare = "{{- $o := dict }}\n"
			}
			source := declare + `components:
  terraform:
    demo:
      vars:
        packs:
          {{- range $p := .packs }}
          - name: {{ $p.name }}
            ` + assignment + `
            overrides:
              {{- range $k, $v := $o }}
              {{ $k }}: {{ $v }}
              {{- end }}
          {{- end }}
`
			protected, restore, err := Protect(source)
			require.NoError(t, err)
			require.Equal(t, source, protected, "templates without auth ownership must reach ordinary rendering unchanged")
			require.Equal(t, source, restore(protected))

			renderer, err := template.New("catalog").Funcs(template.FuncMap{"dict": func() map[string]string { return nil }}).Parse(protected)
			require.NoError(t, err)
			var output bytes.Buffer
			require.NoError(t, renderer.Execute(&output, map[string]any{"packs": []map[string]any{
				{"name": "one", "overrides": map[string]string{"k": "v"}},
				{"name": "two", "overrides": map[string]string{"k": "w"}},
			}}))
			var document struct {
				Components struct {
					Terraform map[string]struct {
						Vars struct {
							Packs []struct {
								Name      string            `yaml:"name"`
								Overrides map[string]string `yaml:"overrides"`
							} `yaml:"packs"`
						} `yaml:"vars"`
					} `yaml:"terraform"`
				} `yaml:"components"`
			}
			require.NoError(t, yaml.Unmarshal(output.Bytes(), &document))
			packs := document.Components.Terraform["demo"].Vars.Packs
			require.Len(t, packs, 2)
			require.Equal(t, "one", packs[0].Name)
			require.Equal(t, map[string]string{"k": "v"}, packs[0].Overrides)
			require.Equal(t, "two", packs[1].Name)
			require.Equal(t, map[string]string{"k": "w"}, packs[1].Overrides)
		})
	}
}

// Dropping assignments must not let templated keys assemble a credential path.
func TestProtectRejectsTemplatedAuthKeysBesideVariableAssignments(t *testing.T) {
	source := `{{- $root := .root }}
{{ $root }}:
  {{ .ids }}:
    service:
      {{ .spec }}:
        {{ .env }}:
          TOKEN: '{{ call .privileged }}'
`
	calls := 0
	protected, _, protectErr := Protect(source)
	if protectErr == nil {
		renderer, err := template.New("credentials").Parse(protected)
		require.NoError(t, err)
		var output bytes.Buffer
		require.NoError(t, renderer.Execute(&output, map[string]any{
			"root": "auth", "ids": "identities", "spec": "spec", "env": "env",
			"privileged": func() string { calls++; return "must-not-run" },
		}))
	}
	require.Zero(t, calls, "templated auth keys must still fail closed")
	require.ErrorIs(t, protectErr, errUtils.ErrInvalidAuthConfig)
	require.Empty(t, protected)
}

// Literal credential keys keep the strict malformed-YAML rejection.
func TestProtectRejectsMalformedAuthBesideVariableAssignments(t *testing.T) {
	source := `auth:
  identities:
    service:
      {{- $t := .token }}
      spec:
        env:
          {{ .key }}: '{{ call .privileged }}'
`
	protected, _, err := Protect(source)
	require.ErrorIs(t, err, errUtils.ErrInvalidAuthConfig)
	require.Empty(t, protected)
}
