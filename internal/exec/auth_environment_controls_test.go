package exec

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestAuthEnvironmentControlOnlyScalarsRejectBeforeHelpers(t *testing.T) {
	server, calls := structuralKeyServer(t)
	condition := fmt.Sprintf("(ds `%s`).token", server.URL)
	config := &schema.AtmosConfiguration{Templates: schema.Templates{Settings: schema.TemplatesSettings{Enabled: true, Gomplate: schema.TemplatesSettingsGomplate{Enabled: true}}}}
	for name, render := range authEnvironmentTemplateRenderers(config) {
		t.Run(name, func(t *testing.T) {
			for kind, source := range authControlOnlyInputs(condition) {
				t.Run(kind, func(t *testing.T) {
					calls.Store(0)
					output, err := render(source, map[string]any{})
					require.Zero(t, calls.Load(), "credential control conditions must not execute before authentication")
					require.ErrorIs(t, err, errUtils.ErrInvalidAuthConfig)
					require.Empty(t, output)
				})
			}
			for kind, value := range map[string]string{
				"complete quoted": "'{{ if " + condition + " }}value{{ end }}'",
				"complete block":  "|-\n            {{ if " + condition + " }}\n            value\n            {{ end }}",
			} {
				t.Run(kind, func(t *testing.T) {
					calls.Store(0)
					source := "auth:\n  identities:\n    service:\n      spec:\n        env:\n          TOKEN: " + value + "\n"
					output, err := render(source, map[string]any{})
					require.NoError(t, err)
					require.Zero(t, calls.Load())
					require.Contains(t, output, condition)
				})
			}
		})
	}
}

func TestAuthEnvironmentCommentKeysKeepOrdinaryTemplatesWorking(t *testing.T) {
	server, calls := structuralKeyServer(t)
	helper := fmt.Sprintf(`{{ (ds %q).token }}`, server.URL)
	config := &schema.AtmosConfiguration{Templates: schema.Templates{Settings: schema.TemplatesSettings{Enabled: true, Gomplate: schema.TemplatesSettingsGomplate{Enabled: true}}}}
	for name, render := range authEnvironmentTemplateRenderers(config) {
		t.Run(name, func(t *testing.T) {
			calls.Store(0)
			output, err := render("# '{{ .stage }}':\nvars: {{ .open }}\n  stage: prod\n", map[string]any{"stage": "prod", "open": ""})
			require.NoError(t, err)
			require.Zero(t, calls.Load())
			var document map[string]any
			require.NoError(t, yaml.Unmarshal([]byte(output), &document))
			require.Equal(t, "prod", document["vars"].(map[string]any)["stage"])
			for _, key := range []string{`'#{{ .authName }}':`, `prefix#{{ .authName }}:`} {
				calls.Store(0)
				source := key + " {identities: {service: {spec: {env: {TOKEN: '" + helper + "'}}}}}\ninvalid: [unterminated\n"
				output, err := render(source, map[string]any{"authName": "auth"})
				require.Zero(t, calls.Load())
				require.ErrorIs(t, err, errUtils.ErrInvalidAuthConfig)
				require.Empty(t, output)
			}
		})
	}
}

func authControlOnlyInputs(condition string) map[string]string {
	opener := "{{ if " + condition + " }}"
	prefix := "auth:\n  identities:\n    service:\n      spec:\n        env:\n          TOKEN: "
	return map[string]string{
		"prefixed plain":                      "# \uE000\n" + prefix + "prefix" + opener + "\n{{ end }}\n",
		"single quoted":                       prefix + "'" + opener + "'\n{{ end }}\n",
		"double quoted":                       prefix + "\"" + opener + "\"\n{{ end }}\n",
		"anchored value":                      prefix + "&control '" + opener + "'\n{{ end }}\n",
		"aliased value":                       "vars: {control: &control '" + opener + "'}\n" + prefix + "*control\n{{ end }}\n",
		"aliased tagged value after comments": "vars:\n  control: !!str # tag property\n    &control # anchor property\n    '" + opener + "'\n" + prefix + "*control\n{{ end }}\n",
		"promoted aliased value":              "vars:\n  control: &control '" + opener + "'\n  defaults: &defaults {auth: {identities: {service: {spec: {env: {TOKEN: *control}}}}}}\n<<: *defaults\n{{ end }}\n",
	}
}
