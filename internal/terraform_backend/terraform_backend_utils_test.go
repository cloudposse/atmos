package terraform_backend_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	tb "github.com/cloudposse/atmos/internal/terraform_backend"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestGetTerraformWorkspace(t *testing.T) {
	sections := map[string]any{"workspace": "test-workspace"}
	result := tb.GetTerraformWorkspace(&sections)
	assert.Equal(t, "test-workspace", result)

	empty := map[string]any{}
	result = tb.GetTerraformWorkspace(&empty)
	assert.Equal(t, "", result)
}

func TestGetTerraformComponent(t *testing.T) {
	sections := map[string]any{"component": "test-component"}
	result := tb.GetTerraformComponent(&sections)
	assert.Equal(t, "test-component", result)

	empty := map[string]any{}
	result = tb.GetTerraformComponent(&empty)
	assert.Equal(t, "", result)
}

func TestGetComponentBackend(t *testing.T) {
	backend := map[string]any{"type": "s3"}
	sections := map[string]any{"backend": backend}
	result := tb.GetComponentBackend(&sections)
	assert.Equal(t, backend, result)

	empty := map[string]any{}
	result = tb.GetComponentBackend(&empty)
	assert.Nil(t, result)
}

func TestGetComponentBackendType(t *testing.T) {
	sections := map[string]any{"backend_type": "s3"}
	result := tb.GetComponentBackendType(&sections)
	assert.Equal(t, "s3", result)

	empty := map[string]any{}
	result = tb.GetComponentBackendType(&empty)
	assert.Equal(t, "", result)
}

func TestGetBackendAttribute(t *testing.T) {
	backend := map[string]any{
		"bucket": "my-bucket",
	}
	result := tb.GetBackendAttribute(&backend, "bucket")
	assert.Equal(t, "my-bucket", result)

	result = tb.GetBackendAttribute(&backend, "region")
	assert.Equal(t, "", result)
}

func TestProcessTerraformStateFile(t *testing.T) {
	// Test simple string value.
	jsonData := []byte(`{
		"version": 4,
		"terraform_version": "1.0.0",
		"outputs": {
			"test_output": {
				"value": "hello",
				"type": "string"
			}
		}
	}`)
	result, err := tb.ProcessTerraformStateFile(jsonData)
	assert.NoError(t, err)
	assert.Equal(t, "hello", result["test_output"])

	// Test number value (should remain number, not string).
	numberData := []byte(`{
		"version": 4,
		"terraform_version": "1.0.0",
		"outputs": {
			"port": {
				"value": 8080,
				"type": "number"
			},
			"ratio": {
				"value": 3.14,
				"type": "number"
			}
		}
	}`)
	result, err = tb.ProcessTerraformStateFile(numberData)
	assert.NoError(t, err)
	assert.Equal(t, float64(8080), result["port"])
	assert.Equal(t, 3.14, result["ratio"])

	// Test map/object value.
	mapData := []byte(`{
		"version": 4,
		"terraform_version": "1.0.0",
		"outputs": {
			"config": {
				"value": {
					"host": "localhost",
					"port": 8080,
					"enabled": true
				},
				"type": "object"
			}
		}
	}`)
	result, err = tb.ProcessTerraformStateFile(mapData)
	assert.NoError(t, err)
	configMap := result["config"].(map[string]any)
	assert.Equal(t, "localhost", configMap["host"])
	assert.Equal(t, float64(8080), configMap["port"])
	assert.Equal(t, true, configMap["enabled"])

	// Test array value.
	arrayData := []byte(`{
		"version": 4,
		"terraform_version": "1.0.0",
		"outputs": {
			"zones": {
				"value": ["us-east-1a", "us-east-1b", "us-east-1c"],
				"type": "list"
			}
		}
	}`)
	result, err = tb.ProcessTerraformStateFile(arrayData)
	assert.NoError(t, err)
	zones := result["zones"].([]any)
	assert.Equal(t, 3, len(zones))
	assert.Equal(t, "us-east-1a", zones[0])

	// Test complex nested structure.
	nestedData := []byte(`{
		"version": 4,
		"terraform_version": "1.0.0",
		"outputs": {
			"cluster": {
				"value": {
					"nodes": [
						{"name": "node1", "cpu": 2},
						{"name": "node2", "cpu": 4}
					],
					"metadata": {
						"region": "us-east-1",
						"count": 2
					}
				},
				"type": "object"
			}
		}
	}`)
	result, err = tb.ProcessTerraformStateFile(nestedData)
	assert.NoError(t, err)
	cluster := result["cluster"].(map[string]any)
	nodes := cluster["nodes"].([]any)
	assert.Equal(t, 2, len(nodes))
	node1 := nodes[0].(map[string]any)
	assert.Equal(t, "node1", node1["name"])
	assert.Equal(t, float64(2), node1["cpu"])

	// Test empty state.
	emptyData := []byte(``)
	result, err = tb.ProcessTerraformStateFile(emptyData)
	assert.NoError(t, err)
	assert.Nil(t, result)

	// Test invalid JSON.
	invalidData := []byte(`{bad json}`)
	_, err = tb.ProcessTerraformStateFile(invalidData)
	assert.Error(t, err)
}

func TestGetTerraformBackendVariable(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	values := map[string]any{
		"backend": map[string]any{
			"bucket": "my-bucket",
		},
	}
	result, err := tb.GetTerraformBackendVariable(atmosConfig, values, "backend.bucket")
	assert.NoError(t, err)
	assert.Equal(t, "my-bucket", result)
}

func TestGetTerraformBackend(t *testing.T) {
	tests := []struct {
		name            string
		componentData   map[string]any
		stateJSON       string
		expectedOutputs map[string]any
		expectError     bool
	}{
		{
			name: "valid local backend with state data",
			componentData: map[string]any{
				"component":    "sample-component",
				"workspace":    "default",
				"backend_type": "",
			},
			stateJSON: `{
				"version": 4,
				"terraform_version": "1.3.0",
				"outputs": {
					"value": {
						"value": "local-output",
						"type": "string"
					}
				}
			}`,
			expectedOutputs: map[string]any{"value": "local-output"},
			expectError:     false,
		},
		{
			name: "missing backend_type defaults to local",
			componentData: map[string]any{
				"component": "sample-component",
				"workspace": "default",
			},
			stateJSON: `{
				"version": 4,
				"terraform_version": "1.3.0",
				"outputs": {
					"value": {
						"value": "default-backend-output",
						"type": "string"
					}
				}
			}`,
			expectedOutputs: map[string]any{"value": "default-backend-output"},
			expectError:     false,
		},
		{
			name: "unsupported backend type",
			componentData: map[string]any{
				"component":    "sample-component",
				"workspace":    "default",
				"backend_type": "unsupported",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temp directory and write the test state file to simulate local backend
			tmpDir := t.TempDir()
			componentDir := filepath.Join(tmpDir, "terraform", tt.componentData["component"].(string))
			stateDir := filepath.Join(componentDir, "terraform.tfstate.d", tt.componentData["workspace"].(string))

			if tt.stateJSON != "" {
				err := os.MkdirAll(stateDir, 0o755)
				assert.NoError(t, err)

				err = os.WriteFile(filepath.Join(stateDir, "terraform.tfstate"), []byte(tt.stateJSON), 0o644)
				assert.NoError(t, err)
			}

			atmosConfig := &schema.AtmosConfiguration{
				TerraformDirAbsolutePath: filepath.Join(tmpDir, "terraform"),
			}

			outputs, err := tb.GetTerraformBackend(atmosConfig, &tt.componentData)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedOutputs, outputs)
			}
		})
	}
}
