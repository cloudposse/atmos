package exec

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
		stackName, stackLocals, err := processStackFileForLocals(atmosConfig, validFile, "")
		require.NoError(t, err)
		assert.Equal(t, "valid", stackName)
		assert.NotEmpty(t, stackLocals)
		assert.Contains(t, stackLocals, "global")
	})

	t.Run("file not found", func(t *testing.T) {
		_, _, err := processStackFileForLocals(atmosConfig, "/nonexistent/file.yaml", "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read stack file")
	})

	t.Run("invalid YAML returns nil", func(t *testing.T) {
		stackName, stackLocals, err := processStackFileForLocals(atmosConfig, invalidFile, "")
		require.NoError(t, err)
		assert.Empty(t, stackName)
		assert.Empty(t, stackLocals)
	})

	t.Run("empty file returns nil", func(t *testing.T) {
		stackName, stackLocals, err := processStackFileForLocals(atmosConfig, emptyFile, "")
		require.NoError(t, err)
		assert.Empty(t, stackName)
		assert.Empty(t, stackLocals)
	})

	t.Run("filter by stack name matches", func(t *testing.T) {
		stackName, stackLocals, err := processStackFileForLocals(atmosConfig, validFile, "valid")
		require.NoError(t, err)
		assert.Equal(t, "valid", stackName)
		assert.NotEmpty(t, stackLocals)
	})

	t.Run("filter by stack name does not match", func(t *testing.T) {
		stackName, stackLocals, err := processStackFileForLocals(atmosConfig, validFile, "other-stack")
		require.NoError(t, err)
		assert.Empty(t, stackName)
		assert.Empty(t, stackLocals)
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
			nameTemplate:    "{{ .namespace }}-{{ .environment }}-{{ .stage }}",
			expected:        "acme-dev-us-east-1",
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
			nameTemplate: "{{ .namespace }}-derived",
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
}
