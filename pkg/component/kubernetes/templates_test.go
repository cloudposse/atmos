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
