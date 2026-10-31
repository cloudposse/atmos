package templatesource

import (
	"bytes"
	"strings"
	"testing"
	"text/template"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestProtectAllowsInlineControlsOutsideCredentialValues(t *testing.T) {
	for _, credential := range []string{"literal", "{{ call .privileged }}", "{{ if call .privileged }}literal{{ end }}"} {
		t.Run(credential, func(t *testing.T) {
			source := "vars:\n  stage: {{ if .enabled }}prod{{ else }}dev{{ end }}\nauth: {identities: {service: {spec: {env: {TOKEN: '" + credential + "'}}}}}"
			protected, restore, err := Protect(source)
			require.NoError(t, err)
			require.Equal(t, source, restore(protected))
			for _, enabled := range []bool{false, true} {
				calls := 0
				renderer, err := template.New("stack").Parse(protected)
				require.NoError(t, err)
				var output bytes.Buffer
				err = renderer.Execute(&output, map[string]any{"enabled": enabled, "privileged": func() string {
					calls++
					return "must-not-run"
				}})
				require.NoError(t, err)
				require.Zero(t, calls)
				var document map[string]any
				require.NoError(t, yaml.Unmarshal([]byte(restore(output.String())), &document))
				stage := "dev"
				if enabled {
					stage = "prod"
				}
				require.Equal(t, stage, document["vars"].(map[string]any)["stage"])
				require.Contains(t, restore(output.String()), "TOKEN: '"+credential+"'")
			}
		})
	}
}

func TestProtectDefersEveryDuplicateAuthPath(t *testing.T) {
	for name, source := range duplicateAuthSources("{TOKEN: '{{ call .privileged }}'}") {
		for spelling, source := range map[string]string{"plain": source, "escaped": escapeAuthKeys(source)} {
			t.Run(name+"/"+spelling, func(t *testing.T) {
				protected, restore, err := Protect(source)
				require.NoError(t, err)
				require.NotContains(t, protected, "call .privileged")
				require.Equal(t, source, restore(protected))
			})
		}
	}
}

func TestProtectDefersConditionalEnvironmentBranches(t *testing.T) {
	source := `auth:
  identities:
    service:
      spec:
{{ if .enabled }}
        env: {TOKEN: '{{ call .privileged }}'}
{{ else }}
        env: {TOKEN: '{{ call .privileged }}'}
{{ end }}
`
	protected, restore, err := Protect(source)
	require.NoError(t, err)
	require.NotContains(t, protected, "call .privileged")
	require.Equal(t, source, restore(protected))
	for _, enabled := range []bool{false, true} {
		calls := 0
		renderer, err := template.New("stack").Parse(protected)
		require.NoError(t, err)
		var output bytes.Buffer
		err = renderer.Execute(&output, map[string]any{"enabled": enabled, "privileged": func() string {
			calls++
			return "must-not-run"
		}})
		require.NoError(t, err)
		require.Zero(t, calls)
		var document map[string]any
		require.NoError(t, yaml.Unmarshal([]byte(restore(output.String())), &document))
		require.Contains(t, restore(output.String()), "TOKEN: '{{ call .privileged }}'")
	}
}

func TestProtectRejectsControlOnlyAliases(t *testing.T) {
	for name, shared := range map[string]string{
		"quoted": "shared: &value '{{ if call .privileged }}literal{{ end }}'\n",
		"block":  "shared: &value |-\n  {{ if call .privileged }}\n  literal\n  {{ end }}\n",
	} {
		t.Run(name, func(t *testing.T) {
			source := shared + "vars:\n  stage: {{ if .enabled }}prod{{ else }}dev{{ end }}\nauth: {identities: {service: {spec: {env: {TOKEN: *value}}}}}"
			protected, _, err := Protect(source)
			require.ErrorIs(t, err, errUtils.ErrInvalidAuthConfig)
			require.Empty(t, protected)
		})
	}
}

func duplicateAuthSources(value string) map[string]string {
	safe := "{TOKEN: 'literal'}"
	identity := func(env string) string { return "{spec: {env: " + env + "}}" }
	identities := func(env string) string { return "{service: " + identity(env) + "}" }
	auth := func(env string) string { return "{identities: " + identities(env) + "}" }
	return map[string]string{
		"auth":                     "auth: " + auth(safe) + "\nauth: " + auth(value),
		"identities":               "auth: {identities: " + identities(safe) + ", identities: " + identities(value) + "}",
		"identity":                 "auth: {identities: {service: " + identity(safe) + ", service: " + identity(value) + "}}",
		"unquoted earlier literal": "auth: {identities: {service: " + identity("{TOKEN: literal}") + ", service: " + identity(value) + "}}",
		"spec":                     "auth: {identities: {service: {spec: {env: " + safe + "}, spec: {env: " + value + "}}}}",
		"env":                      "auth: {identities: {service: {spec: {env: " + safe + ", env: " + value + "}}}}",
	}
}

func escapeAuthKeys(source string) string {
	return strings.NewReplacer("auth:", `"\u0061uth":`, "identities:", `"ident\u0069ties":`, "service:", `"serv\u0069ce":`, "spec:", `"\u0073pec":`, "env:", `"\u0065nv":`).Replace(source)
}

func TestProtectRejectsUnquotedControlPrefixes(t *testing.T) {
	for name, value := range map[string]string{
		"inline":           "{{ if call .privileged }}literal{{ end }}",
		"standalone":       "\n            {{ if call .privileged }}\n            literal\n            {{ end }}",
		"empty standalone": "\n            {{ if call .privileged }}\n            {{ end }}",
	} {
		t.Run(name, func(t *testing.T) {
			source := "auth:\n  identities:\n    service:\n      spec:\n        env:\n          TOKEN: " + value + "\n"
			protected, _, err := Protect(source)
			require.ErrorIs(t, err, errUtils.ErrInvalidAuthConfig)
			require.Empty(t, protected)
		})
	}
}

func TestProtectContinuesAfterNonMappingIdentities(t *testing.T) {
	source := `auth:
{{ if .skip }}
  identities: null
{{ else }}
  identities:
    service:
      spec:
        env: {TOKEN: '{{ call .privileged }}'}
{{ end }}
`
	assertDeferredConditionalCredentials(t, source)
}

func TestProtectContinuesAfterNonMappingIntermediatePaths(t *testing.T) {
	for name, source := range map[string]string{
		"identity": `auth:
  identities:
{{ if .skip }}
    service: literal
{{ else }}
    service:
      spec:
        env: {TOKEN: '{{ call .privileged }}'}
{{ end }}
`,
		"spec": `auth:
  identities:
    service:
{{ if .skip }}
      spec: null
{{ else }}
      spec:
        env: {TOKEN: '{{ call .privileged }}'}
{{ end }}
`,
	} {
		t.Run(name, func(t *testing.T) {
			assertDeferredConditionalCredentials(t, source)
		})
	}
}

func assertDeferredConditionalCredentials(t *testing.T, source string) {
	t.Helper()
	for spelling, source := range map[string]string{"plain": source, "escaped": escapeAuthKeys(source)} {
		t.Run(spelling, func(t *testing.T) {
			protected, restore, err := Protect(source)
			require.NoError(t, err)
			require.NotContains(t, protected, "call .privileged", "later credential paths must survive earlier non-mapping alternatives")
			require.Equal(t, source, restore(protected), "protection must restore the exact original source")
			calls := 0
			renderer, err := template.New("stack").Parse(protected)
			require.NoError(t, err)
			var output bytes.Buffer
			err = renderer.Execute(&output, map[string]any{"skip": false, "privileged": func() string {
				calls++
				return "must-not-run"
			}})
			require.NoError(t, err)
			require.Zero(t, calls, "the selected credential branch must remain deferred")
			var document map[string]any
			require.NoError(t, yaml.Unmarshal([]byte(restore(output.String())), &document))
			env := document["auth"].(map[string]any)["identities"].(map[string]any)["service"].(map[string]any)["spec"].(map[string]any)["env"].(map[string]any)
			require.Equal(t, "{{ call .privileged }}", env["TOKEN"])
		})
	}
}
