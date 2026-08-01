package datafetcher

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xeipuuv/gojsonschema"
)

func TestHelmLifecycleManifestSchemas(t *testing.T) {
	schemaPaths := []string{
		"schema/atmos/manifest/1.0.json",
		"schema/stacks/stack-config/1.0.json",
	}

	valid := map[string]any{
		"helm": map[string]any{
			"rollback_on_failure": true,
			"wait_strategy":       "watcher",
			"timeout":             "10m",
			"max_history":         10,
			"values":              map[string]any{"cluster": "shared"},
			"overrides": map[string]any{
				"values": map[string]any{"environment": "test"},
			},
		},
		"components": map[string]any{
			"helm": map[string]any{
				"demo-release": map[string]any{
					"chart":               "charts/demo-release",
					"namespace":           "demo",
					"wait_for_jobs":       true,
					"cleanup_on_fail":     false,
					"disable_chart_hooks": false,
					"skip_crds":           false,
					"overrides": map[string]any{
						"values": map[string]any{"replicaCount": 2},
					},
				},
			},
		},
	}

	invalid := []struct {
		name     string
		manifest map[string]any
	}{
		{
			name: "unknown wait strategy",
			manifest: map[string]any{
				"helm": map[string]any{"wait_strategy": "unknown"},
			},
		},
		{
			name: "negative history",
			manifest: map[string]any{
				"components": map[string]any{
					"helm": map[string]any{
						"demo-release": map[string]any{"chart": "demo", "max_history": -1},
					},
				},
			},
		},
		{
			name: "numeric timeout",
			manifest: map[string]any{
				"components": map[string]any{
					"helm": map[string]any{
						"demo-release": map[string]any{"chart": "demo", "timeout": 300},
					},
				},
			},
		},
		{
			name: "non-boolean rollback",
			manifest: map[string]any{
				"helm": map[string]any{"rollback_on_failure": "true"},
			},
		},
	}

	for _, schemaPath := range schemaPaths {
		t.Run(schemaPath, func(t *testing.T) {
			schemaData, err := os.ReadFile(schemaPath)
			require.NoError(t, err)

			result := validateHelmManifest(t, schemaData, valid)
			assert.True(t, result.Valid(), "valid native Helm lifecycle manifest was rejected: %v", result.Errors())

			for _, tt := range invalid {
				t.Run(tt.name, func(t *testing.T) {
					result := validateHelmManifest(t, schemaData, tt.manifest)
					assert.False(t, result.Valid(), "invalid native Helm lifecycle manifest was accepted")
				})
			}
		})
	}
}

func validateHelmManifest(t *testing.T, schemaData []byte, manifest map[string]any) *gojsonschema.Result {
	t.Helper()
	document, err := json.Marshal(manifest)
	require.NoError(t, err)
	result, err := gojsonschema.Validate(
		gojsonschema.NewBytesLoader(schemaData),
		gojsonschema.NewBytesLoader(document),
	)
	require.NoError(t, err)
	return result
}
