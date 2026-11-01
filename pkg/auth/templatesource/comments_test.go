package templatesource

import (
	"bytes"
	"testing"
	"text/template"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestProtectHandlesStandaloneTemplateComments(t *testing.T) {
	for name, comment := range map[string]string{
		"single line":                    "{{/* manifest comment */}}\n",
		"whitespace trim":                "{{- /* trimmed comment */ -}}\n",
		"multiline":                      "{{/* first line\nsecond line\n*/}}\n",
		"multiline trim with blank line": "{{- /* first line\n\nlast line */ -}}\n",
	} {
		t.Run(name, func(t *testing.T) {
			source := comment + "vars: {stage: '{{ .stage }}'}\nauth: {identities: {service: {spec: {env: {TOKEN: '{{ call .privileged }}'}}}}}"
			protected, restore, err := Protect(source)
			require.NoError(t, err)
			require.Equal(t, source, restore(protected))
			require.NotContains(t, protected, "call .privileged")
			calls := 0
			renderer, err := template.New("stack").Parse(protected)
			require.NoError(t, err)
			var output bytes.Buffer
			err = renderer.Execute(&output, map[string]any{"stage": "prod", "privileged": func() string {
				calls++
				return "must-not-run"
			}})
			require.NoError(t, err)
			require.Zero(t, calls)
			var document map[string]any
			require.NoError(t, yaml.Unmarshal([]byte(restore(output.String())), &document))
			require.Equal(t, "prod", document["vars"].(map[string]any)["stage"])
			credentials := document["auth"].(map[string]any)["identities"].(map[string]any)["service"].(map[string]any)["spec"].(map[string]any)["env"].(map[string]any)
			require.Equal(t, "{{ call .privileged }}", credentials["TOKEN"])
		})
	}
}

func TestProtectPreservesCommentsInsideCredentialScalars(t *testing.T) {
	for name, value := range map[string]string{
		"quoted": "'{{/* credential comment */}}{{ .credentials.token }}'",
		"block":  "|-\n            {{/* first line\n\n            last line */}}\n            {{ .credentials.token }}",
	} {
		t.Run(name, func(t *testing.T) {
			source := "auth:\n  identities:\n    service:\n      spec:\n        env:\n          TOKEN: " + value + "\n"
			protected, restore, err := Protect(source)
			require.NoError(t, err)
			require.NotContains(t, protected, ".credentials.token")
			require.Equal(t, source, restore(protected), "credential comments must retain their exact source")
		})
	}
}

func TestProtectRetainsCommentMarkersForAliases(t *testing.T) {
	source := "shared: &value |-\n  {{/* first line\n\n  last line */}}\nauth: {identities: {service: {spec: {env: {TOKEN: *value}}}}}"
	protected, _, err := Protect(source)
	require.ErrorIs(t, err, errUtils.ErrInvalidAuthConfig)
	require.True(t, errUtils.HasHint(err, "inline"))
	require.Empty(t, protected)
}

func TestProtectRejectsLiteralAuthAfterLeadingActions(t *testing.T) {
	for name, prefix := range map[string]string{
		"comment":     "{{/* comment */}}",
		"empty value": `{{ "" }}`,
	} {
		t.Run(name, func(t *testing.T) {
			source := prefix + "auth:\n  identities:\n    service:\n      spec:\n        env: {TOKEN: '{{ call .privileged }}'}\n"
			calls := 0
			protected, _, protectErr := Protect(source)
			if protectErr == nil {
				renderer, err := template.New("stack").Parse(protected)
				require.NoError(t, err)
				var output bytes.Buffer
				err = renderer.Execute(&output, map[string]any{"privileged": func() string {
					calls++
					return "must-not-run"
				}})
				require.NoError(t, err)
			}
			require.Zero(t, calls, "literal auth keys after leading actions must fail before helpers run")
			require.ErrorIs(t, protectErr, errUtils.ErrInvalidAuthConfig)
			require.Empty(t, protected)
		})
	}
}
