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
	// Use t.TempDir() for OS-neutral paths.
	tempDir := t.TempDir()
	stacksBase := filepath.Join(tempDir, "stacks")
	otherDir := filepath.Join(tempDir, "other", "location")

	t.Run("simple file path", func(t *testing.T) {
		atmosConfig := &mockAtmosConfig{stacksBaseAbsolutePath: stacksBase}
		filePath := filepath.Join(stacksBase, "dev.yaml")
		result := deriveStackFileName(atmosConfig.toSchema(), filePath)
		assert.Equal(t, "dev", result)
	})

	t.Run("nested file path", func(t *testing.T) {
		atmosConfig := &mockAtmosConfig{stacksBaseAbsolutePath: stacksBase}
		filePath := filepath.Join(stacksBase, "deploy", "dev.yaml")
		result := deriveStackFileName(atmosConfig.toSchema(), filePath)
		assert.Equal(t, "deploy/dev", result)
	})

	t.Run("deeply nested file path", func(t *testing.T) {
		atmosConfig := &mockAtmosConfig{stacksBaseAbsolutePath: stacksBase}
		filePath := filepath.Join(stacksBase, "org", "team", "deploy", "dev.yaml")
		result := deriveStackFileName(atmosConfig.toSchema(), filePath)
		assert.Equal(t, "org/team/deploy/dev", result)
	})

	t.Run("empty base path falls back to filename", func(t *testing.T) {
		atmosConfig := &mockAtmosConfig{stacksBaseAbsolutePath: ""}
		filePath := filepath.Join(stacksBase, "deploy", "dev.yaml")
		result := deriveStackFileName(atmosConfig.toSchema(), filePath)
		assert.Equal(t, "dev", result)
	})

	t.Run("yml extension", func(t *testing.T) {
		atmosConfig := &mockAtmosConfig{stacksBaseAbsolutePath: stacksBase}
		filePath := filepath.Join(stacksBase, "prod.yml")
		result := deriveStackFileName(atmosConfig.toSchema(), filePath)
		assert.Equal(t, "prod", result)
	})

	t.Run("file path not under base path returns relative path", func(t *testing.T) {
		atmosConfig := &mockAtmosConfig{stacksBaseAbsolutePath: stacksBase}
		filePath := filepath.Join(otherDir, "dev.yaml")
		result := deriveStackFileName(atmosConfig.toSchema(), filePath)
		// Result is normalized with forward slashes and contains ".." to traverse up.
		assert.Contains(t, result, "..")
		assert.Contains(t, result, "dev")
	})
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
			expectKeys: []string{"locals"},
		},
		{
			name: "terraform locals",
			localsCtx: &LocalsContext{
				Global:             map[string]any{"global_key": "global_value"},
				Terraform:          map[string]any{"global_key": "global_value", "tf_key": "tf_value"},
				HasTerraformLocals: true,
			},
			expectKeys: []string{"locals", "terraform"},
		},
		{
			name: "helmfile locals",
			localsCtx: &LocalsContext{
				Global:            map[string]any{"global_key": "global_value"},
				Helmfile:          map[string]any{"global_key": "global_value", "hf_key": "hf_value"},
				HasHelmfileLocals: true,
			},
			expectKeys: []string{"locals", "helmfile"},
		},
		{
			name: "packer locals",
			localsCtx: &LocalsContext{
				Global:          map[string]any{"global_key": "global_value"},
				Packer:          map[string]any{"global_key": "global_value", "pk_key": "pk_value"},
				HasPackerLocals: true,
			},
			expectKeys: []string{"locals", "packer"},
		},
		{
			name: "all sections",
			localsCtx: &LocalsContext{
				Global:             map[string]any{"global_key": "global_value"},
				Terraform:          map[string]any{"global_key": "global_value", "tf_key": "tf_value"},
				Helmfile:           map[string]any{"global_key": "global_value", "hf_key": "hf_value"},
				Packer:             map[string]any{"global_key": "global_value", "pk_key": "pk_value"},
				HasTerraformLocals: true,
				HasHelmfileLocals:  true,
				HasPackerLocals:    true,
			},
			expectKeys: []string{"locals", "terraform", "helmfile", "packer"},
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
			expectKeys: []string{"locals"},
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
		assert.Contains(t, result.StackLocals, "locals")
		assert.True(t, result.Found)
	})

	t.Run("file not found", func(t *testing.T) {
		missingFile := filepath.Join(tempDir, "does-not-exist.yaml")
		_, err := processStackFileForLocals(atmosConfig, missingFile, "")
		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrInvalidStackManifest)
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

	t.Run("filter matching invalid YAML returns error", func(t *testing.T) {
		// When filtering by a stack that has YAML errors, return error instead of silently skipping.
		_, err := processStackFileForLocals(atmosConfig, invalidFile, "invalid")
		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrInvalidStackManifest)
		assert.Contains(t, err.Error(), "failed to parse YAML")
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

	t.Run("returns locals for dev stack in direct format", func(t *testing.T) {
		result, err := ExecuteDescribeLocals(atmosConfig, "dev")
		require.NoError(t, err)
		// Result should be direct format: locals: {...}
		assert.Contains(t, result, "locals")
		locals, ok := result["locals"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "acme", locals["namespace"])
		assert.Equal(t, "dev", locals["environment"])
	})

	t.Run("returns locals for prod stack in direct format", func(t *testing.T) {
		result, err := ExecuteDescribeLocals(atmosConfig, "prod")
		require.NoError(t, err)
		// Result should be direct format: locals: {...}
		assert.Contains(t, result, "locals")
		locals, ok := result["locals"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "acme", locals["namespace"])
		assert.Equal(t, "prod", locals["environment"])
	})

	t.Run("returns error for nonexistent stack", func(t *testing.T) {
		_, err := ExecuteDescribeLocals(atmosConfig, "nonexistent")
		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrStackNotFound)
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
				"locals": map[string]any{
					"namespace": "acme",
				},
				"terraform": map[string]any{
					"locals": map[string]any{
						"backend_bucket": "acme-tfstate",
					},
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
				"locals": map[string]any{
					"namespace": "acme",
				},
				"helmfile": map[string]any{
					"locals": map[string]any{
						"release_dir": "/releases",
					},
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
				"locals": map[string]any{
					"namespace": "acme",
				},
				"packer": map[string]any{
					"locals": map[string]any{
						"ami_name": "my-ami",
					},
				},
			},
			componentType: "packer",
			expected: map[string]any{
				"namespace": "acme",
				"ami_name":  "my-ami",
			},
		},
		{
			name: "falls back to locals only when section not available",
			stackLocals: map[string]any{
				"locals": map[string]any{
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
			name: "returns only global locals when section not available",
			stackLocals: map[string]any{
				"locals": map[string]any{
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
				"locals": map[string]any{
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
	t.Run("requires stack", func(t *testing.T) {
		exec := &describeLocalsExec{}

		args := &DescribeLocalsArgs{
			Component: "vpc",
			// FilterByStack is empty - should error.
		}

		_, err := exec.executeForComponent(&schema.AtmosConfiguration{}, args)
		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrStackRequired)
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
				// New direct format (no stack name wrapper).
				return map[string]any{
					"locals": map[string]any{
						"namespace":   "acme",
						"environment": "dev",
					},
					"terraform": map[string]any{
						"locals": map[string]any{
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

		// Verify Atmos schema format.
		components, ok := result["components"].(map[string]any)
		require.True(t, ok)
		terraform, ok := components["terraform"].(map[string]any)
		require.True(t, ok)
		vpc, ok := terraform["vpc"].(map[string]any)
		require.True(t, ok)
		locals, ok := vpc["locals"].(map[string]any)
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
				// New direct format (no stack name wrapper).
				return map[string]any{
					"locals": map[string]any{
						"namespace": "acme",
					},
					"helmfile": map[string]any{
						"locals": map[string]any{
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

		// Verify Atmos schema format.
		components, ok := result["components"].(map[string]any)
		require.True(t, ok)
		helmfile, ok := components["helmfile"].(map[string]any)
		require.True(t, ok)
		nginx, ok := helmfile["nginx"].(map[string]any)
		require.True(t, ok)
		locals, ok := nginx["locals"].(map[string]any)
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
				// New direct format (no stack name wrapper).
				return map[string]any{
					"locals": map[string]any{
						"namespace": "acme",
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

		// Should default to terraform - check in Atmos schema format.
		components, ok := result["components"].(map[string]any)
		require.True(t, ok)
		_, hasTerraform := components["terraform"]
		assert.True(t, hasTerraform, "should default to terraform component type")
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
				// New direct format (no stack name wrapper).
				return map[string]any{
					"locals": map[string]any{
						"namespace": "acme",
					},
					"terraform": map[string]any{
						"locals": map[string]any{
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

		// Verify component output uses Atmos schema format.
		// Expected: components: { terraform: { vpc: { locals: {...} } } }
		assert.Contains(t, result, "components", "component output should have 'components' key")

		components, ok := result["components"].(map[string]any)
		require.True(t, ok)
		terraform, ok := components["terraform"].(map[string]any)
		require.True(t, ok)
		vpc, ok := terraform["vpc"].(map[string]any)
		require.True(t, ok)
		assert.Contains(t, vpc, "locals", "component output should have 'locals' key")

		// Verify locals contain merged values.
		locals, ok := vpc["locals"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "acme", locals["namespace"])
		assert.Equal(t, "acme-tfstate", locals["backend_bucket"])
	})

	t.Run("component output works with direct format", func(t *testing.T) {
		exec := &describeLocalsExec{
			executeDescribeComponent: func(params *ExecuteDescribeComponentParams) (map[string]any, error) {
				// Filter should be "deploy/prod" (file path).
				assert.Equal(t, "deploy/prod", params.Stack)
				return map[string]any{
					"component_type": "terraform",
				}, nil
			},
			executeDescribeLocals: func(ac *schema.AtmosConfiguration, filterByStack string) (map[string]any, error) {
				// New direct format (no stack name wrapper).
				return map[string]any{
					"locals": map[string]any{
						"namespace": "acme",
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

		// Verify component output uses Atmos schema format.
		assert.Contains(t, result, "components")
		components, ok := result["components"].(map[string]any)
		require.True(t, ok)
		terraform, ok := components["terraform"].(map[string]any)
		require.True(t, ok)
		assert.Contains(t, terraform, "vpc")
	})
}

func TestDescribeLocalsExecExecute(t *testing.T) {
	t.Run("execute requires stack", func(t *testing.T) {
		exec := &describeLocalsExec{}

		args := &DescribeLocalsArgs{
			Format: "yaml",
			// FilterByStack is empty - should error.
		}

		err := exec.Execute(&schema.AtmosConfiguration{}, args)
		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrStackRequired)
	})

	t.Run("execute with stack", func(t *testing.T) {
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
			Format:        "yaml",
			FilterByStack: "dev",
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
			Format:        "yaml",
			FilterByStack: "dev",
			Query:         ".dev.locals.namespace",
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
			Format:        "json",
			FilterByStack: "dev",
			File:          outputFile,
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
			Format:        "yaml",
			FilterByStack: "dev",
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
				// New direct format (no stack name wrapper).
				return map[string]any{
					"locals": map[string]any{
						"namespace": "acme",
					},
					"terraform": map[string]any{
						"locals": map[string]any{
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

		// Verify the output structure uses Atmos schema format.
		result, ok := capturedData.(map[string]any)
		require.True(t, ok)
		// New format: components: { terraform: { vpc: { locals: {...} } } }
		components, ok := result["components"].(map[string]any)
		require.True(t, ok)
		terraform, ok := components["terraform"].(map[string]any)
		require.True(t, ok)
		vpc, ok := terraform["vpc"].(map[string]any)
		require.True(t, ok)
		assert.Contains(t, vpc, "locals")
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
		assert.ErrorIs(t, err, errUtils.ErrStackRequired)
	})
}

func TestGetComponentType(t *testing.T) {
	tests := []struct {
		name             string
		componentSection map[string]any
		expected         string
	}{
		{
			name:             "returns terraform for terraform type",
			componentSection: map[string]any{"component_type": "terraform"},
			expected:         "terraform",
		},
		{
			name:             "returns helmfile for helmfile type",
			componentSection: map[string]any{"component_type": "helmfile"},
			expected:         "helmfile",
		},
		{
			name:             "returns packer for packer type",
			componentSection: map[string]any{"component_type": "packer"},
			expected:         "packer",
		},
		{
			name:             "defaults to terraform when not set",
			componentSection: map[string]any{},
			expected:         "terraform",
		},
		{
			name:             "defaults to terraform for nil map",
			componentSection: nil,
			expected:         "terraform",
		},
		{
			name:             "defaults to terraform for non-string type",
			componentSection: map[string]any{"component_type": 123},
			expected:         "terraform",
		},
		{
			name:             "defaults to terraform for empty string",
			componentSection: map[string]any{"component_type": ""},
			expected:         "terraform",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getComponentType(tt.componentSection)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractComponentLocals(t *testing.T) {
	tests := []struct {
		name             string
		componentSection map[string]any
		expected         map[string]any
	}{
		{
			name: "extracts locals from component section",
			componentSection: map[string]any{
				"locals": map[string]any{
					"key1": "value1",
					"key2": "value2",
				},
			},
			expected: map[string]any{
				"key1": "value1",
				"key2": "value2",
			},
		},
		{
			name:             "returns nil when no locals",
			componentSection: map[string]any{},
			expected:         nil,
		},
		{
			name:             "returns nil for nil section",
			componentSection: nil,
			expected:         nil,
		},
		{
			name: "returns nil when locals is not a map",
			componentSection: map[string]any{
				"locals": "not a map",
			},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractComponentLocals(tt.componentSection)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildComponentSchemaOutput(t *testing.T) {
	tests := []struct {
		name          string
		component     string
		componentType string
		locals        map[string]any
	}{
		{
			name:          "builds terraform component output",
			component:     "vpc",
			componentType: "terraform",
			locals:        map[string]any{"namespace": "acme"},
		},
		{
			name:          "builds helmfile component output",
			component:     "nginx",
			componentType: "helmfile",
			locals:        map[string]any{"release": "v1"},
		},
		{
			name:          "builds output with empty locals",
			component:     "test",
			componentType: "terraform",
			locals:        map[string]any{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildComponentSchemaOutput(tt.component, tt.componentType, tt.locals)

			// Verify structure: components -> componentType -> component -> locals.
			components, ok := result["components"].(map[string]any)
			require.True(t, ok)
			typeSection, ok := components[tt.componentType].(map[string]any)
			require.True(t, ok)
			compSection, ok := typeSection[tt.component].(map[string]any)
			require.True(t, ok)
			assert.Contains(t, compSection, "locals")
		})
	}
}

func TestMergeLocals(t *testing.T) {
	tests := []struct {
		name     string
		base     map[string]any
		override map[string]any
		expected map[string]any
	}{
		{
			name:     "merges two maps",
			base:     map[string]any{"key1": "value1"},
			override: map[string]any{"key2": "value2"},
			expected: map[string]any{"key1": "value1", "key2": "value2"},
		},
		{
			name:     "override takes precedence",
			base:     map[string]any{"key": "base"},
			override: map[string]any{"key": "override"},
			expected: map[string]any{"key": "override"},
		},
		{
			name:     "deep merges nested maps",
			base:     map[string]any{"nested": map[string]any{"a": 1, "b": 2}},
			override: map[string]any{"nested": map[string]any{"b": 3, "c": 4}},
			expected: map[string]any{"nested": map[string]any{"a": 1, "b": 3, "c": 4}},
		},
		{
			name:     "handles nil base",
			base:     nil,
			override: map[string]any{"key": "value"},
			expected: map[string]any{"key": "value"},
		},
		{
			name:     "handles nil override",
			base:     map[string]any{"key": "value"},
			override: nil,
			expected: map[string]any{"key": "value"},
		},
		{
			name:     "handles both nil",
			base:     nil,
			override: nil,
			expected: map[string]any{},
		},
		{
			name:     "handles empty maps",
			base:     map[string]any{},
			override: map[string]any{},
			expected: map[string]any{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mergeLocals(tt.base, tt.override)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseStackFileYAML(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("parses valid YAML", func(t *testing.T) {
		validYAML := `
locals:
  key: value
`
		validFile := filepath.Join(tempDir, "valid.yaml")
		err := os.WriteFile(validFile, []byte(validYAML), 0o644)
		require.NoError(t, err)

		result, err := parseStackFileYAML(validFile, false)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Contains(t, result, "locals")
	})

	t.Run("returns error for file not found", func(t *testing.T) {
		missingFile := filepath.Join(tempDir, "does-not-exist.yaml")
		_, err := parseStackFileYAML(missingFile, false)
		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrInvalidStackManifest)
	})

	t.Run("returns nil for invalid YAML when not filtering", func(t *testing.T) {
		invalidYAML := `invalid: yaml: [broken`
		invalidFile := filepath.Join(tempDir, "invalid.yaml")
		err := os.WriteFile(invalidFile, []byte(invalidYAML), 0o644)
		require.NoError(t, err)

		result, err := parseStackFileYAML(invalidFile, false)
		require.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("returns error for invalid YAML when filtering", func(t *testing.T) {
		invalidYAML := `invalid: yaml: [broken`
		invalidFile := filepath.Join(tempDir, "invalid_filter.yaml")
		err := os.WriteFile(invalidFile, []byte(invalidYAML), 0o644)
		require.NoError(t, err)

		_, err = parseStackFileYAML(invalidFile, true)
		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrInvalidStackManifest)
	})

	t.Run("returns nil for empty file", func(t *testing.T) {
		emptyFile := filepath.Join(tempDir, "empty.yaml")
		err := os.WriteFile(emptyFile, []byte(""), 0o644)
		require.NoError(t, err)

		result, err := parseStackFileYAML(emptyFile, false)
		require.NoError(t, err)
		assert.Nil(t, result)
	})
}

func TestStackMatchesFilter(t *testing.T) {
	tests := []struct {
		name          string
		filterByStack string
		stackFileName string
		stackName     string
		expected      bool
	}{
		{
			name:          "matches by filename",
			filterByStack: "deploy/dev",
			stackFileName: "deploy/dev",
			stackName:     "dev-us-east-1",
			expected:      true,
		},
		{
			name:          "matches by derived name",
			filterByStack: "dev-us-east-1",
			stackFileName: "deploy/dev",
			stackName:     "dev-us-east-1",
			expected:      true,
		},
		{
			name:          "no match",
			filterByStack: "prod",
			stackFileName: "deploy/dev",
			stackName:     "dev-us-east-1",
			expected:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stackMatchesFilter(tt.filterByStack, tt.stackFileName, tt.stackName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetSectionOnlyLocals(t *testing.T) {
	tests := []struct {
		name          string
		sectionLocals map[string]any
		globalLocals  map[string]any
		expected      map[string]any
	}{
		{
			name:          "returns section-only keys",
			sectionLocals: map[string]any{"global": "value", "section_only": "value2"},
			globalLocals:  map[string]any{"global": "value"},
			expected:      map[string]any{"section_only": "value2"},
		},
		{
			name:          "returns empty when all keys are global",
			sectionLocals: map[string]any{"global": "value"},
			globalLocals:  map[string]any{"global": "value"},
			expected:      map[string]any{},
		},
		{
			name:          "returns all when no global",
			sectionLocals: map[string]any{"key1": "value1", "key2": "value2"},
			globalLocals:  map[string]any{},
			expected:      map[string]any{"key1": "value1", "key2": "value2"},
		},
		{
			name:          "handles nil section",
			sectionLocals: nil,
			globalLocals:  map[string]any{"key": "value"},
			expected:      map[string]any{},
		},
		{
			name:          "handles nil global",
			sectionLocals: map[string]any{"key": "value"},
			globalLocals:  nil,
			expected:      map[string]any{"key": "value"},
		},
		{
			name:          "excludes keys with same value",
			sectionLocals: map[string]any{"same": "value", "different": "new"},
			globalLocals:  map[string]any{"same": "value", "different": "old"},
			expected:      map[string]any{"different": "new"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getSectionOnlyLocals(tt.sectionLocals, tt.globalLocals)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValuesEqual(t *testing.T) {
	tests := []struct {
		name     string
		a        any
		b        any
		expected bool
	}{
		{
			name:     "equal strings",
			a:        "value",
			b:        "value",
			expected: true,
		},
		{
			name:     "different strings",
			a:        "value1",
			b:        "value2",
			expected: false,
		},
		{
			name:     "equal ints",
			a:        123,
			b:        123,
			expected: true,
		},
		{
			name:     "different ints",
			a:        123,
			b:        456,
			expected: false,
		},
		{
			name:     "equal maps",
			a:        map[string]any{"key": "value"},
			b:        map[string]any{"key": "value"},
			expected: true,
		},
		{
			name:     "different maps",
			a:        map[string]any{"key": "value1"},
			b:        map[string]any{"key": "value2"},
			expected: false,
		},
		{
			name:     "equal slices",
			a:        []any{1, 2, 3},
			b:        []any{1, 2, 3},
			expected: true,
		},
		{
			name:     "different slices",
			a:        []any{1, 2, 3},
			b:        []any{1, 2, 4},
			expected: false,
		},
		{
			name:     "nil values",
			a:        nil,
			b:        nil,
			expected: true,
		},
		{
			name:     "one nil",
			a:        "value",
			b:        nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := valuesEqual(tt.a, tt.b)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetExplicitStackName(t *testing.T) {
	tests := []struct {
		name            string
		stackSectionMap map[string]any
		expected        string
	}{
		{
			name:            "returns explicit name",
			stackSectionMap: map[string]any{"name": "my-stack"},
			expected:        "my-stack",
		},
		{
			name:            "returns empty for missing name",
			stackSectionMap: map[string]any{},
			expected:        "",
		},
		{
			name:            "returns empty for nil map",
			stackSectionMap: nil,
			expected:        "",
		},
		{
			name:            "returns empty for non-string name",
			stackSectionMap: map[string]any{"name": 123},
			expected:        "",
		},
		{
			name:            "returns empty string name as-is",
			stackSectionMap: map[string]any{"name": ""},
			expected:        "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getExplicitStackName(tt.stackSectionMap)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildComponentLocalsResult(t *testing.T) {
	t.Run("direct format with locals succeeds", func(t *testing.T) {
		args := &DescribeLocalsArgs{
			Component:     "vpc",
			FilterByStack: "dev",
		}
		// New direct format (no stack name wrapper).
		stackLocals := map[string]any{
			"locals": map[string]any{"namespace": "acme"},
		}

		result, err := buildComponentLocalsResult(args, stackLocals, "terraform", nil)
		require.NoError(t, err)
		assert.Contains(t, result, "components")
	})

	t.Run("direct format with section-specific locals", func(t *testing.T) {
		args := &DescribeLocalsArgs{
			Component:     "vpc",
			FilterByStack: "deploy/prod",
		}
		// New direct format with terraform section.
		stackLocals := map[string]any{
			"locals": map[string]any{"namespace": "acme"},
			"terraform": map[string]any{
				"locals": map[string]any{"backend_bucket": "acme-tfstate"},
			},
		}

		result, err := buildComponentLocalsResult(args, stackLocals, "terraform", nil)
		require.NoError(t, err)
		assert.Contains(t, result, "components")

		// Verify merged locals include both global and terraform-specific.
		components, ok := result["components"].(map[string]any)
		require.True(t, ok)
		terraform, ok := components["terraform"].(map[string]any)
		require.True(t, ok)
		vpc, ok := terraform["vpc"].(map[string]any)
		require.True(t, ok)
		locals, ok := vpc["locals"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "acme", locals["namespace"])
		assert.Equal(t, "acme-tfstate", locals["backend_bucket"])
	})

	t.Run("uses component locals when stack has no locals", func(t *testing.T) {
		args := &DescribeLocalsArgs{
			Component:     "vpc",
			FilterByStack: "dev",
		}
		componentLocals := map[string]any{"component_key": "value"}

		result, err := buildComponentLocalsResult(args, map[string]any{}, "terraform", componentLocals)
		require.NoError(t, err)

		components, ok := result["components"].(map[string]any)
		require.True(t, ok)
		terraform, ok := components["terraform"].(map[string]any)
		require.True(t, ok)
		vpc, ok := terraform["vpc"].(map[string]any)
		require.True(t, ok)
		locals, ok := vpc["locals"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "value", locals["component_key"])
	})

	t.Run("returns error when no locals available", func(t *testing.T) {
		args := &DescribeLocalsArgs{
			Component:     "vpc",
			FilterByStack: "dev",
		}

		_, err := buildComponentLocalsResult(args, map[string]any{}, "terraform", nil)
		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrStackHasNoLocals)
	})
}

func TestExecuteDescribeLocalsWindowsPathNormalization(t *testing.T) {
	// Test that Windows-style paths are normalized.
	tempDir := t.TempDir()

	devYAML := `
locals:
  namespace: acme
`
	// Create nested directory structure.
	deployDir := filepath.Join(tempDir, "deploy")
	err := os.MkdirAll(deployDir, 0o755)
	require.NoError(t, err)

	devFile := filepath.Join(deployDir, "dev.yaml")
	err = os.WriteFile(devFile, []byte(devYAML), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath:        tempDir,
		StackConfigFilesAbsolutePaths: []string{devFile},
	}

	// Use forward slashes (as would be normalized from Windows backslashes).
	result, err := ExecuteDescribeLocals(atmosConfig, "deploy/dev")
	require.NoError(t, err)
	// Result should be in direct format (locals: {...}).
	assert.Contains(t, result, "locals")
	locals, ok := result["locals"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "acme", locals["namespace"])
}
