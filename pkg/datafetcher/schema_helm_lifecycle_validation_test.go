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
			"command":      "helm",
			"auth":         map[string]any{},
			"secrets":      map[string]any{},
			"dependencies": map[string]any{},
			"source":       map[string]any{"uri": "github.com/cloudposse/atmos"},
			"provision":    map[string]any{},
			"release": map[string]any{
				"timeout":     "10m",
				"chart_hooks": true,
				"wait":        map[string]any{"strategy": "watcher", "jobs": true},
				"history":     map[string]any{"max": 10},
				"install": map[string]any{
					"timeout":    "60m",
					"crds":       "create",
					"on_failure": "uninstall",
				},
				"upgrade": map[string]any{
					"timeout":            "10m",
					"on_failure":         "rollback",
					"cleanup_on_failure": true,
				},
				"delete": map[string]any{
					"timeout": "5m",
					"wait":    map[string]any{"strategy": "legacy"},
				},
			},
			"values": map[string]any{"cluster": "shared"},
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
				"retry": map[string]any{
					"max_attempts": 3,
					"conditions":   []string{"timeout", "/connection reset/"},
				},
			},
		},
		"components": map[string]any{
			"helm": map[string]any{
				"demo-release": map[string]any{
					"chart":     "charts/demo-release",
					"namespace": "demo",
					"secrets":   map[string]any{},
					"release": map[string]any{
						"wait": map[string]any{"strategy": "watcher"},
						"install": map[string]any{
							"on_failure": "keep",
							"crds":       "skip",
						},
						"upgrade": map[string]any{"on_failure": "keep"},
					},
					"overrides": map[string]any{
						"values": map[string]any{"replicaCount": 2},
						"vars":   map[string]any{"region": "us-west-2"},
						"retry": map[string]any{
							"max_attempts": 2,
							"conditions":   []string{"temporary failure"},
						},
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
			field: "helm.release.wait.strategy",
			manifest: map[string]any{
				"helm": map[string]any{"release": map[string]any{"wait": map[string]any{"strategy": "unknown"}}},
			},
		},
		{
			name:  "negative history",
			field: "components.helm.demo-release.release.history.max",
			manifest: map[string]any{
				"components": map[string]any{
					"helm": map[string]any{
						"demo-release": map[string]any{"chart": "demo", "release": map[string]any{"history": map[string]any{"max": -1}}},
					},
				},
			},
		},
		{
			name:  "numeric timeout",
			field: "components.helm.demo-release.release.timeout",
			manifest: map[string]any{
				"components": map[string]any{
					"helm": map[string]any{
						"demo-release": map[string]any{"chart": "demo", "release": map[string]any{"timeout": 300}},
					},
				},
			},
		},
		{
			name:  "unknown upgrade failure action",
			field: "helm.release.upgrade.on_failure",
			manifest: map[string]any{
				"helm": map[string]any{"release": map[string]any{"upgrade": map[string]any{"on_failure": "notify"}}},
			},
		},
		{
			name:  "misplaced cleanup policy",
			field: "helm.release.install",
			manifest: map[string]any{
				"helm": map[string]any{"release": map[string]any{"install": map[string]any{"cleanup_on_failure": true}}},
			},
		},
		{
			name:  "replace CRDs unsupported",
			field: "helm.release.install.crds",
			manifest: map[string]any{
				"helm": map[string]any{"release": map[string]any{"install": map[string]any{"crds": "replace"}}},
			},
		},
		{
			name:  "delete wait jobs unsupported",
			field: "helm.release.delete.wait",
			manifest: map[string]any{
				"helm": map[string]any{"release": map[string]any{"delete": map[string]any{"wait": map[string]any{"jobs": true}}}},
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
