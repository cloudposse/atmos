package kubernetes

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestRenderManifestInputTemplates(t *testing.T) {
	componentSection := map[string]any{
		config.VarsSectionName: map[string]any{
			"overlay":   "dev",
			"name":      "app",
			"replicas":  2,
			"namespace": "demo",
		},
		config.PathsSectionName: []any{"overlays/{{ .vars.overlay }}"},
		config.ManifestsSectionName: []any{
			`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: "{{ .vars.name }}"
  namespace: "{{ .vars.namespace }}"
spec:
  replicas: {{ .vars.replicas }}
`,
			map[string]any{
				"apiVersion": "v1",
				"kind":       "Namespace",
				"metadata": map[string]any{
					"name": "{{ .vars.namespace }}",
				},
			},
		},
	}

	require.NoError(t, renderManifestInputTemplates(&schema.AtmosConfiguration{}, componentSection))

	require.Equal(t, []any{"overlays/dev"}, componentSection[config.PathsSectionName])

	manifests := componentSection[config.ManifestsSectionName].([]any)
	require.Contains(t, manifests[0].(string), "replicas: 2")
	require.Contains(t, manifests[0].(string), `name: "app"`)

	namespace := manifests[1].(map[string]any)
	metadata := namespace["metadata"].(map[string]any)
	require.Equal(t, "demo", metadata["name"])
}

func TestRenderManifestTemplateValueByType(t *testing.T) {
	data := map[string]any{"vars": map[string]any{"name": "app", "ns": "demo"}}

	tests := []struct {
		name  string
		value any
		want  any
	}{
		{
			name:  "string template is rendered",
			value: "{{ .vars.name }}",
			want:  "app",
		},
		{
			name:  "plain string passes through",
			value: "no-template",
			want:  "no-template",
		},
		{
			name:  "slice of any renders each element",
			value: []any{"{{ .vars.name }}", "literal"},
			want:  []any{"app", "literal"},
		},
		{
			name:  "slice of string renders each element",
			value: []string{"{{ .vars.ns }}", "literal"},
			want:  []any{"demo", "literal"},
		},
		{
			name:  "map renders each value",
			value: map[string]any{"key": "{{ .vars.name }}"},
			want:  map[string]any{"key": "app"},
		},
		{
			name:  "non-templatable type passes through unchanged",
			value: 42,
			want:  42,
		},
		{
			name:  "boolean passes through unchanged",
			value: true,
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := renderManifestTemplateValue(&schema.AtmosConfiguration{}, "field", tt.value, data)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestRenderManifestTemplateValuePropagatesNestedError(t *testing.T) {
	data := map[string]any{"vars": map[string]any{}}

	// A malformed template nested inside a slice inside a map must surface the
	// render error from the deepest element rather than being silently dropped.
	value := map[string]any{
		"items": []any{
			"ok",
			"{{ .vars.missing | invalidFunc }}",
		},
	}

	_, err := renderManifestTemplateValue(&schema.AtmosConfiguration{}, "field", value, data)
	require.Error(t, err)
	require.ErrorContains(t, err, "failed to render Kubernetes")
}

func TestRenderManifestInputTemplatesProvisionTargets(t *testing.T) {
	componentSection := map[string]any{
		config.VarsSectionName: map[string]any{
			"stage":    "dev",
			"app_name": "argocd",
		},
		config.ProvisionSectionName: map[string]any{
			"default": "cluster",
			"targets": map[string]any{
				"cluster": map[string]any{"kind": "kubernetes"},
				"deployment-repo": map[string]any{
					"kind":       "git",
					"repository": "deployments",
					"path":       "clusters/{{ .vars.stage }}/{{ .vars.app_name }}",
					"commit": map[string]any{
						"message": "Render {{ .vars.app_name }} for {{ .vars.stage }}",
					},
				},
			},
		},
	}

	require.NoError(t, renderManifestInputTemplates(&schema.AtmosConfiguration{}, componentSection))

	provision := componentSection[config.ProvisionSectionName].(map[string]any)
	targets := provision["targets"].(map[string]any)
	repo := targets["deployment-repo"].(map[string]any)
	require.Equal(t, "clusters/dev/argocd", repo["path"])
	commit := repo["commit"].(map[string]any)
	require.Equal(t, "Render argocd for dev", commit["message"])
}
