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

func TestProtectPreservesCredentialTemplateSource(t *testing.T) {
	for name, source := range map[string]string{
		"escaped identity key":          `auth: {"ident\u0069ties": {service: {spec: {env: {TOKEN: '{{ call .privileged }}'}}}}}`,
		"escaped env key":               `auth: {identities: {service: {spec: {"\u0065nv": {TOKEN: '{{ call .privileged }}'}}}}}`,
		"escaped auth path":             `"\u0061uth": {"ident\u0069ties": {service: {"\u0073pec": {"\u0065nv": {TOKEN: '{{ call .privileged }}'}}}}}`,
		"single quotes":                 "auth: {identities: {service: {spec: {env: {TOKEN: 'Bearer ''quoted'' {{ call .privileged }}'}}}}}\n",
		"double quotes":                 `auth: {identities: {service: {spec: {env: {TOKEN: "Bearer \"quoted\" {{ call .privileged }}"}}}}}`,
		"malformed credential template": "auth:\n  identities:\n    service:\n      spec:\n        env:\n          TOKEN: '{{ call .privileged'\n",
		"unicode columns":               `auth: {identities: {service: {spec: {env: {Ü_TOKEN: '{{ call .privileged }}'}}}}}`,
		"plain with following scalar":   "auth:\n  identities:\n    service:\n      spec:\n        env:\n          TOKEN: Bearer {{ call .privileged }}\n          NUMBER: 42\nvars:\n  stage: '{{ .stage }}'\n",
		"multiline plain":               "auth:\n  identities:\n    service:\n      spec:\n        env:\n          TOKEN: Bearer\n            {{ call .privileged }}\n\nvars:\n  stage: '{{ .stage }}'\n",
		"literal block":                 "auth:\n  identities:\n    service:\n      spec:\n        env:\n          TOKEN: |-\n            {{ call .privileged }}\n            second line\nvars:\n  stage: '{{ .stage }}'\n",
		"folded block at EOF":           "auth:\n  identities:\n    service:\n      spec:\n        env:\n          TOKEN: >-\n            Bearer\n            {{ call .privileged }}",
		"placeholder collision":         "# ATMOS_AUTH_ENV_TEMPLATE_ ATMOS_AUTH_ENV_TEMPLATE_X\nauth:\n  identities:\n    service:\n      spec:\n        env:\n          TOKEN: '{{ call .privileged }}' # keep comment\n",
		"multiple identities":           "auth:\n  identities:\n    first:\n      spec:\n        env:\n          ONE: '{{ call .privileged }}'\n          TWO: '{{ call .privileged }}'\n    second:\n      spec:\n        env:\n          THREE: '{{ call .privileged }}'\n",
		"raw template controls":         "{{ if .enabled }}\ncomponents:\n  terraform:\n    example:\n      auth:\n        identities:\n          service:\n            spec:\n              env:\n                TOKEN: |-\n                  {{ if .credentials.token }}\n                  {{ call .privileged }}\n                  {{ end }}\n{{ end }}\n",
		"CRLF":                          "auth:\r\n  identities:\r\n    service:\r\n      spec:\r\n        env:\r\n          TOKEN: '{{ call .privileged }}'\r\n",
		"ordinary unquoted template":    "vars:\n  stage: {{ .stage }}\nauth:\n  identities:\n    service:\n      spec:\n        env:\n          TOKEN: '{{ call .privileged }}'\n",
	} {
		t.Run(name, func(t *testing.T) {
			protected, restore, err := Protect(source)
			require.NoError(t, err)
			require.NotContains(t, protected, "call .privileged")
			require.Equal(t, source, restore(protected), "source spelling must survive protection exactly")
			calls := 0
			renderer, err := template.New("stack").Parse(protected)
			require.NoError(t, err)
			var output bytes.Buffer
			err = renderer.Execute(&output, map[string]any{
				"enabled": true,
				"stage":   "prod",
				"privileged": func() string {
					calls++
					return "must-not-be-rendered"
				},
			})
			require.NoError(t, err)
			require.Zero(t, calls)
			restored := restore(output.String())
			require.Contains(t, restored, "call .privileged")
			if strings.Contains(source, "stage:") {
				require.Contains(t, restored, "prod", "ordinary templates must still render")
				require.NotContains(t, restored, ".stage", "ordinary templates must still render")
			}
		})
	}
}

