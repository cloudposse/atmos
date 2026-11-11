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

func TestAuthEnvironmentExplicitKeysRejectBeforeHelpers(t *testing.T) {
	server, calls := structuralKeyServer(t)
	helper := fmt.Sprintf(`{{ (ds %q).token }}`, server.URL)
	config := &schema.AtmosConfiguration{Templates: schema.Templates{Settings: schema.TemplatesSettings{Enabled: true, Gomplate: schema.TemplatesSettingsGomplate{Enabled: true}}}}
	for name, render := range authEnvironmentTemplateRenderers(config) {
		t.Run(name, func(t *testing.T) {
			t.Run("ordinary explicit key renders", func(t *testing.T) {
				calls.Store(0)
				output, err := render("? label\n: \"{{ printf \"%s\" \"prod\" }}\"\n", map[string]any{})
				require.NoError(t, err)
				require.Zero(t, calls.Load())
				var document map[string]any
				require.NoError(t, yaml.Unmarshal([]byte(output), &document))
				require.Equal(t, "prod", document["label"])
			})
			for kind, source := range explicitAuthInputs(helper) {
				t.Run(kind, func(t *testing.T) {
					calls.Store(0)
					output, err := render(source, map[string]any{})
					require.Zero(t, calls.Load(), "explicit structural keys must not bypass protection before datasource calls")
					require.ErrorIs(t, err, errUtils.ErrInvalidAuthConfig)
					require.Empty(t, output)
				})
			}
		})
	}
}

func TestAuthEnvironmentRootMergeOwnsOnlyPromotedAuth(t *testing.T) {
	server, calls := structuralKeyServer(t)
	helper := fmt.Sprintf(`{{ (ds %q).token }}`, server.URL)
	config := &schema.AtmosConfiguration{Templates: schema.Templates{Settings: schema.TemplatesSettings{Enabled: true, Gomplate: schema.TemplatesSettingsGomplate{Enabled: true}}}}
	for name, render := range authEnvironmentTemplateRenderers(config) {
		t.Run(name, func(t *testing.T) {
			for _, section := range []string{"vars", "settings"} {
				t.Run(section+" unpromoted helper works", func(t *testing.T) {
					calls.Store(0)
					output, err := render(section+": {auth: {identities: {app: {spec: {env: {TOKEN: '"+helper+"'}}}}}}", map[string]any{})
					require.NoError(t, err)
					require.Positive(t, calls.Load())
					require.Contains(t, output, "general-value")
				})
			}
			for kind, test := range promotedAuthInputs(helper) {
				t.Run(kind, func(t *testing.T) {
					calls.Store(0)
					output, err := render(test.source, map[string]any{"stage": "prod"})
					require.Zero(t, calls.Load(), "only promoted credential helpers must remain deferred")
					if test.reject {
						require.ErrorIs(t, err, errUtils.ErrInvalidAuthConfig)
						require.Empty(t, output)
						return
					}
					require.NoError(t, err)
					var document map[string]any
					require.NoError(t, yaml.Unmarshal([]byte(output), &document))
					env := document["auth"].(map[string]any)["identities"].(map[string]any)["service"].(map[string]any)["spec"].(map[string]any)["env"].(map[string]any)
					require.Equal(t, helper, env["TOKEN"])
					require.NotContains(t, output, "{{ .stage }}")
				})
			}
		})
	}
}

func explicitAuthInputs(credential string) map[string]string {
	base := "auth:\n  identities:\n    service:\n      spec:\n        env:\n          TOKEN: '" + credential + "'\nordinary: \"{{ printf \"%s\" \"prod\" }}\"\n"
	result := make(map[string]string)
	all := base
	for key, indent := range map[string]string{"auth": "", "identities": "  ", "spec": "      ", "env": "        "} {
		from, to := indent+key+":", indent+"? "+key+"\n"+indent+":"
		result[key] = strings.Replace(base, from, to, 1)
		all = strings.Replace(all, from, to, 1)
	}
	result["all explicit"] = all
	result["escaped explicit auth"] = strings.Replace(base, "auth:", "? \"\\u0061uth\"\n:", 1)
	return result
}

