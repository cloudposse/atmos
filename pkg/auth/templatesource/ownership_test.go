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

func TestProtectRejectsMalformedExplicitAuthKeys(t *testing.T) {
	for name, source := range explicitAuthSources("{{ call .privileged }}") {
		t.Run(name, func(t *testing.T) {
			calls := 0
			protected, _, protectErr := Protect(source)
			if protectErr == nil {
				renderer, err := template.New("stack").Parse(protected)
				require.NoError(t, err)
				var output bytes.Buffer
				require.NoError(t, renderer.Execute(&output, map[string]any{"privileged": func() string {
					calls++
					return "must-not-run"
				}}))
			}
			require.Zero(t, calls, "explicit auth keys must not hide credentials from malformed-source inspection")
			require.ErrorIs(t, protectErr, errUtils.ErrInvalidAuthConfig)
			require.Empty(t, protected)
		})
	}
}

func TestProtectDefersAuthPromotedByRootMerge(t *testing.T) {
	for name, test := range promotedAuthSources("{{ call .privileged }}") {
		t.Run(name, func(t *testing.T) {
			calls := 0
			protected, restore, protectErr := Protect(test.source)
			var output bytes.Buffer
			if protectErr == nil {
				require.Equal(t, test.source, restore(protected), "promoted credentials must restore exact source, including repeated aliases")
				renderer, err := template.New("stack").Parse(protected)
				require.NoError(t, err)
				require.NoError(t, renderer.Execute(&output, map[string]any{"stage": "prod", "privileged": func() string {
					calls++
					return "must-not-run"
				}}))
			}
			require.Zero(t, calls, "auth imported by a root merge must defer its credential helpers")
			if test.reject {
				require.ErrorIs(t, protectErr, errUtils.ErrInvalidAuthConfig)
				require.Empty(t, protected)
				return
			}
			require.NoError(t, protectErr)
			var document map[string]any
			restored := restore(output.String())
			require.NoError(t, yaml.Unmarshal([]byte(restored), &document))
			env := document["auth"].(map[string]any)["identities"].(map[string]any)["service"].(map[string]any)["spec"].(map[string]any)["env"].(map[string]any)
			require.Equal(t, "{{ call .privileged }}", env["TOKEN"])
			require.NotContains(t, restored, "{{ .stage }}", "unpromoted vars/settings data must still render normally")
		})
	}
}

func TestProtectRendersUnpromotedAuthData(t *testing.T) {
	for _, section := range []string{"vars", "settings"} {
		t.Run(section, func(t *testing.T) {
			source := section + ": {auth: {identities: {service: {spec: {env: {TOKEN: '{{ call .ordinary }}'}}}}}}"
			protected, restore, err := Protect(source)
			require.NoError(t, err)
			require.Equal(t, source, restore(protected))
			renderer, err := template.New("data").Parse(protected)
			require.NoError(t, err)
			calls := 0
			var output bytes.Buffer
			require.NoError(t, renderer.Execute(&output, map[string]any{"ordinary": func() string { calls++; return "application" }}))
			require.Equal(t, 1, calls)
			require.Contains(t, restore(output.String()), "TOKEN: 'application'")
		})
	}
}

func explicitAuthSources(credential string) map[string]string {
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

func promotedAuthSources(credential string) map[string]struct {
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
	cyclic := result["vars cyclic merge"].source
	result["cyclic merge sequence"] = struct {
		source string
		reject bool
	}{source: strings.TrimSuffix(cyclic, "<<: *base\n") + "<<: [*base]\n", reject: true}
	result["direct and merged ownership"] = struct {
		source string
		reject bool
	}{source: "defaults: &base\n  auth: {identities: {service: {spec: {env: {TOKEN: '" + credential + "'}}}}}\n  vars: {stage: '{{ .stage }}'}\n<<: *base\n"}
	return result
}

func TestProtectRendersMalformedExplicitApplicationKey(t *testing.T) {
	source := "? label\n: \"{{ printf \"%s\" \"prod\" }}\"\n"
	protected, restore, err := Protect(source)
	require.NoError(t, err)
	require.Equal(t, source, restore(protected))
	renderer, err := template.New("data").Parse(protected)
	require.NoError(t, err)
	var output bytes.Buffer
	require.NoError(t, renderer.Execute(&output, nil))
	var document map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(restore(output.String())), &document))
	require.Equal(t, "prod", document["label"])
}

func TestProtectDefersPromotedComponentAuth(t *testing.T) {
	credential := "{{ call .privileged }}"
	for name, source := range componentPromotionSources(credential) {
		t.Run(name, func(t *testing.T) {
			calls := 0
			protected, restore, err := Protect(source)
			require.NoError(t, err)
			require.Equal(t, source, restore(protected), "promotion must preserve the exact anchor/alias source")
			renderer, err := template.New("stack").Parse(protected)
			require.NoError(t, err)
			var output bytes.Buffer
			require.NoError(t, renderer.Execute(&output, map[string]any{"stage": "prod", "privileged": func() string {
				calls++
				return "must-not-run"
			}}))
			require.Zero(t, calls, "referenced component auth must be owned before general rendering")
			var document map[string]any
			restored := restore(output.String())
			require.NoError(t, yaml.Unmarshal([]byte(restored), &document))
			component := document["components"].(map[string]any)["terraform"].(map[string]any)["example"].(map[string]any)
			env := component["auth"].(map[string]any)["identities"].(map[string]any)["service"].(map[string]any)["spec"].(map[string]any)["env"].(map[string]any)
			require.Equal(t, credential, env["TOKEN"])
			require.Equal(t, "prod", component["vars"].(map[string]any)["stage"])
			require.NotContains(t, restored, "{{ .stage }}", "unpromoted auth-shaped application data must still render")
		})
	}
}

func componentPromotionSources(credential string) map[string]string {
	application := "{auth: {identities: {app: {spec: {env: {TOKEN: '{{ .stage }}'}}}}}}"
	component := "{auth: {identities: {service: {spec: {env: {TOKEN: '" + credential + "'}}}}}, vars: {stage: '{{ .stage }}', application: " + application + "}}"
	result := make(map[string]string)
	for _, section := range []string{"vars", "settings"} {
		result[section+" root merge"] = section + ":\n  bundle: &bundle {components: {terraform: {example: " + component + "}}}\n  ordinary: " + application + "\n<<: *bundle\n"
		result[section+" component-map alias"] = section + ":\n  components: &componentsAnchor {example: " + component + "}\n  ordinary: " + application + "\ncomponents: {terraform: *componentsAnchor}\n"
	}
	return result
}
