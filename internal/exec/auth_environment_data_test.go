package exec

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestAuthEnvironmentMalformedApplicationDataRendersNormally(t *testing.T) {
	server, calls := structuralKeyServer(t)
	action := fmt.Sprintf(`{{ printf "%%s" (ds %q).token }}`, server.URL)
	config := &schema.AtmosConfiguration{Templates: schema.Templates{Settings: schema.TemplatesSettings{Enabled: true, Gomplate: schema.TemplatesSettingsGomplate{Enabled: true}}}}
	for name, render := range authEnvironmentTemplateRenderers(config) {
		t.Run(name, func(t *testing.T) {
			for kind, source := range malformedApplicationInputs(action) {
				t.Run(kind, func(t *testing.T) {
					var raw yaml.Node
					require.Error(t, yaml.Unmarshal([]byte(source), &raw))
					calls.Store(0)
					output, err := render(source, map[string]any{})
					require.NoError(t, err)
					require.Positive(t, calls.Load(), "application data helpers must remain available outside credential ownership")
					var document map[string]any
					require.NoError(t, yaml.Unmarshal([]byte(output), &document))
					require.Contains(t, output, `TOKEN: "general-value"`)
				})
			}
			t.Run("actual auth rejects before helpers", func(t *testing.T) {
				calls.Store(0)
				source := `auth: {identities: {service: {spec: {env: {TOKEN: "` + action + `"}}}}}`
				output, err := render(source, map[string]any{})
				require.Zero(t, calls.Load())
				require.ErrorIs(t, err, errUtils.ErrInvalidAuthConfig)
				require.Empty(t, output)
			})
		})
	}
}

func malformedApplicationInputs(action string) map[string]string {
	data := `{auth: {identities: {application: {spec: {env: {TOKEN: "` + action + `"}}}}}}`
	result := make(map[string]string)
	for _, section := range []string{"vars", "settings"} {
		result["root "+section] = section + ": " + data + "\n"
		result["component "+section] = "components: {terraform: {example: {" + section + ": " + data + "}}}\n"
	}
	return result
}

func TestAuthEnvironmentConditionalApplicationConfigRendersNormally(t *testing.T) {
	server, calls := structuralKeyServer(t)
	helper := fmt.Sprintf(`{{ (ds %q).token }}`, server.URL)
	application := "{auth: {identities: {app: {spec: {env: {TOKEN: '" + helper + "'}}}}}}"
	source := "{{ if .enabled }}config: {vars: " + application + "}{{ end }}\nvars: {stage: prod}\n"
	config := &schema.AtmosConfiguration{Templates: schema.Templates{Settings: schema.TemplatesSettings{Enabled: true, Gomplate: schema.TemplatesSettingsGomplate{Enabled: true}}}}
	for name, render := range authEnvironmentTemplateRenderers(config) {
		t.Run(name, func(t *testing.T) {
			for label, enabled := range map[string]bool{"enabled": true, "disabled": false} {
				t.Run(label, func(t *testing.T) {
					calls.Store(0)
					output, err := render(source, map[string]any{"enabled": enabled})
					require.NoError(t, err)
					var document map[string]any
					require.NoError(t, yaml.Unmarshal([]byte(output), &document))
					require.Equal(t, "prod", document["vars"].(map[string]any)["stage"])
					if enabled {
						require.Positive(t, calls.Load())
						require.Contains(t, output, "TOKEN: 'general-value'")
					} else {
						require.Zero(t, calls.Load())
						require.NotContains(t, document, "config")
					}
				})
			}
		})
	}
}

func TestAuthEnvironmentAmbiguousApplicationTemplatesExplainRemedy(t *testing.T) {
	server, calls := structuralKeyServer(t)
	helper := fmt.Sprintf(`{{ (ds %q).token }}`, server.URL)
	config := &schema.AtmosConfiguration{Templates: schema.Templates{Settings: schema.TemplatesSettings{Enabled: true, Gomplate: schema.TemplatesSettingsGomplate{Enabled: true}}}}
	for name, render := range authEnvironmentTemplateRenderers(config) {
		t.Run(name, func(t *testing.T) {
			for _, section := range []string{"vars", "settings"} {
				t.Run(section, func(t *testing.T) {
					calls.Store(0)
					source := section + ":\n  '{{ .key }}': {{ .open }}\n    value: '" + helper + "'\n"
					output, err := render(source, map[string]any{"key": "application", "open": ""})
					require.Zero(t, calls.Load(), "ambiguous source must fail closed before any general helper executes")
					require.ErrorIs(t, err, errUtils.ErrInvalidAuthConfig)
					require.Empty(t, output)
					diagnostics := err.Error() + strings.Join(errUtils.AllDetails(err), " ") + strings.Join(errUtils.AllHints(err), " ")
					require.Contains(t, diagnostics, "ownership")
					require.True(t, errUtils.HasHint(err, "valid raw YAML"))
					require.True(t, errUtils.HasHint(err, "vars/settings"))
					require.NotContains(t, diagnostics, server.URL)

					valid := section + ":\n  '{{ .key }}':\n    value: '" + helper + "'\n"
					output, err = render(valid, map[string]any{"key": "application"})
					require.NoError(t, err)
					require.Positive(t, calls.Load(), "valid raw YAML must preserve ordinary dynamic keys and helpers")
					var document map[string]any
					require.NoError(t, yaml.Unmarshal([]byte(output), &document))
					require.Equal(t, "general-value", document[section].(map[string]any)["application"].(map[string]any)["value"])
				})
			}
		})
	}
}