func promotedAuthInputs(credential string) map[string]struct {
	source string
	reject bool
} {
	result := make(map[string]struct {
		source string
		reject bool
	})
	for _, section := range []string{"vars", "settings"} {
		base := section + ":\n  base: &base\n    auth: {identities: {service: {spec: {env: {TOKEN: '" + credential + "'}}}}}\n    vars: {auth: {identities: {application: {spec: {env: {TOKEN: '{{ .stage }}'}}}}}}\n"
		result[section+" alias"] = struct {
			source string
			reject bool
		}{source: base + "<<: *base\n"}
		result[section+" repeated merge sequence"] = struct {
			source string
			reject bool
		}{source: base + "<<: [*base, *base]\n"}
		result[section+" merge chain"] = struct {
			source string
			reject bool
		}{source: base + "  next: &next {<<: *base}\n<<: *next\n"}
		result[section+" cyclic merge"] = struct {
			source string
			reject bool
		}{source: strings.Replace(base, "base: &base\n", "base: &base\n    <<: *base\n", 1) + "<<: *base\n", reject: true}
	}
	result["direct and merged ownership"] = struct {
		source string
		reject bool
	}{source: "defaults: &base\n  auth: {identities: {service: {spec: {env: {TOKEN: '" + credential + "'}}}}}\n  vars: {stage: '{{ .stage }}'}\n<<: *base\n"}
	return result
}

func TestAuthEnvironmentPromotedComponentsDeferBeforeHelpers(t *testing.T) {
	server, calls := structuralKeyServer(t)
	helper := fmt.Sprintf(`{{ (ds %q).token }}`, server.URL)
	config := &schema.AtmosConfiguration{Templates: schema.Templates{Settings: schema.TemplatesSettings{Enabled: true, Gomplate: schema.TemplatesSettingsGomplate{Enabled: true}}}}
	for name, render := range authEnvironmentTemplateRenderers(config) {
		t.Run(name, func(t *testing.T) {
			for kind, source := range componentPromotionInputs(helper) {
				t.Run(kind, func(t *testing.T) {
					calls.Store(0)
					output, err := render(source, map[string]any{"stage": "prod"})
					require.Zero(t, calls.Load(), "configuration references must not expose promoted component credentials to general helpers")
					require.NoError(t, err)
					var document map[string]any
					require.NoError(t, yaml.Unmarshal([]byte(output), &document))
					component := document["components"].(map[string]any)["terraform"].(map[string]any)["example"].(map[string]any)
					env := component["auth"].(map[string]any)["identities"].(map[string]any)["service"].(map[string]any)["spec"].(map[string]any)["env"].(map[string]any)
					require.Equal(t, helper, env["TOKEN"])
					require.Equal(t, "prod", component["vars"].(map[string]any)["stage"])
					require.NotContains(t, output, "{{ .stage }}")
				})
			}
		})
	}
}

func componentPromotionInputs(credential string) map[string]string {
	application := "{auth: {identities: {app: {spec: {env: {TOKEN: '{{ .stage }}'}}}}}}"
	component := "{auth: {identities: {service: {spec: {env: {TOKEN: '" + credential + "'}}}}}, vars: {stage: '{{ .stage }}', application: " + application + "}}"
	result := make(map[string]string)
	for _, section := range []string{"vars", "settings"} {
		result[section+" root merge"] = section + ":\n  bundle: &bundle {components: {terraform: {example: " + component + "}}}\n  ordinary: " + application + "\n<<: *bundle\n"
		result[section+" component-map alias"] = section + ":\n  components: &componentsAnchor {example: " + component + "}\n  ordinary: " + application + "\ncomponents: {terraform: *componentsAnchor}\n"
	}
	return result
}
