package datafetcher

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xeipuuv/gojsonschema"
)

func TestHelmLifecycleManifestSchemas(t *testing.T) {
	schemaPaths := []string{
		filepath.Join("schema", "atmos", "manifest", "1.0.json"),
		filepath.Join("schema", "stacks", "stack-config", "1.0.json"),
	}

	valid := map[string]any{
		"helm": map[string]any{
			"command":             "helm",
			"auth":                map[string]any{},
			"secrets":             map[string]any{},
			"dependencies":        map[string]any{},
			"source":              map[string]any{"uri": "github.com/cloudposse/atmos"},
			"provision":           map[string]any{},
			"rollback_on_failure": true,
			"wait_strategy":       "watcher",
			"timeout":             "10m",
			"max_history":         10,
			"values":              map[string]any{"cluster": "shared"},
			"overrides": map[string]any{
				"values": map[string]any{"environment": "test"},
				"vars":   map[string]any{"region": "us-east-1"},
				"env":    map[string]any{"HELM_DEBUG": "true"},
				"settings": map[string]any{
					"enabled": true,
				},
				"command": "helm",
				"auth":    map[string]any{},
				"secrets": map[string]any{},
				"retry":   map[string]any{"max_attempts": 3},
			},
		},
		"components": map[string]any{
			"helm": map[string]any{
				"demo-release": map[string]any{
					"chart":               "charts/demo-release",
					"namespace":           "demo",
					"secrets":             map[string]any{},
					"wait_for_jobs":       true,
					"cleanup_on_fail":     false,
					"disable_chart_hooks": false,
					"skip_crds":           false,
					"overrides": map[string]any{
						"values": map[string]any{"replicaCount": 2},
						"vars":   map[string]any{"region": "us-west-2"},
						"retry":  map[string]any{"max_attempts": 2},
					},
				},
			},
		},
	}

	invalid := []struct {
		name     string
		field    string
		manifest map[string]any
	}{
		{
			name:  "unknown wait strategy",
			field: "helm.wait_strategy",
			manifest: map[string]any{
				"helm": map[string]any{"wait_strategy": "unknown"},
			},
		},
		{
			name:  "negative history",
			field: "components.helm.demo-release.max_history",
			manifest: map[string]any{
				"components": map[string]any{
					"helm": map[string]any{
						"demo-release": map[string]any{"chart": "demo", "max_history": -1},
					},
				},
			},
		},
		{
			name:  "numeric timeout",
			field: "components.helm.demo-release.timeout",
			manifest: map[string]any{
				"components": map[string]any{
					"helm": map[string]any{
						"demo-release": map[string]any{"chart": "demo", "timeout": 300},
					},
				},
			},
		},
		{
			name:  "non-boolean rollback",
			field: "helm.rollback_on_failure",
			manifest: map[string]any{
				"helm": map[string]any{"rollback_on_failure": "true"},
			},
		},
		{
			name:  "helm values rejected by shared overrides",
			field: "overrides",
			manifest: map[string]any{
				"overrides": map[string]any{"values": map[string]any{"invalid": true}},
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
					fields := make([]string, 0, len(result.Errors()))
					for _, validationErr := range result.Errors() {
						fields = append(fields, validationErr.Field())
					}
					assert.Contains(t, fields, tt.field, "rejection did not cite the expected field: %v", result.Errors())
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
