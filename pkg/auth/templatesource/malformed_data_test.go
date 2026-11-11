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

func TestProtectRendersMalformedAuthShapedApplicationData(t *testing.T) {
	action := `{{ printf "%s" (call .ordinary) }}`
	for name, source := range malformedApplicationSources(action) {
		t.Run(name, func(t *testing.T) {
			var raw yaml.Node
			require.Error(t, yaml.Unmarshal([]byte(source), &raw), "raw template must exercise the malformed-YAML fallback")
			_, parseErr := template.New("raw-template").Parse(source)
			require.NoError(t, parseErr, "raw source must remain valid Go template syntax")
			protected, restore, err := Protect(source)
			require.NoError(t, err)
			require.Equal(t, source, protected, "application templates must remain available to ordinary rendering")
			require.Equal(t, source, restore(protected))
			renderer, err := template.New("application").Parse(protected)
			require.NoError(t, err)
			calls := 0
			var output bytes.Buffer
			require.NoError(t, renderer.Execute(&output, map[string]any{"ordinary": func() string { calls++; return "application" }}))
			require.Equal(t, 1, calls)
			var document map[string]any
			require.NoError(t, yaml.Unmarshal([]byte(restore(output.String())), &document))
			require.Contains(t, restore(output.String()), `TOKEN: "application"`)
		})
	}
}

func TestProtectRejectsMalformedOwnedCredentialAction(t *testing.T) {
	source := `auth: {identities: {service: {spec: {env: {TOKEN: "{{ printf "%s" (call .ordinary) }}"}}}}}`
	calls := 0
	protected, _, protectErr := Protect(source)
	if protectErr == nil {
		renderer, err := template.New("credentials").Parse(protected)
		require.NoError(t, err)
		var output bytes.Buffer
		require.NoError(t, renderer.Execute(&output, map[string]any{"ordinary": func() string { calls++; return "must-not-run" }}))
	}
	require.Zero(t, calls, "actual auth must still fail closed when raw YAML is malformed")
	require.ErrorIs(t, protectErr, errUtils.ErrInvalidAuthConfig)
	require.Empty(t, protected)
}

func malformedApplicationSources(action string) map[string]string {
	data := `{auth: {identities: {application: {spec: {env: {TOKEN: "` + action + `"}}}}}}`
	result := make(map[string]string)
	for _, section := range []string{"vars", "settings"} {
		result["root "+section] = section + ": " + data + "\n"
		result["component "+section] = "components: {terraform: {example: {" + section + ": " + data + "}}}\n"
	}
	return result
}

func TestProtectRendersConditionalApplicationConfig(t *testing.T) {
	application := "{auth: {identities: {app: {spec: {env: {TOKEN: '{{ call .ordinary }}'}}}}}}"
	source := "{{ if .enabled }}config: {vars: " + application + "}{{ end }}\nvars: {stage: prod}\n"
	for name, enabled := range map[string]bool{"enabled": true, "disabled": false} {
		t.Run(name, func(t *testing.T) {
			protected, restore, err := Protect(source)
			require.NoError(t, err)
			require.Equal(t, source, restore(protected))
			renderer, err := template.New("application").Parse(protected)
			require.NoError(t, err)
			calls := 0
			var output bytes.Buffer
			require.NoError(t, renderer.Execute(&output, map[string]any{"enabled": enabled, "ordinary": func() string { calls++; return "application" }}))
			var document map[string]any
			require.NoError(t, yaml.Unmarshal([]byte(restore(output.String())), &document))
			require.Equal(t, "prod", document["vars"].(map[string]any)["stage"])
			if enabled {
				require.Equal(t, 1, calls)
				require.Contains(t, output.String(), "TOKEN: 'application'")
			} else {
				require.Zero(t, calls)
				require.NotContains(t, document, "config")
			}
		})
	}
}

func TestProtectExplainsAmbiguousApplicationTemplates(t *testing.T) {
	for _, section := range []string{"vars", "settings"} {
		t.Run(section, func(t *testing.T) {
			source := section + ":\n  '{{ .key }}': {{ .open }}\n    value: '{{ call .ordinary }}'\n"
			var raw yaml.Node
			require.Error(t, yaml.Unmarshal([]byte(source), &raw))
			renderer, err := template.New("raw").Parse(source)
			require.NoError(t, err)
			calls := 0
			data := map[string]any{"key": "application", "open": "", "ordinary": func() string { calls++; return "application" }}
			var preview bytes.Buffer
			require.NoError(t, renderer.Execute(&preview, data))
			require.Equal(t, 1, calls, "the ambiguous raw template would otherwise execute its ordinary helper")
			var document map[string]any
			require.NoError(t, yaml.Unmarshal(preview.Bytes(), &document), "rendering could produce valid YAML despite ambiguous raw ownership")
			calls = 0
			protected, _, protectErr := Protect(source)
			if protectErr == nil {
				guarded, err := template.New("guarded").Parse(protected)
				require.NoError(t, err)
				var output bytes.Buffer
				require.NoError(t, guarded.Execute(&output, data))
			}
			require.Zero(t, calls)
			require.ErrorIs(t, protectErr, errUtils.ErrInvalidAuthConfig)
			require.Empty(t, protected)
			diagnostics := protectErr.Error() + strings.Join(errUtils.AllDetails(protectErr), " ") + strings.Join(errUtils.AllHints(protectErr), " ")
			require.Contains(t, diagnostics, "ownership")
			require.True(t, errUtils.HasHint(protectErr, "valid raw YAML"))
			require.True(t, errUtils.HasHint(protectErr, "vars/settings"))
			require.NotContains(t, diagnostics, ".ordinary")

			valid := strings.Replace(source, "': {{ .open }}", "':", 1)
			protected, restore, err := Protect(valid)
			require.NoError(t, err)
			require.Equal(t, valid, restore(protected))
			renderer, err = template.New("valid-raw-yaml").Parse(protected)
			require.NoError(t, err)
			var output bytes.Buffer
			require.NoError(t, renderer.Execute(&output, data))
			require.Equal(t, 1, calls, "following the raw-YAML hint must preserve ordinary dynamic application keys")
			require.NoError(t, yaml.Unmarshal([]byte(restore(output.String())), &document))
			require.Equal(t, "application", document[section].(map[string]any)["application"].(map[string]any)["value"])
		})
	}
}