func TestProtectLeavesUnownedAndMalformedSourceUnchanged(t *testing.T) {
	for name, source := range map[string]string{
		"shell with JSON auth":          `curl -d '{"auth": "{{ .token }}"}' https://example.invalid`,
		"shell without credential spec": `curl -d '{"auth": "{{ .token }}", "identities": ["users"], "env": "dev"}' https://example.invalid`,
		"shell with ordinary words":     `curl -d '{"auth": "{{ .token }}"}' https://example.invalid/identities/spec/env`,
		"no templates":                  "auth: {identities: [invalid",
		"plain text":                    "ordinary {{ .value }}",
		"missing auth":                  "identities: {env: '{{ .value }}'}",
		"non mapping auth":              "auth: '{{ .value }}'\nidentities: {}\nenv: {}",
		"non mapping identities":        "auth: {identities: [invalid], env: '{{ .value }}'}",
		"non mapping identity":          "auth: {identities: {service: 'env {{ .value }}'}}",
		"non string env":                "auth: {identities: {service: {spec: {env: {TOKEN: 42, FLAG: true, EMPTY: null}}}}}\n# {{ .value }}",
		"missing spec":                  "auth: {identities: {service: {kind: secrets/env}}}\n# {{ .value }}",
		"malformed unowned YAML":        "identities: [invalid\nenv: '{{ .value }}'",
		"literal value":                 "auth: {identities: {service: {spec: {env: {TOKEN: ordinary}}}}}\n# {{ .value }}",
	} {
		t.Run(name, func(t *testing.T) {
			protected, restore, err := Protect(source)
			require.NoError(t, err)
			require.Equal(t, source, protected)
			require.Equal(t, source, restore(protected))
		})
	}
}

func TestProtectRejectsTemplatedAnchorsAndAliases(t *testing.T) {
	for name, source := range map[string]string{
		"scalar outside identities": "shared: &value '{{ env \"MUST_NOT_RUN\" }}'\nauth:\n  identities:\n    service:\n      spec:\n        env:\n          TOKEN: *value\n",
		"scalar within env":         "auth:\n  identities:\n    service:\n      spec:\n        env:\n          TOKEN: &value '{{ .credentials.token }}'\n          COPY: *value\n",
		"env mapping":               "shared: &value {TOKEN: '{{ .credentials.token }}'}\nauth: {identities: {service: {spec: {env: *value}}}}",
		"spec mapping":              "shared: &value {env: {TOKEN: '{{ .credentials.token }}'}}\nauth: {identities: {service: {spec: *value}}}",
		"identity mapping":          "shared: &value {spec: {env: {TOKEN: '{{ .credentials.token }}'}}}\nauth: {identities: {service: *value}}",
		"identities mapping":        "shared: &value {service: {spec: {env: {TOKEN: '{{ .credentials.token }}'}}}}\nauth: {identities: *value}",
		"auth mapping":              "shared: &value {identities: {service: {spec: {env: {TOKEN: '{{ .credentials.token }}'}}}}}\nauth: *value",
		"env merge":                 "shared: &value {TOKEN: '{{ .credentials.token }}'}\nauth: {identities: {service: {spec: {env: {<<: *value}}}}}",
		"spec merge":                "shared: &value {env: {TOKEN: '{{ .credentials.token }}'}}\nauth: {identities: {service: {spec: {<<: *value}}}}",
		"identity merge":            "shared: &value {spec: {env: {TOKEN: '{{ .credentials.token }}'}}}\nauth: {identities: {service: {<<: *value}}}",
		"identities merge":          "shared: &value {service: {spec: {env: {TOKEN: '{{ .credentials.token }}'}}}}\nauth: {identities: {<<: *value}}",
		"auth merge":                "shared: &value {identities: {service: {spec: {env: {TOKEN: '{{ .credentials.token }}'}}}}}\nauth: {<<: *value}",
		"alias sequence":            "shared: &value ['{{ .credentials.token }}']\nauth: {identities: {service: {spec: {env: {TOKEN: *value}}}}}",
		"recursive mapping":         "shared: &value {SELF: *value, TOKEN: '{{ .credentials.token }}'}\nauth: {identities: {service: {spec: {env: {TOKEN: *value}}}}}",
	} {
		t.Run(name, func(t *testing.T) {
			protected, _, err := Protect(source)
			require.ErrorIs(t, err, errUtils.ErrInvalidAuthConfig)
			require.True(t, errUtils.HasHint(err, "inline"))
			diagnostics := err.Error() + strings.Join(errUtils.AllDetails(err), " ") + strings.Join(errUtils.AllHints(err), " ")
			require.NotContains(t, diagnostics, ".credentials.token", "errors must not include template contents")
			require.NotContains(t, diagnostics, "MUST_NOT_RUN")
			require.Empty(t, protected)
		})
	}
}

