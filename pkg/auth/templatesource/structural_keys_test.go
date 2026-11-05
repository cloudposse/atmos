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

func TestProtectRejectsTemplatedStructuralKeys(t *testing.T) {
	for name, source := range structuralKeySources("{{ call .privileged }}") {
		t.Run(name, func(t *testing.T) {
			calls := 0
			protected, _, protectErr := Protect(source)
			if protectErr == nil {
				renderer, err := template.New("stack").Parse(protected)
				require.NoError(t, err)
				var output bytes.Buffer
				err = renderer.Execute(&output, structuralKeyData(func() string {
					calls++
					return "must-not-run"
				}))
				require.NoError(t, err)
			}
			require.Zero(t, calls, "structural key templates must be rejected before credential helpers execute")
			require.ErrorIs(t, protectErr, errUtils.ErrInvalidAuthConfig)
			require.Empty(t, protected)
		})
	}
}

func TestProtectPreservesDynamicLogicalNames(t *testing.T) {
	for _, quoted := range []bool{false, true} {
		name := "{{ .component }}"
		identity := "{{ .identity }}"
		if quoted {
			name, identity = "'"+name+"'", "'"+identity+"'"
		}
		source := "shared: &empty {}\nleft: &left {<<: *empty}\nright: &right {<<: *empty}\ncomponents:\n  terraform:\n    " + name + ":\n      <<: [*left, *right]\n      auth:\n        identities:\n          " + identity + ":\n            spec:\n              env: {TOKEN: '{{ call .privileged }}'}\n      vars:\n        '{{ .dataKey }}': {auth: {identities: {app: {spec: {env: {TOKEN: '{{ .stage }}'}}}}}}\n      settings:\n        '{{ .dataKey }}': {auth: {identities: {app: {spec: {env: {TOKEN: '{{ .stage }}'}}}}}}\n"
		protected, restore, err := Protect(source)
		require.NoError(t, err)
		require.NotContains(t, protected, "call .privileged")
		require.Equal(t, source, restore(protected))
		calls := 0
		renderer, err := template.New("stack").Parse(protected)
		require.NoError(t, err)
		var output bytes.Buffer
		data := structuralKeyData(func() string { calls++; return "must-not-run" })
		err = renderer.Execute(&output, data)
		require.NoError(t, err)
		require.Zero(t, calls)
		var document map[string]any
		require.NoError(t, yaml.Unmarshal([]byte(restore(output.String())), &document))
		component := document["components"].(map[string]any)["terraform"].(map[string]any)["example"].(map[string]any)
		credential := component["auth"].(map[string]any)["identities"].(map[string]any)["service"].(map[string]any)["spec"].(map[string]any)["env"].(map[string]any)
		require.Equal(t, "{{ call .privileged }}", credential["TOKEN"])
		for _, section := range []string{"vars", "settings"} {
			value := component[section].(map[string]any)["application"].(map[string]any)["auth"].(map[string]any)["identities"].(map[string]any)["app"].(map[string]any)["spec"].(map[string]any)["env"].(map[string]any)
			require.Equal(t, "prod", value["TOKEN"])
		}
	}
}

func TestProtectHandlesMultilineControlBeforeAuth(t *testing.T) {
	source := "{{ if\n.enabled }}auth:\n  identities:\n    service:\n      spec:\n        env: {TOKEN: '{{ call .privileged }}'}\n{{ end }}\n"
	protected, restore, err := Protect(source)
	if err != nil {
		require.ErrorIs(t, err, errUtils.ErrInvalidAuthConfig)
		return
	}
	require.NotContains(t, protected, "call .privileged")
	require.Equal(t, source, restore(protected))
	calls := 0
	renderer, err := template.New("stack").Parse(protected)
	require.NoError(t, err)
	var output bytes.Buffer
	require.NoError(t, renderer.Execute(&output, structuralKeyData(func() string { calls++; return "must-not-run" })))
	require.Zero(t, calls)
}

func structuralKeyData(privileged func() string) map[string]any {
	return map[string]any{"privileged": privileged, "enabled": true, "authName": "auth", "identitiesName": "identities", "specName": "spec", "envName": "env", "authPrefix": "au", "identPrefix": "ident", "specPrefix": "sp", "envPrefix": "en", "component": "example", "identity": "service", "dataKey": "application", "stage": "prod"}
}

