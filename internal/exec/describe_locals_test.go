package exec

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestDeriveStackFileName(t *testing.T) {
	tests := []struct {
		name           string
		stacksBasePath string
		filePath       string
		expected       string
	}{
		{
			name:           "simple file path",
			stacksBasePath: "/path/to/stacks",
			filePath:       "/path/to/stacks/dev.yaml",
			expected:       "dev",
		},
		{
			name:           "nested file path",
			stacksBasePath: "/path/to/stacks",
			filePath:       "/path/to/stacks/deploy/dev.yaml",
			expected:       "deploy/dev",
		},
		{
			name:           "deeply nested file path",
			stacksBasePath: "/path/to/stacks",
			filePath:       "/path/to/stacks/org/team/deploy/dev.yaml",
			expected:       "org/team/deploy/dev",
		},
		{
			name:           "empty base path falls back to filename",
			stacksBasePath: "",
			filePath:       "/path/to/stacks/deploy/dev.yaml",
			expected:       "dev",
		},
		{
			name:           "yml extension",
			stacksBasePath: "/path/to/stacks",
			filePath:       "/path/to/stacks/prod.yml",
			expected:       "prod",
		},
		{
			name:           "file path not under base path returns relative path",
			stacksBasePath: "/path/to/stacks",
			filePath:       "/other/location/dev.yaml",
			expected:       "../../../other/location/dev",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &mockAtmosConfig{
				stacksBaseAbsolutePath: tt.stacksBasePath,
			}

			result := deriveStackFileName(atmosConfig.toSchema(), tt.filePath)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDeriveStackName(t *testing.T) {
	tests := []struct {
		name            string
		stackFileName   string
		varsSection     map[string]any
		stackSectionMap map[string]any
		expected        string
	}{
		{
			name:          "explicit name in manifest",
			stackFileName: "deploy/dev",
			varsSection:   nil,
			stackSectionMap: map[string]any{
				"name": "my-custom-stack-name",
			},
			expected: "my-custom-stack-name",
		},
		{
			name:          "empty name falls back to filename",
			stackFileName: "deploy/dev",
			varsSection:   nil,
			stackSectionMap: map[string]any{
				"name": "",
			},
			expected: "deploy/dev",
		},
		{
			name:            "no name uses filename",
			stackFileName:   "deploy/prod",
			varsSection:     nil,
			stackSectionMap: map[string]any{},
			expected:        "deploy/prod",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &mockAtmosConfig{}

			result := deriveStackName(atmosConfig.toSchema(), tt.stackFileName, tt.varsSection, tt.stackSectionMap)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// mockAtmosConfig is a helper for creating test configurations.
type mockAtmosConfig struct {
	stacksBaseAbsolutePath string
	nameTemplate           string
	namePattern            string
}

func (m *mockAtmosConfig) toSchema() *schema.AtmosConfiguration {
	return &schema.AtmosConfiguration{
		StacksBaseAbsolutePath: m.stacksBaseAbsolutePath,
		Stacks: schema.Stacks{
			NameTemplate: m.nameTemplate,
			NamePattern:  m.namePattern,
		},
	}
}

func TestBuildStackLocalsFromContext(t *testing.T) {
	tests := []struct {
		name       string
		localsCtx  *LocalsContext
		expectKeys []string
	}{
		{
			name:       "nil context returns empty map",
			localsCtx:  nil,
			expectKeys: []string{},
		},
		{
			name: "global only",
			localsCtx: &LocalsContext{
				Global: map[string]any{"key": "value"},
			},
			expectKeys: []string{"global", "merged"},
		},
		{
			name: "terraform locals",
			localsCtx: &LocalsContext{
				Global:             map[string]any{"global_key": "global_value"},
				Terraform:          map[string]any{"tf_key": "tf_value"},
				HasTerraformLocals: true,
			},
			expectKeys: []string{"global", "terraform", "merged"},
		},
		{
			name: "helmfile locals",
			localsCtx: &LocalsContext{
				Global:            map[string]any{"global_key": "global_value"},
				Helmfile:          map[string]any{"hf_key": "hf_value"},
				HasHelmfileLocals: true,
			},
			expectKeys: []string{"global", "helmfile", "merged"},
		},
		{
			name: "packer locals",
			localsCtx: &LocalsContext{
				Global:          map[string]any{"global_key": "global_value"},
				Packer:          map[string]any{"pk_key": "pk_value"},
				HasPackerLocals: true,
			},
			expectKeys: []string{"global", "packer", "merged"},
		},
		{
			name: "all sections",
			localsCtx: &LocalsContext{
				Global:             map[string]any{"global_key": "global_value"},
				Terraform:          map[string]any{"tf_key": "tf_value"},
				Helmfile:           map[string]any{"hf_key": "hf_value"},
				Packer:             map[string]any{"pk_key": "pk_value"},
				HasTerraformLocals: true,
				HasHelmfileLocals:  true,
				HasPackerLocals:    true,
			},
			expectKeys: []string{"global", "terraform", "helmfile", "packer", "merged"},
		},
		{
			name: "empty global returns empty map",
			localsCtx: &LocalsContext{
				Global: map[string]any{},
			},
			expectKeys: []string{},
		},
		{
			name: "terraform without flag not included",
			localsCtx: &LocalsContext{
				Global:             map[string]any{"key": "value"},
				Terraform:          map[string]any{"tf_key": "tf_value"},
				HasTerraformLocals: false,
			},
			expectKeys: []string{"global", "merged"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildStackLocalsFromContext(tt.localsCtx)
			assert.Len(t, result, len(tt.expectKeys))
			for _, key := range tt.expectKeys {
				assert.Contains(t, result, key)
			}
		})
	}
}

func TestProcessStackFileForLocals(t *testing.T) {
	// Create a temporary directory for test files.
	tempDir := t.TempDir()

	// Create a valid YAML file with locals.
	validYAML := `
locals:
  namespace: acme
  environment: dev
vars:
  stage: test
`
	validFile := filepath.Join(tempDir, "valid.yaml")
	err := os.WriteFile(validFile, []byte(validYAML), 0o644)
	require.NoError(t, err)

	// Create an invalid YAML file.
	invalidYAML := `invalid: yaml: content: [broken`
	invalidFile := filepath.Join(tempDir, "invalid.yaml")
	err = os.WriteFile(invalidFile, []byte(invalidYAML), 0o644)
	require.NoError(t, err)

	// Create an empty YAML file.
	emptyFile := filepath.Join(tempDir, "empty.yaml")
	err = os.WriteFile(emptyFile, []byte(""), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath: tempDir,
	}

	t.Run("valid file with locals", func(t *testing.T) {
		result, err := processStackFileForLocals(atmosConfig, validFile, "")
		require.NoError(t, err)
		assert.Equal(t, "valid", result.StackName)
		assert.NotEmpty(t, result.StackLocals)
		assert.Contains(t, result.StackLocals, "global")
		assert.True(t, result.Found)
	})

	t.Run("file not found", func(t *testing.T) {
		_, err := processStackFileForLocals(atmosConfig, "/nonexistent/file.yaml", "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read stack file")
	})

	t.Run("invalid YAML returns empty result", func(t *testing.T) {
		result, err := processStackFileForLocals(atmosConfig, invalidFile, "")
		require.NoError(t, err)
		assert.Empty(t, result.StackName)
		assert.Empty(t, result.StackLocals)
		assert.False(t, result.Found)
	})

	t.Run("empty file returns empty result", func(t *testing.T) {
		result, err := processStackFileForLocals(atmosConfig, emptyFile, "")
		require.NoError(t, err)
		assert.Empty(t, result.StackName)
		assert.Empty(t, result.StackLocals)
		assert.False(t, result.Found)
	})

	t.Run("filter by stack name matches", func(t *testing.T) {
		result, err := processStackFileForLocals(atmosConfig, validFile, "valid")
		require.NoError(t, err)
		assert.Equal(t, "valid", result.StackName)
		assert.NotEmpty(t, result.StackLocals)
		assert.True(t, result.Found)
	})

	t.Run("filter by stack name does not match", func(t *testing.T) {
		result, err := processStackFileForLocals(atmosConfig, validFile, "other-stack")
		require.NoError(t, err)
		assert.Empty(t, result.StackName)
		assert.Empty(t, result.StackLocals)
		assert.False(t, result.Found)
	})
}

func TestExecuteDescribeLocals(t *testing.T) {
	// Create a temporary directory for test files.
	tempDir := t.TempDir()

	// Create stack files.
	devYAML := `
locals:
  namespace: acme
  environment: dev
`
	devFile := filepath.Join(tempDir, "dev.yaml")
	err := os.WriteFile(devFile, []byte(devYAML), 0o644)
	require.NoError(t, err)

	prodYAML := `
locals:
  namespace: acme
  environment: prod
`
	prodFile := filepath.Join(tempDir, "prod.yaml")
	err = os.WriteFile(prodFile, []byte(prodYAML), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath:        tempDir,
		StackConfigFilesAbsolutePaths: []string{devFile, prodFile},
	}

	t.Run("returns all stacks", func(t *testing.T) {
		result, err := ExecuteDescribeLocals(atmosConfig, "")
		require.NoError(t, err)
		assert.Len(t, result, 2)
		assert.Contains(t, result, "dev")
		assert.Contains(t, result, "prod")
	})

	t.Run("filters by stack", func(t *testing.T) {
		result, err := ExecuteDescribeLocals(atmosConfig, "dev")
		require.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Contains(t, result, "dev")
	})

	t.Run("empty config returns empty map", func(t *testing.T) {
		emptyConfig := &schema.AtmosConfiguration{}
		result, err := ExecuteDescribeLocals(emptyConfig, "")
		require.NoError(t, err)
		assert.Empty(t, result)
	})
}

func TestDeriveStackNameWithTemplate(t *testing.T) {
	tests := []struct {
		name            string
		stackFileName   string
		varsSection     map[string]any
		stackSectionMap map[string]any
		nameTemplate    string
		expected        string
	}{
		{
			name:          "name template with vars",
			stackFileName: "deploy/dev",
			varsSection: map[string]any{
				"namespace":   "acme",
				"environment": "dev",
				"stage":       "us-east-1",
			},
			stackSectionMap: map[string]any{},
			// Template uses .vars.* to access varsSection values.
			nameTemplate: "{{ .vars.namespace }}-{{ .vars.environment }}-{{ .vars.stage }}",
			expected:     "acme-dev-us-east-1",
		},
		{
			name:          "explicit name overrides template",
			stackFileName: "deploy/dev",
			varsSection: map[string]any{
				"namespace": "acme",
			},
			stackSectionMap: map[string]any{
				"name": "custom-name",
			},
			nameTemplate: "{{ .vars.namespace }}-derived",
			expected:     "custom-name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{
				Stacks: schema.Stacks{
					NameTemplate: tt.nameTemplate,
				},
			}
			result := deriveStackName(atmosConfig, tt.stackFileName, tt.varsSection, tt.stackSectionMap)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDeriveStackNameWithNamePattern(t *testing.T) {
	tests := []struct {
		name            string
		stackFileName   string
		varsSection     map[string]any
		stackSectionMap map[string]any
		namePattern     string
		expected        string
	}{
		{
			name:          "name pattern with vars",
			stackFileName: "deploy/dev",
			varsSection: map[string]any{
				"namespace":   "acme",
				"environment": "dev",
				"stage":       "us-east-1",
			},
			stackSectionMap: map[string]any{},
			namePattern:     "{namespace}-{environment}-{stage}",
			expected:        "acme-dev-us-east-1",
		},
		{
			name:          "explicit name overrides pattern",
			stackFileName: "deploy/dev",
			varsSection: map[string]any{
				"namespace": "acme",
			},
			stackSectionMap: map[string]any{
				"name": "custom-name",
			},
			namePattern: "{namespace}-derived",
			expected:    "custom-name",
		},
		{
			name:            "fallback to filename when vars missing",
			stackFileName:   "deploy/prod",
			varsSection:     map[string]any{},
			stackSectionMap: map[string]any{},
			namePattern:     "{namespace}-{environment}",
			expected:        "deploy/prod",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{
				Stacks: schema.Stacks{
					NamePattern: tt.namePattern,
				},
			}
			result := deriveStackName(atmosConfig, tt.stackFileName, tt.varsSection, tt.stackSectionMap)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNewDescribeLocalsExec(t *testing.T) {
	exec := NewDescribeLocalsExec()
	assert.NotNil(t, exec)
}

func TestGetLocalsForComponentType(t *testing.T) {
	tests := []struct {
		name          string
		stackLocals   map[string]any
		componentType string
		expected      map[string]any
	}{
		{
			name: "returns terraform section when available",
			stackLocals: map[string]any{
				"global": map[string]any{
					"namespace": "acme",
				},
				"terraform": map[string]any{
					"namespace":      "acme",
					"backend_bucket": "acme-tfstate",
				},
				"merged": map[string]any{
					"namespace":      "acme",
					"backend_bucket": "acme-tfstate",
				},
			},
			componentType: "terraform",
			expected: map[string]any{
				"namespace":      "acme",
				"backend_bucket": "acme-tfstate",
			},
		},
		{
			name: "returns helmfile section when available",
			stackLocals: map[string]any{
				"global": map[string]any{
					"namespace": "acme",
				},
				"helmfile": map[string]any{
					"namespace":   "acme",
					"release_dir": "/releases",
				},
				"merged": map[string]any{
					"namespace":   "acme",
					"release_dir": "/releases",
				},
			},
			componentType: "helmfile",
			expected: map[string]any{
				"namespace":   "acme",
				"release_dir": "/releases",
			},
		},
		{
			name: "returns packer section when available",
			stackLocals: map[string]any{
				"global": map[string]any{
					"namespace": "acme",
				},
				"packer": map[string]any{
					"namespace": "acme",
					"ami_name":  "my-ami",
				},
				"merged": map[string]any{
					"namespace": "acme",
					"ami_name":  "my-ami",
				},
			},
			componentType: "packer",
			expected: map[string]any{
				"namespace": "acme",
				"ami_name":  "my-ami",
			},
		},
		{
			name: "falls back to merged when section not available",
			stackLocals: map[string]any{
				"global": map[string]any{
					"namespace": "acme",
				},
				"merged": map[string]any{
					"namespace":   "acme",
					"environment": "dev",
				},
			},
			componentType: "terraform",
			expected: map[string]any{
				"namespace":   "acme",
				"environment": "dev",
			},
		},
		{
			name: "falls back to global when merged not available",
			stackLocals: map[string]any{
				"global": map[string]any{
					"namespace": "acme",
				},
			},
			componentType: "terraform",
			expected: map[string]any{
				"namespace": "acme",
			},
		},
		{
			name:          "returns empty map when no locals available",
			stackLocals:   map[string]any{},
			componentType: "terraform",
			expected:      map[string]any{},
		},
		{
			name: "handles unknown component type",
			stackLocals: map[string]any{
				"global": map[string]any{
					"namespace": "acme",
				},
				"merged": map[string]any{
					"namespace": "acme",
				},
			},
			componentType: "unknown",
			expected: map[string]any{
				"namespace": "acme",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getLocalsForComponentType(tt.stackLocals, tt.componentType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExecuteForComponent(t *testing.T) {
	t.Run("requires stack when component is specified", func(t *testing.T) {
		exec := &describeLocalsExec{}

		args := &DescribeLocalsArgs{
			Component: "vpc",
			// FilterByStack is empty - should error.
		}

		_, err := exec.executeForComponent(&schema.AtmosConfiguration{}, args)
		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrStackRequiredWithComponent)
	})

	t.Run("returns error when component not found", func(t *testing.T) {
		expectedErr := errors.New("component not found")

		exec := &describeLocalsExec{
			executeDescribeComponent: func(params *ExecuteDescribeComponentParams) (map[string]any, error) {
				return nil, expectedErr
			},
		}

		args := &DescribeLocalsArgs{
			Component:     "nonexistent",
			FilterByStack: "dev",
		}

		_, err := exec.executeForComponent(&schema.AtmosConfiguration{}, args)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to describe component")
	})

	t.Run("returns error when stack has no locals", func(t *testing.T) {
		exec := &describeLocalsExec{
			executeDescribeComponent: func(params *ExecuteDescribeComponentParams) (map[string]any, error) {
				return map[string]any{
					"component_type": "terraform",
				}, nil
			},
			executeDescribeLocals: func(ac *schema.AtmosConfiguration, filterByStack string) (map[string]any, error) {
				// Return empty map - stack exists but has no locals.
				return map[string]any{}, nil
			},
		}

		args := &DescribeLocalsArgs{
			Component:     "vpc",
			FilterByStack: "dev",
		}

		_, err := exec.executeForComponent(&schema.AtmosConfiguration{}, args)
		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrStackHasNoLocals)
	})

	t.Run("returns error when stack not found", func(t *testing.T) {
		exec := &describeLocalsExec{
			executeDescribeComponent: func(params *ExecuteDescribeComponentParams) (map[string]any, error) {
				return map[string]any{
					"component_type": "terraform",
				}, nil
			},
			executeDescribeLocals: func(ac *schema.AtmosConfiguration, filterByStack string) (map[string]any, error) {
				// Return ErrStackNotFound - stack doesn't exist.
				return nil, fmt.Errorf("%w: %s", errUtils.ErrStackNotFound, filterByStack)
			},
		}

		args := &DescribeLocalsArgs{
			Component:     "vpc",
			FilterByStack: "nonexistent",
		}

		_, err := exec.executeForComponent(&schema.AtmosConfiguration{}, args)
		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrStackNotFound)
	})

	t.Run("returns locals for terraform component", func(t *testing.T) {
		exec := &describeLocalsExec{
			executeDescribeComponent: func(params *ExecuteDescribeComponentParams) (map[string]any, error) {
				assert.Equal(t, "vpc", params.Component)
				assert.Equal(t, "dev", params.Stack)
				return map[string]any{
					"component_type": "terraform",
				}, nil
			},
			executeDescribeLocals: func(ac *schema.AtmosConfiguration, filterByStack string) (map[string]any, error) {
				return map[string]any{
					"dev": map[string]any{
						"global": map[string]any{
							"namespace":   "acme",
							"environment": "dev",
						},
						"terraform": map[string]any{
							"namespace":      "acme",
							"environment":    "dev",
							"backend_bucket": "acme-dev-tfstate",
						},
						"merged": map[string]any{
							"namespace":      "acme",
							"environment":    "dev",
							"backend_bucket": "acme-dev-tfstate",
						},
					},
				}, nil
			},
		}

		args := &DescribeLocalsArgs{
			Component:     "vpc",
			FilterByStack: "dev",
		}

		result, err := exec.executeForComponent(&schema.AtmosConfiguration{}, args)
		require.NoError(t, err)

		assert.Equal(t, "vpc", result["component"])
		assert.Equal(t, "dev", result["stack"])
		assert.Equal(t, "terraform", result["component_type"])

		locals, ok := result["locals"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "acme", locals["namespace"])
		assert.Equal(t, "dev", locals["environment"])
		assert.Equal(t, "acme-dev-tfstate", locals["backend_bucket"])
	})

	t.Run("returns locals for helmfile component", func(t *testing.T) {
		exec := &describeLocalsExec{
			executeDescribeComponent: func(params *ExecuteDescribeComponentParams) (map[string]any, error) {
				return map[string]any{
					"component_type": "helmfile",
				}, nil
			},
			executeDescribeLocals: func(ac *schema.AtmosConfiguration, filterByStack string) (map[string]any, error) {
				return map[string]any{
					"prod": map[string]any{
						"global": map[string]any{
							"namespace": "acme",
						},
						"helmfile": map[string]any{
							"namespace":    "acme",
							"release_name": "my-release",
						},
						"merged": map[string]any{
							"namespace":    "acme",
							"release_name": "my-release",
						},
					},
				}, nil
			},
		}

		args := &DescribeLocalsArgs{
			Component:     "nginx",
			FilterByStack: "prod",
		}

		result, err := exec.executeForComponent(&schema.AtmosConfiguration{}, args)
		require.NoError(t, err)

		assert.Equal(t, "nginx", result["component"])
		assert.Equal(t, "prod", result["stack"])
		assert.Equal(t, "helmfile", result["component_type"])

		locals, ok := result["locals"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "my-release", locals["release_name"])
	})

	t.Run("defaults to terraform when component_type not set", func(t *testing.T) {
		exec := &describeLocalsExec{
			executeDescribeComponent: func(params *ExecuteDescribeComponentParams) (map[string]any, error) {
				// Return without component_type.
				return map[string]any{}, nil
			},
			executeDescribeLocals: func(ac *schema.AtmosConfiguration, filterByStack string) (map[string]any, error) {
				return map[string]any{
					"dev": map[string]any{
						"global": map[string]any{
							"namespace": "acme",
						},
						"merged": map[string]any{
							"namespace": "acme",
						},
					},
				}, nil
			},
		}

		args := &DescribeLocalsArgs{
			Component:     "vpc",
			FilterByStack: "dev",
		}

		result, err := exec.executeForComponent(&schema.AtmosConfiguration{}, args)
		require.NoError(t, err)

		// Should default to terraform.
		assert.Equal(t, "terraform", result["component_type"])
	})
}

// TestExecuteForComponentOutputStructure verifies that component queries return the correct structure.
func TestExecuteForComponentOutputStructure(t *testing.T) {
	t.Run("component output has expected structure", func(t *testing.T) {
		exec := &describeLocalsExec{
			executeDescribeComponent: func(params *ExecuteDescribeComponentParams) (map[string]any, error) {
				return map[string]any{
					"component_type": "terraform",
				}, nil
			},
			executeDescribeLocals: func(ac *schema.AtmosConfiguration, filterByStack string) (map[string]any, error) {
				return map[string]any{
					"dev-us-east-1": map[string]any{
						"global": map[string]any{
							"namespace": "acme",
						},
						"terraform": map[string]any{
							"namespace":      "acme",
							"backend_bucket": "acme-tfstate",
						},
						"merged": map[string]any{
							"namespace":      "acme",
							"backend_bucket": "acme-tfstate",
						},
					},
				}, nil
			},
		}

		args := &DescribeLocalsArgs{
			Component:     "vpc",
			FilterByStack: "dev-us-east-1",
		}

		result, err := exec.executeForComponent(&schema.AtmosConfiguration{}, args)
		require.NoError(t, err)

		// Verify component output structure.
		assert.Contains(t, result, "component", "component output should have 'component' key")
		assert.Contains(t, result, "stack", "component output should have 'stack' key")
		assert.Contains(t, result, "component_type", "component output should have 'component_type' key")
		assert.Contains(t, result, "locals", "component output should have 'locals' key")

		// Verify values.
		assert.Equal(t, "vpc", result["component"])
		assert.Equal(t, "dev-us-east-1", result["stack"])
		assert.Equal(t, "terraform", result["component_type"])

		// Verify locals are flattened (not nested with global/terraform/merged).
		locals, ok := result["locals"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "acme", locals["namespace"])
		assert.Equal(t, "acme-tfstate", locals["backend_bucket"])

		// Verify locals do NOT have nested section keys.
		_, hasGlobal := locals["global"]
		_, hasTerraform := locals["terraform"]
		_, hasMerged := locals["merged"]
		assert.False(t, hasGlobal, "component locals should NOT have nested 'global' key")
		assert.False(t, hasTerraform, "component locals should NOT have nested 'terraform' key")
		assert.False(t, hasMerged, "component locals should NOT have nested 'merged' key")
	})

	t.Run("component output uses logical stack name", func(t *testing.T) {
		exec := &describeLocalsExec{
			executeDescribeComponent: func(params *ExecuteDescribeComponentParams) (map[string]any, error) {
				// Filter should be "deploy/prod" (file path).
				assert.Equal(t, "deploy/prod", params.Stack)
				return map[string]any{
					"component_type": "terraform",
				}, nil
			},
			executeDescribeLocals: func(ac *schema.AtmosConfiguration, filterByStack string) (map[string]any, error) {
				// Return logical stack name as key.
				return map[string]any{
					"prod-us-west-2": map[string]any{
						"global": map[string]any{
							"namespace": "acme",
						},
						"merged": map[string]any{
							"namespace": "acme",
						},
					},
				}, nil
			},
		}

		args := &DescribeLocalsArgs{
			Component:     "vpc",
			FilterByStack: "deploy/prod",
		}

		result, err := exec.executeForComponent(&schema.AtmosConfiguration{}, args)
		require.NoError(t, err)

		// The output should use the logical stack name from ExecuteDescribeLocals result.
		assert.Equal(t, "prod-us-west-2", result["stack"])
	})
}

func TestDescribeLocalsExecExecute(t *testing.T) {
	t.Run("execute without query", func(t *testing.T) {
		// Create a temporary directory for test files.
		tempDir := t.TempDir()

		devYAML := `
locals:
  namespace: acme
`
		devFile := filepath.Join(tempDir, "dev.yaml")
		err := os.WriteFile(devFile, []byte(devYAML), 0o644)
		require.NoError(t, err)

		atmosConfig := &schema.AtmosConfiguration{
			StacksBaseAbsolutePath:        tempDir,
			StackConfigFilesAbsolutePaths: []string{devFile},
		}

		// Create a custom exec with mocked dependencies.
		exec := &describeLocalsExec{
			executeDescribeLocals: ExecuteDescribeLocals,
			isTTYSupportForStdout: func() bool { return false },
			printOrWriteToFile: func(ac *schema.AtmosConfiguration, format string, file string, data any) error {
				return nil
			},
		}

		args := &DescribeLocalsArgs{
			Format: "yaml",
		}

		err = exec.Execute(atmosConfig, args)
		require.NoError(t, err)
	})

	t.Run("execute with query", func(t *testing.T) {
		tempDir := t.TempDir()

		devYAML := `
locals:
  namespace: acme
  environment: dev
`
		devFile := filepath.Join(tempDir, "dev.yaml")
		err := os.WriteFile(devFile, []byte(devYAML), 0o644)
		require.NoError(t, err)

		atmosConfig := &schema.AtmosConfiguration{
			StacksBaseAbsolutePath:        tempDir,
			StackConfigFilesAbsolutePaths: []string{devFile},
		}

		exec := &describeLocalsExec{
			executeDescribeLocals: ExecuteDescribeLocals,
			isTTYSupportForStdout: func() bool { return false },
			printOrWriteToFile: func(ac *schema.AtmosConfiguration, format string, file string, data any) error {
				return nil
			},
		}

		args := &DescribeLocalsArgs{
			Format: "yaml",
			Query:  ".dev.merged.namespace",
		}

		err = exec.Execute(atmosConfig, args)
		require.NoError(t, err)
	})

	t.Run("execute with file output", func(t *testing.T) {
		tempDir := t.TempDir()

		devYAML := `
locals:
  namespace: acme
`
		devFile := filepath.Join(tempDir, "dev.yaml")
		err := os.WriteFile(devFile, []byte(devYAML), 0o644)
		require.NoError(t, err)

		atmosConfig := &schema.AtmosConfiguration{
			StacksBaseAbsolutePath:        tempDir,
			StackConfigFilesAbsolutePaths: []string{devFile},
		}

		outputFile := filepath.Join(tempDir, "output.json")

		exec := &describeLocalsExec{
			executeDescribeLocals: ExecuteDescribeLocals,
			isTTYSupportForStdout: func() bool { return false },
			printOrWriteToFile: func(ac *schema.AtmosConfiguration, format string, file string, data any) error {
				return nil
			},
		}

		args := &DescribeLocalsArgs{
			Format: "json",
			File:   outputFile,
		}

		err = exec.Execute(atmosConfig, args)
		require.NoError(t, err)
	})

	t.Run("execute with stack filter", func(t *testing.T) {
		tempDir := t.TempDir()

		devYAML := `
locals:
  namespace: acme
  environment: dev
`
		devFile := filepath.Join(tempDir, "dev.yaml")
		err := os.WriteFile(devFile, []byte(devYAML), 0o644)
		require.NoError(t, err)

		prodYAML := `
locals:
  namespace: acme
  environment: prod
`
		prodFile := filepath.Join(tempDir, "prod.yaml")
		err = os.WriteFile(prodFile, []byte(prodYAML), 0o644)
		require.NoError(t, err)

		atmosConfig := &schema.AtmosConfiguration{
			StacksBaseAbsolutePath:        tempDir,
			StackConfigFilesAbsolutePaths: []string{devFile, prodFile},
		}

		exec := &describeLocalsExec{
			executeDescribeLocals: ExecuteDescribeLocals,
			isTTYSupportForStdout: func() bool { return false },
			printOrWriteToFile: func(ac *schema.AtmosConfiguration, format string, file string, data any) error {
				return nil
			},
		}

		args := &DescribeLocalsArgs{
			Format:        "yaml",
			FilterByStack: "dev",
		}

		err = exec.Execute(atmosConfig, args)
		require.NoError(t, err)
	})

	t.Run("execute returns error from executeDescribeLocals", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{}
		expectedErr := errors.New("execute error")

		exec := &describeLocalsExec{
			executeDescribeLocals: func(ac *schema.AtmosConfiguration, filterByStack string) (map[string]any, error) {
				return nil, expectedErr
			},
			isTTYSupportForStdout: func() bool { return false },
			printOrWriteToFile: func(ac *schema.AtmosConfiguration, format string, file string, data any) error {
				return nil
			},
		}

		args := &DescribeLocalsArgs{
			Format: "yaml",
		}

		err := exec.Execute(atmosConfig, args)
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("execute with component argument", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{}
		var capturedData any

		exec := &describeLocalsExec{
			executeDescribeComponent: func(params *ExecuteDescribeComponentParams) (map[string]any, error) {
				return map[string]any{
					"component_type": "terraform",
				}, nil
			},
			executeDescribeLocals: func(ac *schema.AtmosConfiguration, filterByStack string) (map[string]any, error) {
				return map[string]any{
					"dev": map[string]any{
						"global": map[string]any{
							"namespace": "acme",
						},
						"terraform": map[string]any{
							"namespace":      "acme",
							"backend_bucket": "acme-dev-tfstate",
						},
						"merged": map[string]any{
							"namespace":      "acme",
							"backend_bucket": "acme-dev-tfstate",
						},
					},
				}, nil
			},
			isTTYSupportForStdout: func() bool { return false },
			printOrWriteToFile: func(ac *schema.AtmosConfiguration, format string, file string, data any) error {
				capturedData = data
				return nil
			},
		}

		args := &DescribeLocalsArgs{
			Component:     "vpc",
			FilterByStack: "dev",
			Format:        "yaml",
		}

		err := exec.Execute(atmosConfig, args)
		require.NoError(t, err)

		// Verify the output structure.
		result, ok := capturedData.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "vpc", result["component"])
		assert.Equal(t, "dev", result["stack"])
		assert.Equal(t, "terraform", result["component_type"])
	})

	t.Run("execute with component but missing stack returns error", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{}

		exec := &describeLocalsExec{
			isTTYSupportForStdout: func() bool { return false },
			printOrWriteToFile: func(ac *schema.AtmosConfiguration, format string, file string, data any) error {
				return nil
			},
		}

		args := &DescribeLocalsArgs{
			Component: "vpc",
			// FilterByStack is empty - should error.
			Format: "yaml",
		}

		err := exec.Execute(atmosConfig, args)
		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrStackRequiredWithComponent)
	})
}