func TestProtectAllowsLiteralAliases(t *testing.T) {
	for name, source := range map[string]string{
		"scalar":             "shared: &value ordinary-literal\nauth: {identities: {service: {spec: {env: {TOKEN: *value}}}}}\nvars: '{{ .stage }}'",
		"env mapping":        "shared: &value {TOKEN: ordinary-literal}\nauth: {identities: {service: {spec: {env: *value}}}}\nvars: '{{ .stage }}'",
		"spec mapping":       "shared: &value {env: {TOKEN: ordinary-literal}}\nauth: {identities: {service: {spec: *value}}}\nvars: '{{ .stage }}'",
		"literal auth merge": "shared: &value {identities: {service: {spec: {env: {TOKEN: ordinary-literal}}}}}\nauth: {<<: *value}\nvars: '{{ .stage }}'",
		"literal spec merge": "shared: &value {env: {TOKEN: ordinary-literal}}\nauth: {identities: {service: {spec: {<<: *value}}}}\nvars: '{{ .stage }}'",
	} {
		t.Run(name, func(t *testing.T) {
			protected, restore, err := Protect(source)
			require.NoError(t, err)
			require.Equal(t, source, protected)
			require.Equal(t, source, restore(protected))
		})
	}
}

func TestProtectRejectsUnlocatableCredentialTemplates(t *testing.T) {
	for name, source := range map[string]string{
		"malformed escaped auth path": "\"\\u0061uth\":\n  \"ident\\u0069ties\":\n    service:\n      \"\\u0073pec\":\n        \"\\u0065nv\":\n          TOKEN: '{{ call .privileged }}'\ninvalid: [unterminated",
		"malformed YAML":              "auth:\n  identities:\n    service:\n      spec:\n        env:\n          TOKEN: '{{ call .privileged }}'\ninvalid: [unterminated",
		"tagged scalar":               "auth:\n  identities:\n    service:\n      spec:\n        env:\n          TOKEN: !custom '{{ call .privileged }}'",
		"explicit string tag":         "auth:\n  identities:\n    service:\n      spec:\n        env:\n          TOKEN: !!str '{{ call .privileged }}'",
	} {
		t.Run(name, func(t *testing.T) {
			protected, _, err := Protect(source)
			require.ErrorIs(t, err, errUtils.ErrInvalidAuthConfig)
			require.True(t, errUtils.HasHint(err, "inline"))
			require.Empty(t, protected)
		})
	}
}

