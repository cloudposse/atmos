package output

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// testAtmosConfig returns a minimal atmos configuration for testing.
func testAtmosConfig(basePath string, autoGenerateBackend, initRunReconfigure bool) *schema.AtmosConfiguration {
	return &schema.AtmosConfiguration{
		BasePath: basePath,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath:                "components/terraform",
				AutoGenerateBackendFile: autoGenerateBackend,
				InitRunReconfigure:      initRunReconfigure,
			},
		},
	}
}

func TestIsComponentProcessable(t *testing.T) {
	tests := []struct {
		name             string
		sections         map[string]any
		expectedEnabled  bool
		expectedAbstract bool
	}{
		{
			name:             "empty sections - defaults to enabled, not abstract",
			sections:         map[string]any{},
			expectedEnabled:  true,
			expectedAbstract: false,
		},
		{
			name: "enabled true in vars",
			sections: map[string]any{
				cfg.VarsSectionName: map[string]any{
					"enabled": true,
				},
			},
			expectedEnabled:  true,
			expectedAbstract: false,
		},
		{
			name: "enabled false in vars",
			sections: map[string]any{
				cfg.VarsSectionName: map[string]any{
					"enabled": false,
				},
			},
			expectedEnabled:  false,
			expectedAbstract: false,
		},
		{
			name: "abstract component",
			sections: map[string]any{
				cfg.MetadataSectionName: map[string]any{
					"type": "abstract",
				},
			},
			expectedEnabled:  true,
			expectedAbstract: true,
		},
		{
			name: "real component (not abstract)",
			sections: map[string]any{
				cfg.MetadataSectionName: map[string]any{
					"type": "real",
				},
			},
			expectedEnabled:  true,
			expectedAbstract: false,
		},
		{
			name: "disabled and abstract",
			sections: map[string]any{
				cfg.VarsSectionName: map[string]any{
					"enabled": false,
				},
				cfg.MetadataSectionName: map[string]any{
					"type": "abstract",
				},
			},
			expectedEnabled:  false,
			expectedAbstract: true,
		},
		{
			name: "invalid vars section type - defaults to enabled",
			sections: map[string]any{
				cfg.VarsSectionName: "invalid",
			},
			expectedEnabled:  true,
			expectedAbstract: false,
		},
		{
			name: "invalid metadata section type - defaults to not abstract",
			sections: map[string]any{
				cfg.MetadataSectionName: "invalid",
			},
			expectedEnabled:  true,
			expectedAbstract: false,
		},
		{
			name: "invalid enabled type - defaults to enabled",
			sections: map[string]any{
				cfg.VarsSectionName: map[string]any{
					"enabled": "yes",
				},
			},
			expectedEnabled:  true,
			expectedAbstract: false,
		},
		{
			name: "invalid type field - defaults to not abstract",
			sections: map[string]any{
				cfg.MetadataSectionName: map[string]any{
					"type": 123,
				},
			},
			expectedEnabled:  true,
			expectedAbstract: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enabled, abstract := IsComponentProcessable(tt.sections)
			assert.Equal(t, tt.expectedEnabled, enabled, "enabled mismatch")
			assert.Equal(t, tt.expectedAbstract, abstract, "abstract mismatch")
		})
	}
}

