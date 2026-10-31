package exec

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestAuthEnvironmentGeneratedTemplatesStayDeferredAcrossPasses(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token":"ordinary-helper-result"}`))
	}))
	defer server.Close()
	helper := fmt.Sprintf(`{{ (ds %q).token }}`, server.URL)
	config := &schema.AtmosConfiguration{Templates: schema.Templates{Settings: schema.TemplatesSettings{
		Enabled: true, Evaluations: 1, Gomplate: schema.TemplatesSettingsGomplate{Enabled: true},
	}}}
	control, err := ProcessTmplWithDatasources(config, &schema.ConfigAndStacksInfo{}, schema.Settings{}, "control.yaml", "vars: {value: '"+helper+"'}", map[string]any{}, false)
	require.NoError(t, err)
	require.Contains(t, control, "ordinary-helper-result")
	require.Positive(t, calls.Load())
	config.Templates.Settings.Evaluations = 3
	for name, generated := range map[string]string{
		"inline": "{spec: {env: {GENERATED: '" + helper + "'}}}",
		"block":  "\n      spec:\n        env:\n          GENERATED: |-\n            " + helper + "\n",
	} {
		t.Run(name, func(t *testing.T) {
			calls.Store(0)
			source := "auth:\n  identities:\n    original:\n      spec:\n        env:\n          ORIGINAL: '{{ .credentials.original }}'\n    generated: {{ .generated }}\nvars:\n  stage: '{{ .ordinary }}'\n"
			output, err := ProcessTmplWithDatasources(config, &schema.ConfigAndStacksInfo{}, schema.Settings{}, "generated.yaml", source, map[string]any{
				"generated": generated, "ordinary": "{{ .stage }}", "stage": "prod",
			}, false)
			require.NoError(t, err)
			require.Zero(t, calls.Load(), "generated credentials must be protected before the next evaluation pass")
			var document map[string]any
			require.NoError(t, yaml.Unmarshal([]byte(output), &document))
			identities := document["auth"].(map[string]any)["identities"].(map[string]any)
			original := identities["original"].(map[string]any)["spec"].(map[string]any)["env"].(map[string]any)
			later := identities["generated"].(map[string]any)["spec"].(map[string]any)["env"].(map[string]any)
			require.Equal(t, "{{ .credentials.original }}", original["ORIGINAL"])
			require.Equal(t, helper, later["GENERATED"])
			require.Equal(t, "prod", document["vars"].(map[string]any)["stage"])
		})
	}
}

func TestAuthEnvironmentGeneratedInvalidShapesFailBeforeNextPass(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token":"must-not-run"}`))
	}))
	defer server.Close()
	helper := fmt.Sprintf(`{{ (ds %q).token }}`, server.URL)
	config := &schema.AtmosConfiguration{Templates: schema.Templates{Settings: schema.TemplatesSettings{
		Enabled: true, Evaluations: 2, Gomplate: schema.TemplatesSettingsGomplate{Enabled: true},
	}}}
	output, err := ProcessTmplWithDatasources(config, &schema.ConfigAndStacksInfo{}, schema.Settings{}, "generated.yaml", "auth: {identities: {generated: {{ .generated }}}}", map[string]any{
		"generated": "{spec: {env: {TOKEN: {nested: '" + helper + "'}}}}",
	}, false)
	require.Zero(t, calls.Load())
	require.ErrorIs(t, err, errUtils.ErrInvalidAuthConfig)
	require.Empty(t, output)
}

func TestAuthEnvironmentWholeDocumentGeneratedBetweenPasses(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token":"must-not-run"}`))
	}))
	defer server.Close()
	helper := fmt.Sprintf(`{{ (ds %q).token }}`, server.URL)
	config := &schema.AtmosConfiguration{Templates: schema.Templates{Settings: schema.TemplatesSettings{
		Enabled: true, Evaluations: 3, Gomplate: schema.TemplatesSettingsGomplate{Enabled: true},
	}}}
	output, err := ProcessTmplWithDatasources(config, &schema.ConfigAndStacksInfo{}, schema.Settings{}, "generated.yaml", "{{ .authYaml }}", map[string]any{
		"authYaml": "auth: {identities: {service: {spec: {env: {TOKEN: '" + helper + "'}}}}}\nvars: {stage: '{{ .ordinary }}'}",
		"ordinary": "{{ .stage }}", "stage": "prod",
	}, false)
	require.NoError(t, err)
	require.Zero(t, calls.Load())
	require.Contains(t, output, "TOKEN: '"+helper+"'")
	require.Contains(t, output, "stage: 'prod'")
}

func TestAuthEnvironmentApplicationVariablesRenderNormally(t *testing.T) {
	config := &schema.AtmosConfiguration{Templates: schema.Templates{Settings: schema.TemplatesSettings{Enabled: true}}}
	source := "vars: {auth: {identities: {application: {spec: {env: {TOKEN: '{{ .stage }}'}}}}}}\nauth: {identities: {service: {spec: {env: {TOKEN: '{{ .credentials.token }}'}}}}}"
	for name, render := range authEnvironmentTemplateRenderers(config) {
		t.Run(name, func(t *testing.T) {
			output, err := render(source, map[string]any{"stage": "prod"})
			require.NoError(t, err)
			require.Contains(t, output, "TOKEN: 'prod'")
			require.Contains(t, output, "TOKEN: '{{ .credentials.token }}'")
		})
	}
}

func TestAuthEnvironmentRenderedPlaceholderLookalikesStayLiteral(t *testing.T) {
	config := &schema.AtmosConfiguration{Templates: schema.Templates{Settings: schema.TemplatesSettings{Enabled: true}}}
	source := "vars: {value: '{{ .value }}'}\nauth: {identities: {service: {spec: {env: {TOKEN: '{{ .credentials.token }}'}}}}}"
	for name, render := range authEnvironmentTemplateRenderers(config) {
		t.Run(name, func(t *testing.T) {
			output, err := render(source, map[string]any{"value": "ATMOS_AUTH_ENV_TEMPLATE_0_END"})
			require.NoError(t, err)
			require.Contains(t, output, "value: 'ATMOS_AUTH_ENV_TEMPLATE_0_END'")
			require.Contains(t, output, "TOKEN: '{{ .credentials.token }}'")
		})
	}
}

func TestAuthEnvironmentConditionalApplicationDataRendersNormally(t *testing.T) {
	config := &schema.AtmosConfiguration{Templates: schema.Templates{Settings: schema.TemplatesSettings{Enabled: true}}}
	source := "vars:\n  payload: |-\n    {{ if .enabled }}auth: {identities: {app: {spec: {env: {TOKEN: '{{ .stage }}'}}}}}{{ end }}\nauth: {identities: {service: {spec: {env: {TOKEN: '{{ .credentials.token }}'}}}}}"
	for name, render := range authEnvironmentTemplateRenderers(config) {
		t.Run(name, func(t *testing.T) {
			output, err := render(source, map[string]any{"stage": "prod", "enabled": true})
			require.NoError(t, err)
			require.Contains(t, output, "TOKEN: 'prod'")
			require.Contains(t, output, "TOKEN: '{{ .credentials.token }}'")
		})
	}
}
