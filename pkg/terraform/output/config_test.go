package output

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
)

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
	tests := []struct {
		name                  string
		sections              map[string]any
		autoGenerateBackend   bool
		initRunReconfigure    bool
		expectError           bool
		expectedErr           error
		expectedExecutable    string
		expectedWorkspace     string
		expectedComponentPath string
		expectedBackendType   string
	}{
		{
			name: "valid minimal config",
			sections: map[string]any{
				cfg.CommandSectionName:   "/usr/bin/terraform",
				cfg.WorkspaceSectionName: "test-ws",
				"component_info": map[string]any{
					"component_path": "/path/to/component",
				},
			},
			autoGenerateBackend:   false,
			initRunReconfigure:    false,
			expectedExecutable:    "/usr/bin/terraform",
			expectedWorkspace:     "test-ws",
			expectedComponentPath: "/path/to/component",
		},
		{
			name: "valid full config",
			sections: map[string]any{
				cfg.CommandSectionName:     "/usr/bin/opentofu",
				cfg.WorkspaceSectionName:   "prod-ws",
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
					"component_path": "/path/to/prod",
				},
			},
			autoGenerateBackend:   true,
			initRunReconfigure:    true,
			expectedExecutable:    "/usr/bin/opentofu",
			expectedWorkspace:     "prod-ws",
			expectedComponentPath: "/path/to/prod",
			expectedBackendType:   "s3",
		},
		{
			name: "missing executable",
			sections: map[string]any{
				cfg.WorkspaceSectionName: "test-ws",
				"component_info": map[string]any{
					"component_path": "/path/to/component",
				},
			},
			expectError: true,
			expectedErr: errUtils.ErrMissingExecutable,
		},
		{
			name: "missing workspace",
			sections: map[string]any{
				cfg.CommandSectionName: "/usr/bin/terraform",
				"component_info": map[string]any{
					"component_path": "/path/to/component",
				},
			},
			expectError: true,
			expectedErr: errUtils.ErrMissingWorkspace,
		},
		{
			name: "missing component_info",
			sections: map[string]any{
				cfg.CommandSectionName:   "/usr/bin/terraform",
				cfg.WorkspaceSectionName: "test-ws",
			},
			expectError: true,
			expectedErr: errUtils.ErrMissingComponentInfo,
		},
		{
			name: "invalid component_info type",
			sections: map[string]any{
				cfg.CommandSectionName:   "/usr/bin/terraform",
				cfg.WorkspaceSectionName: "test-ws",
				"component_info":         "invalid",
			},
			expectError: true,
			expectedErr: errUtils.ErrInvalidComponentInfoS,
		},
		{
			name: "missing component_path in component_info",
			sections: map[string]any{
				cfg.CommandSectionName:   "/usr/bin/terraform",
				cfg.WorkspaceSectionName: "test-ws",
				"component_info": map[string]any{
					"other_field": "value",
				},
			},
			expectError: true,
			expectedErr: errUtils.ErrMissingComponentPath,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := ExtractComponentConfig(tt.sections, "test-component", "test-stack", tt.autoGenerateBackend, tt.initRunReconfigure)

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
			assert.Equal(t, tt.expectedComponentPath, config.ComponentPath)
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
	sections := map[string]any{
		cfg.CommandSectionName:   "/usr/bin/terraform",
		cfg.WorkspaceSectionName: "my-workspace",
		"component_info": map[string]any{
			"component_path": "/components/terraform/vpc",
		},
	}

	config := &ComponentConfig{}
	err := extractRequiredFields(sections, "vpc", "dev", config)

	require.NoError(t, err)
	assert.Equal(t, "/usr/bin/terraform", config.Executable)
	assert.Equal(t, "my-workspace", config.Workspace)
	assert.Equal(t, "/components/terraform/vpc", config.ComponentPath)
}

func TestExtractComponentPath(t *testing.T) {
	tests := []struct {
		name        string
		sections    map[string]any
		expectError bool
		expectedErr error
		expected    string
	}{
		{
			name: "valid component_path",
			sections: map[string]any{
				"component_info": map[string]any{
					"component_path": "/path/to/component",
				},
			},
			expected: "/path/to/component",
		},
		{
			name:        "missing component_info",
			sections:    map[string]any{},
			expectError: true,
			expectedErr: errUtils.ErrMissingComponentInfo,
		},
		{
			name: "invalid component_info type",
			sections: map[string]any{
				"component_info": []string{"invalid"},
			},
			expectError: true,
			expectedErr: errUtils.ErrInvalidComponentInfoS,
		},
		{
			name: "missing component_path in component_info",
			sections: map[string]any{
				"component_info": map[string]any{
					"name": "vpc",
				},
			},
			expectError: true,
			expectedErr: errUtils.ErrMissingComponentPath,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := extractComponentPath(tt.sections, "comp", "stack")
			if tt.expectError {
				require.Error(t, err)
				if tt.expectedErr != nil {
					assert.True(t, errors.Is(err, tt.expectedErr))
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}
