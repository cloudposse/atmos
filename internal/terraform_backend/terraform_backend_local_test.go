package terraform_backend_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tb "github.com/cloudposse/atmos/internal/terraform_backend"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestGetTerraformBackendLocal(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(t *testing.T) (string, func())
		componentData map[string]any
		expected      map[string]any
		expectedError string
	}{
		{
			name: "successful state file read",
			setup: func(t *testing.T) (string, func()) {
				tempDir := t.TempDir()
				componentDir := filepath.Join(tempDir, "terraform", "test-component")
				stateDir := filepath.Join(componentDir, "terraform.tfstate.d", "test-workspace")

				err := os.MkdirAll(stateDir, 0o755)
				require.NoError(t, err)

				stateFile := filepath.Join(stateDir, "terraform.tfstate")
				err = os.WriteFile(stateFile, []byte(`{
					"version": 4,
					"terraform_version": "1.0.0",
					"outputs": {
						"test_output": {
							"value": "test-value",
							"type": "string"
						}
					}
				}`), 0o644)
				require.NoError(t, err)

				return tempDir, func() {}
			},
			componentData: map[string]any{
				"component": "test-component",
				"workspace": "test-workspace",
			},
			expected: map[string]any{
				"test_output": "test-value",
			},
		},
		{
			name: "non-existent state file",
			setup: func(t *testing.T) (string, func()) {
				tempDir := t.TempDir()
				return tempDir, func() {}
			},
			componentData: map[string]any{
				"component": "non-existent",
				"workspace": "test-workspace",
			},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir, cleanup := tt.setup(t)
			defer cleanup()

			config := &schema.AtmosConfiguration{
				TerraformDirAbsolutePath: filepath.Join(tempDir, "terraform"),
			}

			// Use componentData as a pointer.
			content, err := tb.ReadTerraformBackendLocal(config, &tt.componentData, nil)

			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				return
			}

			require.NoError(t, err)

			if content == nil {
				assert.Nil(t, tt.expected)
				return
			}

			result, err := tb.ProcessTerraformStateFile(content)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
