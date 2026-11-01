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

func TestProtectRejectsControlOnlyCredentialScalars(t *testing.T) {
	for name, source := range controlOnlySources("call .privileged") {
		t.Run(name, func(t *testing.T) {
			var declaration yaml.Node
			require.NoError(t, yaml.Unmarshal([]byte(strings.TrimSuffix(source, "{{ end }}\n")), &declaration), "credential declarations must be valid YAML before the external control end")
			calls := 0
			protected, _, protectErr := Protect(source)
			if protectErr == nil {
				renderer, err := template.New("stack").Parse(protected)
				require.NoError(t, err)
				var output bytes.Buffer
				require.NoError(t, renderer.Execute(&output, map[string]any{"privileged": func() bool { calls++; return true }}))
			}
			require.Zero(t, calls, "projection must not hide an executable credential control opener")
			require.ErrorIs(t, protectErr, errUtils.ErrInvalidAuthConfig)
			require.Empty(t, protected)
		})
	}
}

func TestProtectPreservesCompleteCredentialControls(t *testing.T) {
	for name, value := range map[string]string{
		"quoted": `'{{ if call .privileged }}value{{ end }}'`,
		"block":  "|-\n            {{ if call .privileged }}\n            value\n            {{ end }}",
	} {
		t.Run(name, func(t *testing.T) {
			source := "auth:\n  identities:\n    service:\n      spec:\n        env:\n          TOKEN: " + value + "\nvars: {stage: '{{ .stage }}'}\n"
			protected, restore, err := Protect(source)
			require.NoError(t, err)
			require.NotContains(t, protected, ".privileged")
			require.Equal(t, source, restore(protected))
			calls := 0
			renderer, err := template.New("stack").Parse(protected)
			require.NoError(t, err)
			var output bytes.Buffer
			require.NoError(t, renderer.Execute(&output, map[string]any{"stage": "prod", "privileged": func() bool { calls++; return true }}))
			require.Zero(t, calls)
			require.Contains(t, restore(output.String()), value)
		})
	}
}

func TestProtectIgnoresTemplateKeysInYAMLComments(t *testing.T) {
	body := "vars: {{ .open }}\n  stage: prod\n"
	for name, source := range map[string]string{
		"leading comment":               "# '{{ .stage }}':\n" + body,
		"final comment without newline": body + "# '{{ .stage }}':",
	} {
		t.Run(name, func(t *testing.T) {
			protected, restore, err := Protect(source)
			require.NoError(t, err)
			require.Equal(t, source, restore(protected))
			renderer, err := template.New("data").Parse(protected)
			require.NoError(t, err)
			var output bytes.Buffer
			require.NoError(t, renderer.Execute(&output, map[string]any{"stage": "prod", "open": ""}))
			var document map[string]any
			require.NoError(t, yaml.Unmarshal([]byte(restore(output.String())), &document))
			require.Equal(t, "prod", document["vars"].(map[string]any)["stage"])
		})
	}
}

func TestProtectKeepsQuotedAndAdjacentHashesInTemplateKeys(t *testing.T) {
	base := "auth: {identities: {service: {spec: {env: {TOKEN: '{{ call .privileged }}'}}}}}\ninvalid: [unterminated\n"
	for _, key := range []string{`'#{{ .authName }}':`, `prefix#{{ .authName }}:`} {
		source := strings.Replace(base, "auth:", key, 1)
		calls := 0
		protected, _, protectErr := Protect(source)
		if protectErr == nil {
			renderer, err := template.New("stack").Parse(protected)
			require.NoError(t, err)
			var output bytes.Buffer
			require.NoError(t, renderer.Execute(&output, map[string]any{"authName": "auth", "privileged": func() string { calls++; return "must-not-run" }}))
		}
		require.Zero(t, calls)
		require.ErrorIs(t, protectErr, errUtils.ErrInvalidAuthConfig)
		require.Empty(t, protected)
	}
}

func controlOnlySources(condition string) map[string]string {
	opener := "{{ if " + condition + " }}"
	prefix := "auth:\n  identities:\n    service:\n      spec:\n        env:\n          TOKEN: "
	return map[string]string{
		"block control opener":                prefix + "|-\n            prefix" + opener + "\n{{ end }}\n",
		"prefixed plain":                      "# \uE000\n" + prefix + "prefix" + opener + "\n{{ end }}\n",
		"single quoted":                       prefix + "'" + opener + "'\n{{ end }}\n",
		"double quoted":                       prefix + "\"" + opener + "\"\n{{ end }}\n",
		"anchored value":                      prefix + "&control '" + opener + "'\n{{ end }}\n",
		"aliased value":                       "vars: {control: &control '" + opener + "'}\n" + prefix + "*control\n{{ end }}\n",
		"aliased tagged value after comments": "vars:\n  control: !!str # tag property\n    &control # anchor property\n    '" + opener + "'\n" + prefix + "*control\n{{ end }}\n",
		"promoted aliased value":              "vars:\n  control: &control '" + opener + "'\n  defaults: &defaults {auth: {identities: {service: {spec: {env: {TOKEN: *control}}}}}}\n<<: *defaults\n{{ end }}\n",
	}
}