func TestExtractComponentConfig(t *testing.T) {
	// Use a temp directory for cross-platform compatibility.
	tempDir := t.TempDir()

	tests := []struct {
		name                        string
		basePath                    string
		sections                    map[string]any
		autoGenerateBackend         bool
		initRunReconfigure          bool
		expectError                 bool
		expectedErr                 error
		expectedExecutable          string
		expectedWorkspace           string
		expectedComponentPathSuffix string // Use suffix for cross-platform compatibility.
		expectedBackendType         string
	}{
		{
			name:     "valid minimal config",
			basePath: tempDir,
			sections: map[string]any{
				cfg.CommandSectionName:   "/usr/bin/terraform",
				cfg.WorkspaceSectionName: "test-ws",
				cfg.ComponentSectionName: "vpc",
				"component_info": map[string]any{
					"component_type": "terraform",
				},
			},
			autoGenerateBackend:         false,
			initRunReconfigure:          false,
			expectedExecutable:          "/usr/bin/terraform",
			expectedWorkspace:           "test-ws",
			expectedComponentPathSuffix: filepath.Join("components", "terraform", "vpc"),
		},
		{
			name:     "valid full config",
			basePath: tempDir,
			sections: map[string]any{
				cfg.CommandSectionName:     "/usr/bin/opentofu",
				cfg.WorkspaceSectionName:   "prod-ws",
				cfg.ComponentSectionName:   "database",
				cfg.BackendTypeSectionName: "s3",
				cfg.BackendSectionName: map[string]any{
					"bucket": "my-bucket",
					"key":    "state.tfstate",
				},
				cfg.ProvidersSectionName: map[string]any{
					"aws": map[string]any{
						"region": "us-west-2",
					},
				},
				cfg.EnvSectionName: map[string]any{
					"AWS_REGION": "us-west-2",
				},
				"component_info": map[string]any{
					"component_type": "terraform",
				},
			},
			autoGenerateBackend:         true,
			initRunReconfigure:          true,
			expectedExecutable:          "/usr/bin/opentofu",
			expectedWorkspace:           "prod-ws",
			expectedComponentPathSuffix: filepath.Join("components", "terraform", "database"),
			expectedBackendType:         "s3",
		},
		{
			name:     "missing executable",
			basePath: tempDir,
			sections: map[string]any{
				cfg.WorkspaceSectionName: "test-ws",
				cfg.ComponentSectionName: "vpc",
				"component_info": map[string]any{
					"component_type": "terraform",
				},
			},
			expectError: true,
			expectedErr: errUtils.ErrMissingExecutable,
		},
		{
			name:     "missing workspace",
			basePath: tempDir,
			sections: map[string]any{
				cfg.CommandSectionName:   "/usr/bin/terraform",
				cfg.ComponentSectionName: "vpc",
				"component_info": map[string]any{
					"component_type": "terraform",
				},
			},
			expectError: true,
			expectedErr: errUtils.ErrMissingWorkspace,
		},
		{
			name:     "missing component_info",
			basePath: tempDir,
			sections: map[string]any{
				cfg.CommandSectionName:   "/usr/bin/terraform",
				cfg.WorkspaceSectionName: "test-ws",
				cfg.ComponentSectionName: "vpc",
			},
			expectError: true,
			expectedErr: errUtils.ErrMissingComponentInfo,
		},
		{
			name:     "invalid component_info type",
			basePath: tempDir,
			sections: map[string]any{
				cfg.CommandSectionName:   "/usr/bin/terraform",
				cfg.WorkspaceSectionName: "test-ws",
				cfg.ComponentSectionName: "vpc",
				"component_info":         "invalid",
			},
			expectError: true,
			expectedErr: errUtils.ErrInvalidComponentInfoS,
		},
		{
			name:     "missing base component name",
			basePath: tempDir,
			sections: map[string]any{
				cfg.CommandSectionName:   "/usr/bin/terraform",
				cfg.WorkspaceSectionName: "test-ws",
				"component_info": map[string]any{
					"component_type": "terraform",
				},
			},
			expectError: true,
			expectedErr: errUtils.ErrMissingComponentPath,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := testAtmosConfig(tt.basePath, tt.autoGenerateBackend, tt.initRunReconfigure)
			config, err := ExtractComponentConfig(atmosConfig, tt.sections, "test-component", "test-stack")

			if tt.expectError {
				require.Error(t, err)
				if tt.expectedErr != nil {
					assert.True(t, errors.Is(err, tt.expectedErr), "expected %v, got %v", tt.expectedErr, err)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, config)
			assert.Equal(t, tt.expectedExecutable, config.Executable)
			assert.Equal(t, tt.expectedWorkspace, config.Workspace)
			// Use suffix check for cross-platform compatibility.
			assert.True(t, filepath.IsAbs(config.ComponentPath), "expected absolute path, got %s", config.ComponentPath)
			assert.Contains(t, filepath.ToSlash(config.ComponentPath), filepath.ToSlash(tt.expectedComponentPathSuffix),
				"expected path to contain %s, got %s", tt.expectedComponentPathSuffix, config.ComponentPath)
			assert.Equal(t, tt.autoGenerateBackend, config.AutoGenerateBackend)
			assert.Equal(t, tt.initRunReconfigure, config.InitRunReconfigure)

			if tt.expectedBackendType != "" {
				assert.Equal(t, tt.expectedBackendType, config.BackendType)
			}
		})
	}
}