func TestProtectRejectsMalformedUnrelatedKeyBeforeCredentialTemplate(t *testing.T) {
	source := `"unrelated\q": literal
auth:
  identities:
    service:
      spec:
        env:
          TOKEN: 'Bearer {{ .credentials.token }}'
`
	protected, _, err := Protect(source)
	require.ErrorIs(t, err, errUtils.ErrInvalidAuthConfig)
	require.Empty(t, protected)
	require.True(t, errUtils.HasHint(err, "inline"))
	diagnostics := err.Error() + strings.Join(errUtils.AllDetails(err), " ") + strings.Join(errUtils.AllHints(err), " ")
	require.NotContains(t, diagnostics, ".credentials.token", "errors must not include credential template contents")
	require.NotContains(t, diagnostics, "Bearer")
}

func TestProtectPreservesEveryYAMLDocument(t *testing.T) {
	source := "vars: '{{ .stage }}'\n---\nauth:\n  identities:\n    service:\n      spec:\n        env:\n          TOKEN: '{{ call .privileged }}'\n"
	protected, restore, err := Protect(source)
	require.NoError(t, err)
	require.NotContains(t, protected, "call .privileged")
	require.Contains(t, protected, "vars: '{{ .stage }}'")
	require.Equal(t, source, restore(protected))
}

func TestProtectRetainsFlowMappingSiblings(t *testing.T) {
	source := `auth: {identities: {service: {spec: {env: {TOKEN: '{{ .credentials.token }}', OTHER: literal}}}}}
vars: {stage: '{{ .stage }}'}`
	protected, restore, err := Protect(source)
	require.NoError(t, err)
	var document map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(protected), &document), "protected source must retain valid flow YAML")
	require.Contains(t, protected, "OTHER: literal")
	require.Contains(t, protected, "stage: '{{ .stage }}'")
	require.Equal(t, source, restore(protected))
}

// Invalid environment shapes must not execute templates before schema validation.
// Scalar literals and aliases remain supported; composite shapes cannot safely expose actions.
func TestProtectRejectsInvalidEnvironmentTemplateShapes(t *testing.T) {
	for name, env := range map[string]string{
		"mapping value":           "env: {TOKEN: {nested: '{{ call .privileged }}'}}",
		"sequence value":          "env: {TOKEN: ['{{ call .privileged }}']}",
		"sequence env":            "env: ['{{ call .privileged }}']",
		"scalar env":              "env: '{{ call .privileged }}'",
		"unquoted value":          "env: {TOKEN: {{ call .privileged }}}",
		"unquoted nested value":   "env: {TOKEN: {nested: {{ call .privileged }}}}",
		"unquoted env":            "env: {{ call .privileged }}",
		"unquoted key":            "env: { {{ call .privileged }}: literal}",
		"quoted key":              "env: {'{{ call .privileged }}': literal}",
		"numeric key":             "env: {1: '{{ call .privileged }}'}",
		"literal mapping value":   "env: {TOKEN: {nested: ordinary}}",
		"literal sequence value":  "env: {TOKEN: [ordinary]}",
		"literal sequence env":    "env: [ordinary]",
		"literal scalar env":      "env: ordinary",
		"recursive literal alias": "env: &value {TOKEN: *value}",
	} {
		t.Run(name, func(t *testing.T) {
			source := "auth: {identities: {service: {spec: {" + env + "}}}}\n# {{ .value }}"
			protected, _, err := Protect(source)
			require.ErrorIs(t, err, errUtils.ErrInvalidAuthConfig)
			require.True(t, errUtils.HasHint(err, "inline"))
			require.Empty(t, protected)
			diagnostics := err.Error() + strings.Join(errUtils.AllDetails(err), " ") + strings.Join(errUtils.AllHints(err), " ")
			require.NotContains(t, diagnostics, ".privileged", "errors must not include template contents")
		})
	}
}
