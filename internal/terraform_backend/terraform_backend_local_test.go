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

// TestReadTerraformBackendLocal_DefaultWorkspace verifies that when workspace
// is "default" (meaning workspaces are disabled), the state file path should be
// just terraform.tfstate, not terraform.tfstate.d/default/terraform.tfstate.
//
// This is based on Terraform local backend behavior:
// - For the default workspace, state is stored directly at terraform.tfstate
// - For named workspaces, state is stored at terraform.tfstate.d/<workspace>/terraform.tfstate
//
// See: https://github.com/cloudposse/atmos/issues/1920
func TestReadTerraformBackendLocal_DefaultWorkspace(t *testing.T) {
	tests := []struct {
		name          string
		workspace     string
		stateLocation string // Where to place the state file (relative to component dir).
		expected      string // Expected output value.
	}{
		{
			name:          "default workspace - state at root",
			workspace:     "default",
			stateLocation: "terraform.tfstate",
			expected:      "default-workspace-value",
		},
		{
			name:          "empty workspace - state at root",
			workspace:     "",
			stateLocation: "terraform.tfstate",
			expected:      "empty-workspace-value",
		},
		{
			name:          "named workspace - state in workspace dir",
			workspace:     "prod-us-east-1",
			stateLocation: "terraform.tfstate.d/prod-us-east-1/terraform.tfstate",
			expected:      "named-workspace-value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			componentDir := filepath.Join(tempDir, "terraform", "test-component")

			// Create the state file at the expected location.
			stateFilePath := filepath.Join(componentDir, tt.stateLocation)
			err := os.MkdirAll(filepath.Dir(stateFilePath), 0o755)
			require.NoError(t, err)

			stateContent := `{
				"version": 4,
				"terraform_version": "1.0.0",
				"outputs": {
					"test_output": {
						"value": "` + tt.expected + `",
						"type": "string"
					}
				}
			}`
			err = os.WriteFile(stateFilePath, []byte(stateContent), 0o644)
			require.NoError(t, err)

			config := &schema.AtmosConfiguration{
				TerraformDirAbsolutePath: filepath.Join(tempDir, "terraform"),
			}
			componentData := map[string]any{
				"component": "test-component",
				"workspace": tt.workspace,
			}

			content, err := tb.ReadTerraformBackendLocal(config, &componentData, nil)
			require.NoError(t, err)
			require.NotNil(t, content, "Expected to find state file at %s", tt.stateLocation)

			result, err := tb.ProcessTerraformStateFile(content)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result["test_output"],
				"For workspace '%s', expected state from '%s'", tt.workspace, tt.stateLocation)
		})
	}
}

// TestReadTerraformBackendLocal_DefaultWorkspace_WrongLocation verifies that
// when workspace is "default", we do NOT look in terraform.tfstate.d/default/.
func TestReadTerraformBackendLocal_DefaultWorkspace_WrongLocation(t *testing.T) {
	tempDir := t.TempDir()
	componentDir := filepath.Join(tempDir, "terraform", "test-component")

	// Create state file in the WRONG location (terraform.tfstate.d/default/).
	wrongLocation := filepath.Join(componentDir, "terraform.tfstate.d", "default", "terraform.tfstate")
	err := os.MkdirAll(filepath.Dir(wrongLocation), 0o755)
	require.NoError(t, err)

	stateContent := `{
		"version": 4,
		"terraform_version": "1.0.0",
		"outputs": {
			"test_output": {
				"value": "wrong-location-value",
				"type": "string"
			}
		}
	}`
	err = os.WriteFile(wrongLocation, []byte(stateContent), 0o644)
	require.NoError(t, err)

	config := &schema.AtmosConfiguration{
		TerraformDirAbsolutePath: filepath.Join(tempDir, "terraform"),
	}
	componentData := map[string]any{
		"component": "test-component",
		"workspace": "default",
	}

	// Should NOT find the state file since it's in the wrong location.
	content, err := tb.ReadTerraformBackendLocal(config, &componentData, nil)
	require.NoError(t, err)
	assert.Nil(t, content, "Should not find state file in terraform.tfstate.d/default/ for default workspace")
}