func structuralKeySources(credential string) map[string]string {
	base := "auth: {identities: {service: {spec: {env: {TOKEN: '" + credential + "'}}}}}"
	result := make(map[string]string)
	for name, key := range map[string]struct{ from, to string }{
		"partial auth":               {"auth:", `'{{ "au" }}th':`},
		"invalid quoted auth":        {"auth:", `"{{ "au" }}th":`},
		"invalid quoted identities":  {"identities:", `"{{ "ident" }}ities":`},
		"partial identities":         {"identities:", `'{{ "ident" }}ities':`},
		"partial spec":               {"spec:", `'s{{ "pe" }}c':`},
		"partial env":                {"env:", `'e{{ "n" }}v':`},
		"multiple auth segments":     {"auth:", `'{{ "a" }}{{ "u" }}th':`},
		"multiple identity segments": {"identities:", `'{{ "ide" }}nt{{ "ities" }}':`},
		"escaped auth":               {"auth:", `"{{ .authPrefix }}t\u0068":`},
		"escaped identities":         {"identities:", `"{{ .identPrefix }}iti\u0065s":`},
		"escaped spec":               {"spec:", `"{{ .specPrefix }}\u0065c":`},
		"escaped env":                {"env:", `"{{ .envPrefix }}\u0076":`},
		"unquoted auth":              {"auth:", ` {{ .authName }}:`},
		"unquoted identities":        {"identities:", ` {{ .identitiesName }}:`},
		"unquoted spec":              {"spec:", ` {{ .specName }}:`},
		"unquoted env":               {"env:", ` {{ .envName }}:`},
	} {
		result[name] = strings.Replace(base, key.from, key.to, 1)
	}
	shared := "shared: &cfg {identities: {service: {spec: {env: {TOKEN: '" + credential + "'}}}}}\n"
	result["aliased root"] = shared + `'{{ "au" }}th': *cfg`
	result["alias key"] = `keyname: &k '{{ "au" }}th'` + "\n*k: " + strings.TrimPrefix(base, "auth: ")
	result["URI-tagged key"] = `!<tag:yaml.org,2002:str> '{{ "au" }}th': ` + strings.TrimPrefix(base, "auth: ")
	result["merged root"] = shared + `'{{ "au" }}th': {<<: *cfg}`
	result["merge sequence root"] = shared + `'{{ "au" }}th': {<<: [*cfg]}`
	all := strings.Replace(base, "auth:", `'{{ "au" }}th':`, 1)
	all = strings.Replace(all, "identities:", `'{{ "ident" }}ities':`, 1)
	all = strings.Replace(all, "spec:", `'s{{ "pe" }}c':`, 1)
	result["all structural keys"] = strings.Replace(all, "env:", `'e{{ "n" }}v':`, 1)
	result["cyclic merge"] = "shared: &cfg {<<: *cfg, identities: {service: {spec: {env: {TOKEN: '" + credential + "'}}}}}\n" + `'{{ "au" }}th': *cfg`
	result["cyclic alias under application data"] = "vars:\n  shared: &cfg {<<: *cfg, identities: {service: {spec: {env: {TOKEN: '" + credential + "'}}}}}\n" + `'{{ "au" }}th': *cfg`
	return result
}

// The prefilter is a routing contract: ordinary value properties must not make
// every non-auth template pay for YAML inspection.
func TestAuthPrefilterIgnoresOrdinaryYAMLValueProperties(t *testing.T) {
	for name, input := range map[string]string{
		"URI value tag":                  `value: !<tag:yaml.org,2002:str> '{{ .stage }}'`,
		"short value tag":                `value: !!str '{{ .stage }}'`,
		"value anchor and alias":         "value: &shared '{{ .stage }}'\ncopy: *shared\n",
		"colon in value action argument": `timestamp: {{ now | date "2006-01-02T15:04:05Z07:00" }}`,
	} {
		t.Run(name, func(t *testing.T) {
			require.False(t, mayContainAuth(input), "value syntax must not impersonate a templated mapping key")
		})
	}
}

func TestProtectRendersOrdinaryTemplateExpressions(t *testing.T) {
	for name, test := range map[string]struct {
		source string
		want   string
	}{
		"stack name":                            {`{{ .tenant }}-{{ .stage }}`, "acme-prod"},
		"variable assignment":                   {`{{ $value := . }}{{ $value.stage }}`, "prod"},
		"colon in action string":                {`{{ printf "%s: %s" "name" .stage }}`, "name: prod"},
		"non-output action before ordinary key": {`{{ printf "%s" "" }}Project: {{ .stage }}`, "Project: prod"},
		"dynamic application key": {`vars:
  {{ if true }}value: 'literal'{{ end }}
  {{ printf "dynamic" }}: {{ printf "%q" "rendered" }}
`, "vars:\n  value: 'literal'\n  dynamic: \"rendered\"\n"},
		"generated document": {`{{ .generated }}`, "vars: {stage: prod}"},
		"generated literal": {`vars:
  value: {{ printf "!literal %q" "{{external.email}}" }}
`, "vars:\n  value: !literal \"{{external.email}}\"\n"},
	} {
		t.Run(name, func(t *testing.T) {
			protected, restore, err := Protect(test.source)
			require.NoError(t, err)
			require.Equal(t, test.source, restore(protected))
			renderer, err := template.New("ordinary").Parse(protected)
			require.NoError(t, err)
			var output bytes.Buffer
			err = renderer.Execute(&output, map[string]any{"tenant": "acme", "stage": "prod", "generated": "vars: {stage: prod}"})
			require.NoError(t, err)
			require.Equal(t, test.want, restore(output.String()))
		})
	}
}

