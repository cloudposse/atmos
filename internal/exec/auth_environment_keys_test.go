package exec

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestAuthEnvironmentTemplatedStructuralKeysRejectBeforeHelpers(t *testing.T) {
	server, calls := structuralKeyServer(t)
	helper := fmt.Sprintf(`{{ (ds %q).token }}`, server.URL)
	config := &schema.AtmosConfiguration{Templates: schema.Templates{Settings: schema.TemplatesSettings{Enabled: true, Gomplate: schema.TemplatesSettingsGomplate{Enabled: true}}}}
	for name, render := range authEnvironmentTemplateRenderers(config) {
		t.Run(name, func(t *testing.T) {
			t.Run("ordinary helper works", func(t *testing.T) {
				calls.Store(0)
				output, err := render("vars: {probe: '"+helper+"'}", structuralKeyRenderData())
				require.NoError(t, err)
				require.Contains(t, output, "general-value")
				require.Positive(t, calls.Load())
			})
			for kind, source := range structuralKeyInputs(helper) {
				t.Run(kind, func(t *testing.T) {
					calls.Store(0)
					output, err := render(source, structuralKeyRenderData())
					require.Zero(t, calls.Load(), "structural keys must be checked before any datasource helper executes")
					require.ErrorIs(t, err, errUtils.ErrInvalidAuthConfig)
					require.Empty(t, output)
				})
			}
		})
	}
}

func TestAuthEnvironmentDynamicLogicalNamesRemainSupported(t *testing.T) {
	server, calls := structuralKeyServer(t)
	helper := fmt.Sprintf(`{{ (ds %q).token }}`, server.URL)
	config := &schema.AtmosConfiguration{Templates: schema.Templates{Settings: schema.TemplatesSettings{Enabled: true, Gomplate: schema.TemplatesSettingsGomplate{Enabled: true}}}}
	for name, render := range authEnvironmentTemplateRenderers(config) {
		t.Run(name, func(t *testing.T) {
			source := "components:\n  terraform:\n    '{{ .component }}':\n      auth:\n        identities:\n          '{{ .identity }}':\n            spec:\n              env: {TOKEN: '" + helper + "'}\n      vars:\n        '{{ .dataKey }}': {auth: {identities: {app: {spec: {env: {TOKEN: '{{ .stage }}'}}}}}}\n      settings:\n        '{{ .dataKey }}': {auth: {identities: {app: {spec: {env: {TOKEN: '{{ .stage }}'}}}}}}\n"
			calls.Store(0)
			output, err := render(source, structuralKeyRenderData())
			require.NoError(t, err)
			require.Zero(t, calls.Load())
			var document map[string]any
			require.NoError(t, yaml.Unmarshal([]byte(output), &document))
			component := document["components"].(map[string]any)["terraform"].(map[string]any)["example"].(map[string]any)
			credential := component["auth"].(map[string]any)["identities"].(map[string]any)["service"].(map[string]any)["spec"].(map[string]any)["env"].(map[string]any)
			require.Equal(t, helper, credential["TOKEN"])
			for _, section := range []string{"vars", "settings"} {
				value := component[section].(map[string]any)["application"].(map[string]any)["auth"].(map[string]any)["identities"].(map[string]any)["app"].(map[string]any)["spec"].(map[string]any)["env"].(map[string]any)
				require.Equal(t, "prod", value["TOKEN"])
			}
		})
	}
}

func structuralKeyInputs(credential string) map[string]string {
	base := "auth: {identities: {service: {spec: {env: {TOKEN: '" + credential + "'}}}}}"
	result := make(map[string]string)
	for name, key := range map[string]struct{ from, to string }{
		"partial auth":              {"auth:", `'{{ "au" }}th':`},
		"invalid quoted auth":       {"auth:", `"{{ "au" }}th":`},
		"invalid quoted identities": {"identities:", `"{{ "ident" }}ities":`},
		"partial identities":        {"identities:", `'{{ "ident" }}ities':`},
		"partial spec":              {"spec:", `'s{{ "pe" }}c':`},
		"partial env":               {"env:", `'e{{ "n" }}v':`},
		"multiple segments":         {"auth:", `'{{ "a" }}{{ "u" }}th':`},
		"escaped auth":              {"auth:", `"{{ .authPrefix }}t\u0068":`},
		"escaped identities":        {"identities:", `"{{ .identPrefix }}iti\u0065s":`},
		"escaped spec":              {"spec:", `"{{ .specPrefix }}\u0065c":`},
		"escaped env":               {"env:", `"{{ .envPrefix }}\u0076":`},
		"unquoted auth":             {"auth:", ` {{ .authName }}:`},
		"unquoted identities":       {"identities:", ` {{ .identitiesName }}:`},
		"unquoted spec":             {"spec:", ` {{ .specName }}:`},
		"unquoted env":              {"env:", ` {{ .envName }}:`},
	} {
		result[name] = strings.Replace(base, key.from, key.to, 1)
	}
	result["datasource key"] = strings.Replace(base, "auth:", "'"+credential+"':", 1)
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
	return result
}

func structuralKeyRenderData() map[string]any {
	return map[string]any{"authName": "auth", "identitiesName": "identities", "specName": "spec", "envName": "env", "authPrefix": "au", "identPrefix": "ident", "specPrefix": "sp", "envPrefix": "en", "component": "example", "identity": "service", "dataKey": "application", "stage": "prod"}
}

func structuralKeyServer(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	calls := new(atomic.Int32)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token":"general-value"}`))
	}))
	t.Cleanup(server.Close)
	return server, calls
}
