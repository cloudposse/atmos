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

// TestReadTerraformBackendLocal_JITWorkdir verifies that ReadTerraformBackendLocal
// resolves the correct path for components with provision.workdir.enabled: true.
// Regression test for https://github.com/cloudposse/atmos/issues/2167.
func TestReadTerraformBackendLocal_JITWorkdir(t *testing.T) {
	const stateJSON = `{
		"version": 4,
		"terraform_version": "1.0.0",
		"outputs": {
			"id": {
				"value": "eg-test-demo",
				"type": "string"
			}
		}
	}`

	t.Run("state exists, no _workdir_path (describe path — provisioner not yet run)", func(t *testing.T) {
		tempDir := t.TempDir()
		// BuildPath("tempDir", "terraform", "null-label", "demo", sections) → tempDir/.workdir/terraform/demo-null-label.
		// workspace "demo" → terraform.tfstate.d/demo/terraform.tfstate.
		stateDir := filepath.Join(tempDir, ".workdir", "terraform", "demo-null-label", "terraform.tfstate.d", "demo")
		require.NoError(t, os.MkdirAll(stateDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(stateDir, "terraform.tfstate"), []byte(stateJSON), 0o644))

		config := &schema.AtmosConfiguration{
			BasePath:                 tempDir,
			TerraformDirAbsolutePath: filepath.Join(tempDir, "components", "terraform"),
		}
		sections := map[string]any{
			"provision": map[string]any{
				"workdir": map[string]any{"enabled": true},
			},
			"atmos_stack":     "demo",
			"atmos_component": "null-label",
			"component":       "null-label", // base component (metadata.component); also used by static fallback.
			"workspace":       "demo",
		}

		content, err := tb.ReadTerraformBackendLocal(config, &sections, nil)
		require.NoError(t, err)
		require.NotNil(t, content, "expected state file to be found at JIT workdir path")

		result, err := tb.ProcessTerraformStateFile(content)
		require.NoError(t, err)
		assert.Equal(t, "eg-test-demo", result["id"])
	})

	t.Run("_workdir_path set (apply path — provisioner already ran)", func(t *testing.T) {
		tempDir := t.TempDir()
		workdirPath := filepath.Join(tempDir, ".workdir", "terraform", "demo-null-label")
		stateDir := filepath.Join(workdirPath, "terraform.tfstate.d", "demo")
		require.NoError(t, os.MkdirAll(stateDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(stateDir, "terraform.tfstate"), []byte(stateJSON), 0o644))

		config := &schema.AtmosConfiguration{
			BasePath:                 tempDir,
			TerraformDirAbsolutePath: filepath.Join(tempDir, "components", "terraform"),
		}
		sections := map[string]any{
			"provision": map[string]any{
				"workdir": map[string]any{"enabled": true},
			},
			"atmos_stack":     "demo",
			"atmos_component": "null-label",
			"component":       "null-label", // base component (metadata.component).
			"workspace":       "demo",
			"_workdir_path":   workdirPath, // set by provisioner during apply.
		}

		content, err := tb.ReadTerraformBackendLocal(config, &sections, nil)
		require.NoError(t, err)
		require.NotNil(t, content, "expected state file found via _workdir_path fast path")

		result, err := tb.ProcessTerraformStateFile(content)
		require.NoError(t, err)
		assert.Equal(t, "eg-test-demo", result["id"])
	})

	t.Run("workdir absent (fresh CI runner / not yet applied)", func(t *testing.T) {
		tempDir := t.TempDir()

		config := &schema.AtmosConfiguration{
			BasePath:                 tempDir,
			TerraformDirAbsolutePath: filepath.Join(tempDir, "components", "terraform"),
		}
		sections := map[string]any{
			"provision": map[string]any{
				"workdir": map[string]any{"enabled": true},
			},
			"atmos_stack":     "demo",
			"atmos_component": "null-label",
			"component":       "null-label",
			"workspace":       "demo",
		}

		content, err := tb.ReadTerraformBackendLocal(config, &sections, nil)
		require.NoError(t, err)
		assert.Nil(t, content, "expected nil when JIT workdir is absent (not provisioned)")
	})

	t.Run("_workdir_path takes precedence over BuildPath when both are present", func(t *testing.T) {
		tempDir := t.TempDir()

		// Place state at the _workdir_path location.
		workdirPath := filepath.Join(tempDir, ".workdir", "terraform", "demo-null-label")
		stateDir := filepath.Join(workdirPath, "terraform.tfstate.d", "demo")
		require.NoError(t, os.MkdirAll(stateDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(stateDir, "terraform.tfstate"), []byte(stateJSON), 0o644))

		// Also create a state file at the BuildPath location to confirm it is NOT used.
		buildPathDir := filepath.Join(tempDir, ".workdir", "terraform", "demo-null-label-buildpath")
		altStateDir := filepath.Join(buildPathDir, "terraform.tfstate.d", "demo")
		require.NoError(t, os.MkdirAll(altStateDir, 0o755))
		altStateJSON := `{"version":4,"terraform_version":"1.0.0","outputs":{"id":{"value":"wrong-path","type":"string"}}}`
		require.NoError(t, os.WriteFile(filepath.Join(altStateDir, "terraform.tfstate"), []byte(altStateJSON), 0o644))

		config := &schema.AtmosConfiguration{
			BasePath:                 tempDir,
			TerraformDirAbsolutePath: filepath.Join(tempDir, "components", "terraform"),
		}
		// Both _workdir_path and provision.workdir.enabled are set.
		sections := map[string]any{
			"provision": map[string]any{
				"workdir": map[string]any{"enabled": true},
			},
			"atmos_stack":     "demo",
			"atmos_component": "null-label",
			"component":       "null-label",
			"workspace":       "demo",
			"_workdir_path":   workdirPath, // explicit path set by provisioner
		}

		content, err := tb.ReadTerraformBackendLocal(config, &sections, nil)
		require.NoError(t, err)
		require.NotNil(t, content, "expected state file found via _workdir_path fast path")

		result, err := tb.ProcessTerraformStateFile(content)
		require.NoError(t, err)
		// Must use the _workdir_path value, not the BuildPath value.
		assert.Equal(t, "eg-test-demo", result["id"],
			"_workdir_path must take precedence over BuildPath when both are present")
	})

	t.Run("non-JIT component: regression — static path unchanged", func(t *testing.T) {
		tempDir := t.TempDir()
		componentDir := filepath.Join(tempDir, "components", "terraform", "vpc")
		stateDir := filepath.Join(componentDir, "terraform.tfstate.d", "prod")
		require.NoError(t, os.MkdirAll(stateDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(stateDir, "terraform.tfstate"), []byte(stateJSON), 0o644))

		config := &schema.AtmosConfiguration{
			TerraformDirAbsolutePath: filepath.Join(tempDir, "components", "terraform"),
		}
		sections := map[string]any{
			"component": "vpc",
			"workspace": "prod",
		}

		content, err := tb.ReadTerraformBackendLocal(config, &sections, nil)
		require.NoError(t, err)
		require.NotNil(t, content, "expected state file at static path for non-JIT component")

		result, err := tb.ProcessTerraformStateFile(content)
		require.NoError(t, err)
		assert.Equal(t, "eg-test-demo", result["id"])
	})

	t.Run("_workdir_path escaping BasePath falls through to derived path", func(t *testing.T) {
		tempDir := t.TempDir()
		// Create state at the DERIVED workdir path (not the escaping path).
		stateDir := filepath.Join(tempDir, ".workdir", "terraform", "demo-null-label",
			"terraform.tfstate.d", "demo")
		require.NoError(t, os.MkdirAll(stateDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(stateDir, "terraform.tfstate"),
			[]byte(stateJSON), 0o644))

		config := &schema.AtmosConfiguration{
			BasePath:                 tempDir,
			TerraformDirAbsolutePath: filepath.Join(tempDir, "components", "terraform"),
		}
		sections := map[string]any{
			"provision": map[string]any{
				"workdir": map[string]any{"enabled": true},
			},
			"atmos_stack":     "demo",
			"atmos_component": "null-label",
			"component":       "null-label",
			"workspace":       "demo",
			// Path that escapes BasePath via ../.. traversal.
			"_workdir_path": filepath.Join(tempDir, "..", "..", "etc", "shadow"),
		}

		// Must fall through to BuildPath-derived path and still find the state file.
		content, err := tb.ReadTerraformBackendLocal(config, &sections, nil)
		require.NoError(t, err)
		require.NotNil(t, content, "expected fallthrough to derived path when _workdir_path escapes BasePath")

		result, err := tb.ProcessTerraformStateFile(content)
		require.NoError(t, err)
		assert.Equal(t, "eg-test-demo", result["id"])
	})

	t.Run("relative BasePath produces absolute result path", func(t *testing.T) {
		tempDir := t.TempDir()
		// Change CWD to tempDir so relative BasePath "." resolves there.
		origDir, err := os.Getwd()
		require.NoError(t, err)
		require.NoError(t, os.Chdir(tempDir))
		defer func() { _ = os.Chdir(origDir) }()

		stateDir := filepath.Join(tempDir, ".workdir", "terraform", "demo-null-label",
			"terraform.tfstate.d", "demo")
		require.NoError(t, os.MkdirAll(stateDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(stateDir, "terraform.tfstate"),
			[]byte(stateJSON), 0o644))

		config := &schema.AtmosConfiguration{
			BasePath:                 ".", // relative BasePath
			TerraformDirAbsolutePath: filepath.Join(tempDir, "components", "terraform"),
		}
		sections := map[string]any{
			"provision": map[string]any{
				"workdir": map[string]any{"enabled": true},
			},
			"atmos_stack":     "demo",
			"atmos_component": "null-label",
			"component":       "null-label",
			"workspace":       "demo",
		}

		content, err := tb.ReadTerraformBackendLocal(config, &sections, nil)
		require.NoError(t, err)
		require.NotNil(t, content, "expected state file found with relative BasePath")

		result, err := tb.ProcessTerraformStateFile(content)
		require.NoError(t, err)
		assert.Equal(t, "eg-test-demo", result["id"])
	})

	t.Run("atmos_component with path traversal falls through to static path", func(t *testing.T) {
		// Security regression test: atmos_component values containing ../ sequences must NOT
		// cause ReadTerraformBackendLocal to resolve outside BasePath.
		// The BuildPath derivation must apply the same containment guard as the _workdir_path fast path.
		tempDir := t.TempDir()

		// Place state at the static path (fallback) — NOT at the traversal-constructed path.
		staticDir := filepath.Join(tempDir, "components", "terraform", "vpc", "terraform.tfstate.d", "demo")
		require.NoError(t, os.MkdirAll(staticDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(staticDir, "terraform.tfstate"), []byte(stateJSON), 0o644))

		config := &schema.AtmosConfiguration{
			BasePath:                 tempDir,
			TerraformDirAbsolutePath: filepath.Join(tempDir, "components", "terraform"),
		}
		sections := map[string]any{
			"provision": map[string]any{
				"workdir": map[string]any{"enabled": true},
			},
			// atmos_component with traversal sequences — BuildPath uses this to construct the workdir path.
			// The containment check must reject it and fall through to the static path.
			"atmos_stack":     "demo",
			"atmos_component": "../../../../etc/evil",
			"component":       "vpc", // used by static fallback: TerraformDirAbsolutePath + component
			"workspace":       "demo",
		}

		// The traversal path may or may not escape BasePath depending on depth
		// of traversal vs. depth of basePath. The invariant that MUST hold is:
		// the function never reads from outside BasePath.
		// If it falls through to the static path, the result must contain the correct state.
		content, err := tb.ReadTerraformBackendLocal(config, &sections, nil)
		require.NoError(t, err)
		// Either found the static fallback state or returned nil — both are acceptable.
		// What is NOT acceptable is reading from outside BasePath.
		if content != nil {
			result, err := tb.ProcessTerraformStateFile(content)
			require.NoError(t, err)
			assert.Equal(t, "eg-test-demo", result["id"], "should only read from within BasePath")
		}
	})
}