func TestProtectRejectsMalformedTemplatedAuthPaths(t *testing.T) {
	for name, source := range structuralKeySources("{{ call .privileged }}") {
		t.Run(name, func(t *testing.T) {
			protected, _, err := Protect(source + "\ninvalid: [unterminated\n")
			require.ErrorIs(t, err, errUtils.ErrInvalidAuthConfig)
			require.Empty(t, protected, "malformed credential declarations must never reach general rendering")
		})
	}
}

func TestProtectPreservesNonYAMLExpressionsWithoutTemplateKeys(t *testing.T) {
	for name, expression := range map[string]string{
		"variable assignment":      `{{ $value := . }}{{ $value.stage }}`,
		"colon in quoted argument": `{{ printf "%s: %s" "name" .stage }}`,
	} {
		t.Run(name, func(t *testing.T) {
			source := expression + "\n---\n[unterminated"
			protected, restore, err := Protect(source)
			require.NoError(t, err)
			require.Equal(t, source, restore(protected))
			renderer, err := template.New("text").Parse(protected)
			require.NoError(t, err)
			var output bytes.Buffer
			require.NoError(t, renderer.Execute(&output, map[string]any{"stage": "prod"}))
			want := "prod"
			if name == "colon in quoted argument" {
				want = "name: prod"
			}
			require.Equal(t, want+"\n---\n[unterminated", restore(output.String()))
		})
	}
}

func TestProtectDistinguishesGeneratedValuesFromStructuralKeys(t *testing.T) {
	for name, test := range map[string]struct {
		source string
		reject bool
	}{
		"whole-value identity generator": {source: `auth:
  identities:
    original: {spec: {env: {TOKEN: '{{ call .privileged }}'}}}
    generated: {{ .generated }}
vars: {stage: '{{ .stage }}'}
`},
		"action followed by structural key colon": {source: `auth:
  {{ .identitiesName }}:
    service:
      spec:
        env: {TOKEN: '{{ call .privileged }}'}
`, reject: true},
	} {
		t.Run(name, func(t *testing.T) {
			calls := 0
			protected, restore, protectErr := Protect(test.source)
			var output bytes.Buffer
			if protectErr == nil {
				require.Equal(t, test.source, restore(protected))
				renderer, err := template.New("stack").Parse(protected)
				require.NoError(t, err)
				data := structuralKeyData(func() string { calls++; return "must-not-run" })
				data["generated"] = "{spec: {env: {LATER: '{{ call .privileged }}'}}}"
				require.NoError(t, renderer.Execute(&output, data))
			}
			require.Zero(t, calls, "credential helpers must remain deferred across the generator/key boundary")
			if test.reject {
				require.ErrorIs(t, protectErr, errUtils.ErrInvalidAuthConfig)
				require.Empty(t, protected)
				return
			}
			require.NoError(t, protectErr)
			var document map[string]any
			require.NoError(t, yaml.Unmarshal([]byte(restore(output.String())), &document))
			identities := document["auth"].(map[string]any)["identities"].(map[string]any)
			original := identities["original"].(map[string]any)["spec"].(map[string]any)["env"].(map[string]any)
			generated := identities["generated"].(map[string]any)["spec"].(map[string]any)["env"].(map[string]any)
			require.Equal(t, "{{ call .privileged }}", original["TOKEN"])
			require.Equal(t, "{{ call .privileged }}", generated["LATER"])
			require.Equal(t, "prod", document["vars"].(map[string]any)["stage"])
		})
	}
}

func TestProtectRendersApplicationDataWithNonEnvironmentIdentity(t *testing.T) {
	source := `auth: {identities: {service: {kind: aws/user}}}
vars:
  {{ if true }}value: '{{ call .ordinary }}'{{ end }}
`
	protected, restore, err := Protect(source)
	require.NoError(t, err)
	require.Equal(t, source, restore(protected))
	renderer, err := template.New("stack").Parse(protected)
	require.NoError(t, err)
	calls := 0
	var output bytes.Buffer
	require.NoError(t, renderer.Execute(&output, map[string]any{"ordinary": func() string {
		calls++
		return "prod"
	}}))
	require.Equal(t, 1, calls, "ordinary application functions must still execute beside non-env identities")
	var document map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(restore(output.String())), &document))
	identity := document["auth"].(map[string]any)["identities"].(map[string]any)["service"].(map[string]any)
	require.Equal(t, "aws/user", identity["kind"])
	require.Equal(t, "prod", document["vars"].(map[string]any)["value"])
}