func TestValidateBackendConfig(t *testing.T) {
	tests := []struct {
		name        string
		config      *ComponentConfig
		expectError bool
	}{
		{
			name: "valid backend config",
			config: &ComponentConfig{
				BackendType: "s3",
				Backend: map[string]any{
					"bucket": "test-bucket",
				},
			},
			expectError: false,
		},
		{
			name: "missing backend type",
			config: &ComponentConfig{
				BackendType: "",
				Backend: map[string]any{
					"bucket": "test-bucket",
				},
			},
			expectError: true,
		},
		{
			name: "missing backend config",
			config: &ComponentConfig{
				BackendType: "s3",
				Backend:     nil,
			},
			expectError: true,
		},
		{
			name: "both missing",
			config: &ComponentConfig{
				BackendType: "",
				Backend:     nil,
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBackendConfig(tt.config, "test-component", "test-stack")
			if tt.expectError {
				require.Error(t, err)
				assert.True(t, errors.Is(err, errUtils.ErrBackendFileGeneration))
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestGetComponentInfo(t *testing.T) {
	result := GetComponentInfo("vpc", "dev-us-west-2")
	assert.Equal(t, "component 'vpc' in stack 'dev-us-west-2'", result)

	result = GetComponentInfo("database", "prod-eu-central-1")
	assert.Equal(t, "component 'database' in stack 'prod-eu-central-1'", result)
}

func TestExtractOptionalFields(t *testing.T) {
	sections := map[string]any{
		cfg.BackendTypeSectionName: "gcs",
		cfg.BackendSectionName: map[string]any{
			"bucket": "gcs-bucket",
		},
		cfg.ProvidersSectionName: map[string]any{
			"google": map[string]any{
				"project": "my-project",
			},
		},
		cfg.EnvSectionName: map[string]any{
			"GOOGLE_PROJECT": "my-project",
		},
	}

	config := &ComponentConfig{}
	extractOptionalFields(sections, config)

	assert.Equal(t, "gcs", config.BackendType)
	assert.Equal(t, map[string]any{"bucket": "gcs-bucket"}, config.Backend)
	assert.Equal(t, map[string]any{"google": map[string]any{"project": "my-project"}}, config.Providers)
	assert.Equal(t, map[string]any{"GOOGLE_PROJECT": "my-project"}, config.Env)
}

func TestExtractOptionalFields_Empty(t *testing.T) {
	sections := map[string]any{}
	config := &ComponentConfig{}

	extractOptionalFields(sections, config)

	assert.Empty(t, config.BackendType)
	assert.Nil(t, config.Backend)
	assert.Nil(t, config.Providers)
	assert.Nil(t, config.Env)
}

func TestExtractOptionalFields_InvalidTypes(t *testing.T) {
	sections := map[string]any{
		cfg.BackendTypeSectionName: 123,
		cfg.BackendSectionName:     "invalid",
		cfg.ProvidersSectionName:   []string{"invalid"},
		cfg.EnvSectionName:         true,
	}

	config := &ComponentConfig{}
	extractOptionalFields(sections, config)

	// Invalid types should be skipped.
	assert.Empty(t, config.BackendType)
	assert.Nil(t, config.Backend)
	assert.Nil(t, config.Providers)
	assert.Nil(t, config.Env)
}

func TestExtractRequiredFields(t *testing.T) {
	tempDir := t.TempDir()
	atmosConfig := testAtmosConfig(tempDir, false, false)

	sections := map[string]any{
		cfg.CommandSectionName:   "/usr/bin/terraform",
		cfg.WorkspaceSectionName: "my-workspace",
		cfg.ComponentSectionName: "vpc",
		"component_info": map[string]any{
			"component_type": "terraform",
		},
	}

	config := &ComponentConfig{}
	err := extractRequiredFields(atmosConfig, sections, "vpc", "dev", config)

	require.NoError(t, err)
	assert.Equal(t, "/usr/bin/terraform", config.Executable)
	assert.Equal(t, "my-workspace", config.Workspace)
	// Verify the path is absolute and contains expected suffix.
	assert.True(t, filepath.IsAbs(config.ComponentPath), "expected absolute path")
	expectedSuffix := filepath.Join("components", "terraform", "vpc")
	assert.Contains(t, filepath.ToSlash(config.ComponentPath), filepath.ToSlash(expectedSuffix))
}

func TestExtractComponentPath(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name           string
		basePath       string
		sections       map[string]any
		expectError    bool
		expectedErr    error
		expectedSuffix string // Use suffix for cross-platform compatibility.
	}{
		{
			name:     "valid component path with terraform type",
			basePath: tempDir,
			sections: map[string]any{
				cfg.ComponentSectionName: "vpc",
				"component_info": map[string]any{
					"component_type": "terraform",
				},
			},
			expectedSuffix: filepath.Join("components", "terraform", "vpc"),
		},
		{
			name:     "component with folder prefix",
			basePath: tempDir,
			sections: map[string]any{
				cfg.ComponentSectionName: "mycomponent",
				cfg.MetadataSectionName: map[string]any{
					"component_folder_prefix": "custom",
				},
				"component_info": map[string]any{
					"component_type": "terraform",
				},
			},
			expectedSuffix: filepath.Join("components", "terraform", "custom", "mycomponent"),
		},
		{
			name:     "defaults to terraform type when not specified",
			basePath: tempDir,
			sections: map[string]any{
				cfg.ComponentSectionName: "vpc",
				"component_info":         map[string]any{},
			},
			expectedSuffix: filepath.Join("components", "terraform", "vpc"),
		},
		{
			name:        "missing component_info",
			basePath:    tempDir,
			sections:    map[string]any{cfg.ComponentSectionName: "vpc"},
			expectError: true,
			expectedErr: errUtils.ErrMissingComponentInfo,
		},
		{
			name:     "invalid component_info type",
			basePath: tempDir,
			sections: map[string]any{
				cfg.ComponentSectionName: "vpc",
				"component_info":         []string{"invalid"},
			},
			expectError: true,
			expectedErr: errUtils.ErrInvalidComponentInfoS,
		},
		{
			name:     "missing base component name",
			basePath: tempDir,
			sections: map[string]any{
				"component_info": map[string]any{
					"component_type": "terraform",
				},
			},
			expectError: true,
			expectedErr: errUtils.ErrMissingComponentPath,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := testAtmosConfig(tt.basePath, false, false)
			result, err := extractComponentPath(atmosConfig, tt.sections, "comp", "stack")
			if tt.expectError {
				require.Error(t, err)
				if tt.expectedErr != nil {
					assert.True(t, errors.Is(err, tt.expectedErr), "expected %v, got %v", tt.expectedErr, err)
				}
			} else {
				require.NoError(t, err)
				// Verify the path is absolute and contains expected suffix.
				assert.True(t, filepath.IsAbs(result), "expected absolute path, got %s", result)
				assert.Contains(t, filepath.ToSlash(result), filepath.ToSlash(tt.expectedSuffix),
					"expected path to contain %s, got %s", tt.expectedSuffix, result)
			}
		})
	}
}
