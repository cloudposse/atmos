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
		backendInfo   tb.TerraformBackendInfo
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
			backendInfo: tb.TerraformBackendInfo{
				TerraformComponent: "test-component",
				Workspace:          "test-workspace",
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
			backendInfo: tb.TerraformBackendInfo{
				TerraformComponent: "non-existent",
				Workspace:          "test-workspace",
			},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test environment
			tempDir, cleanup := tt.setup(t)
			defer cleanup()

			// Create test config
			config := &schema.AtmosConfiguration{
				TerraformDirAbsolutePath: filepath.Join(tempDir, "terraform"),
			}

			// Call the function
			result, err := tb.GetTerraformBackendLocal(config, &tt.backendInfo)

			// Verify results
			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}
