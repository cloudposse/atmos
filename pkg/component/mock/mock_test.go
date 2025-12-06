package mock

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestMockComponentProvider_GetType(t *testing.T) {
	provider := &MockComponentProvider{}
	assert.Equal(t, "mock", provider.GetType())
}

func TestMockComponentProvider_GetGroup(t *testing.T) {
	provider := &MockComponentProvider{}
	assert.Equal(t, "Testing", provider.GetGroup())
}

func TestMockComponentProvider_GetBasePath(t *testing.T) {
	provider := &MockComponentProvider{}

	tests := []struct {
		name         string
		atmosConfig  *schema.AtmosConfiguration
		expectedPath string
	}{
		{
			name: "with configured base_path",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Plugins: map[string]any{
						"mock": map[string]any{
							"base_path": "custom/mock/path",
							"enabled":   true,
							"tags":      []string{"test"},
						},
					},
				},
			},
			expectedPath: "custom/mock/path",
		},
		{
			name: "with empty Plugins map",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Plugins: map[string]any{},
				},
			},
			expectedPath: "components/mock",
		},
		{
			name: "with nil Plugins",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Plugins: nil,
				},
			},
			expectedPath: "components/mock",
		},
		{
			name: "with invalid config structure",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Plugins: map[string]any{
						"mock": "invalid-string",
					},
				},
			},
			expectedPath: "components/mock",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := provider.GetBasePath(tt.atmosConfig)
			assert.Equal(t, tt.expectedPath, path)
		})
	}
}

func TestMockComponentProvider_ListComponents(t *testing.T) {
	provider := &MockComponentProvider{}

	tests := []struct {
		name          string
		stackConfig   map[string]any
		expectedComps []string
		expectedErr   bool
	}{
		{
			name: "with mock components",
			stackConfig: map[string]any{
				"components": map[string]any{
					"mock": map[string]any{
						"test-component-1": map[string]any{
							"vars": map[string]any{},
						},
						"test-component-2": map[string]any{
							"vars": map[string]any{},
						},
					},
				},
			},
			expectedComps: []string{"test-component-1", "test-component-2"},
			expectedErr:   false,
		},
		{
			name: "without mock section",
			stackConfig: map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{},
				},
			},
			expectedComps: []string{},
			expectedErr:   false,
		},
		{
			name:          "without components section",
			stackConfig:   map[string]any{},
			expectedComps: []string{},
			expectedErr:   false,
		},
		{
			name:          "with nil stack config",
			stackConfig:   nil,
			expectedComps: []string{},
			expectedErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			components, err := provider.ListComponents(context.Background(), "test-stack", tt.stackConfig)

			if tt.expectedErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.ElementsMatch(t, tt.expectedComps, components)
			}
		})
	}
}

func TestMockComponentProvider_ValidateComponent(t *testing.T) {
	provider := &MockComponentProvider{}

	tests := []struct {
		name    string
		config  map[string]any
		wantErr bool
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: false,
		},
		{
			name:    "empty config",
			config:  map[string]any{},
			wantErr: false,
		},
		{
			name: "valid config with vars",
			config: map[string]any{
				"vars": map[string]any{
					"test_mode": true,
				},
			},
			wantErr: false,
		},
		{
			name: "config with invalid flag set",
			config: map[string]any{
				"invalid": true,
			},
			wantErr: true,
		},
		{
			name: "config with invalid flag false",
			config: map[string]any{
				"invalid": false,
			},
			wantErr: false,
		},
		{
			name: "complex valid config",
			config: map[string]any{
				"vars": map[string]any{
					"example": "value",
				},
				"settings": map[string]any{
					"timeout": 120,
				},
				"metadata": map[string]any{
					"component": "test",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := provider.ValidateComponent(tt.config)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "invalid flag set")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestMockComponentProvider_Execute(t *testing.T) {
	provider := &MockComponentProvider{}

	tests := []struct {
		name string
		ctx  component.ExecutionContext
	}{
		{
			name: "basic execution",
			ctx: component.ExecutionContext{
				ComponentType: "mock",
				Component:     "test-component",
				Stack:         "test-stack",
				Command:       "plan",
			},
		},
		{
			name: "execution with subcommand",
			ctx: component.ExecutionContext{
				ComponentType: "mock",
				Component:     "test-component",
				Stack:         "test-stack",
				Command:       "terraform",
				SubCommand:    "plan",
			},
		},
		{
			name: "execution with args and flags",
			ctx: component.ExecutionContext{
				ComponentType: "mock",
				Component:     "test-component",
				Stack:         "test-stack",
				Command:       "apply",
				Args:          []string{"arg1", "arg2"},
				Flags: map[string]any{
					"auto-approve": true,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := provider.Execute(&tt.ctx)
			assert.NoError(t, err)
		})
	}
}

func TestMockComponentProvider_GenerateArtifacts(t *testing.T) {
	provider := &MockComponentProvider{}

	ctx := component.ExecutionContext{
		Component: "test-component",
		Stack:     "test-stack",
	}

	err := provider.GenerateArtifacts(&ctx)
	assert.NoError(t, err)
}

func TestMockComponentProvider_GetAvailableCommands(t *testing.T) {
	provider := &MockComponentProvider{}

	commands := provider.GetAvailableCommands()

	assert.NotEmpty(t, commands)
	assert.Contains(t, commands, "plan")
	assert.Contains(t, commands, "apply")
	assert.Contains(t, commands, "destroy")
	assert.Contains(t, commands, "validate")
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	assert.Equal(t, "components/mock", config.BasePath)
	assert.True(t, config.Enabled)
	assert.False(t, config.DryRun)
	assert.Empty(t, config.Tags)
	assert.Empty(t, config.Metadata)
	assert.Empty(t, config.Dependencies)
}

func TestParseConfig(t *testing.T) {
	tests := []struct {
		name        string
		raw         any
		expected    Config
		expectError bool
	}{
		{
			name: "valid config map with all fields",
			raw: map[string]any{
				"base_path": "custom/path",
				"enabled":   false,
				"dry_run":   true,
				"tags":      []string{"test", "dev"},
				"metadata": map[string]any{
					"owner":   "platform-team",
					"version": "1.0.0",
				},
				"dependencies": []string{"vpc", "database"},
			},
			expected: Config{
				BasePath:     "custom/path",
				Enabled:      false,
				DryRun:       true,
				Tags:         []string{"test", "dev"},
				Metadata:     map[string]any{"owner": "platform-team", "version": "1.0.0"},
				Dependencies: []string{"vpc", "database"},
			},
			expectError: false,
		},
		{
			name: "partial config with base_path only",
			raw: map[string]any{
				"base_path": "custom/path",
			},
			expected: Config{
				BasePath:     "custom/path",
				Enabled:      false,
				DryRun:       false,
				Tags:         nil,
				Metadata:     nil,
				Dependencies: nil,
			},
			expectError: false,
		},
		{
			name:        "empty map",
			raw:         map[string]any{},
			expected:    Config{},
			expectError: false,
		},
		{
			name:        "nil input returns empty config",
			raw:         nil,
			expected:    Config{}, // parseConfig doesn't return default for nil.
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := parseConfig(tt.raw)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected.BasePath, config.BasePath)
				assert.Equal(t, tt.expected.Enabled, config.Enabled)
				assert.Equal(t, tt.expected.DryRun, config.DryRun)
				assert.Equal(t, tt.expected.Tags, config.Tags)
				assert.Equal(t, tt.expected.Metadata, config.Metadata)
				assert.Equal(t, tt.expected.Dependencies, config.Dependencies)
			}
		})
	}
}

// TestMockComponentProvider_Integration tests the provider with realistic scenarios.
func TestMockComponentProvider_Integration(t *testing.T) {
	provider := &MockComponentProvider{}

	// Simulate a complete workflow.
	atmosConfig := &schema.AtmosConfiguration{
		Components: schema.Components{
			Plugins: map[string]any{
				"mock": map[string]any{
					"base_path":    "test/mock",
					"enabled":      true,
					"dry_run":      false,
					"tags":         []string{"integration", "test"},
					"metadata":     map[string]any{"owner": "test-team"},
					"dependencies": []string{},
				},
			},
		},
	}

	// Get base path.
	basePath := provider.GetBasePath(atmosConfig)
	assert.Equal(t, "test/mock", basePath)

	// List components.
	stackConfig := map[string]any{
		"components": map[string]any{
			"mock": map[string]any{
				"component1": map[string]any{},
				"component2": map[string]any{},
			},
		},
	}

	components, err := provider.ListComponents(context.Background(), "test-stack", stackConfig)
	require.NoError(t, err)
	assert.Len(t, components, 2)

	// Validate component.
	componentConfig := map[string]any{
		"vars": map[string]any{
			"test": true,
		},
	}

	err = provider.ValidateComponent(componentConfig)
	assert.NoError(t, err)

	// Execute component.
	ctx := component.ExecutionContext{
		AtmosConfig:   atmosConfig,
		ComponentType: "mock",
		Component:     "component1",
		Stack:         "test-stack",
		Command:       "plan",
	}

	err = provider.Execute(&ctx)
	assert.NoError(t, err)

	// Generate artifacts.
	err = provider.GenerateArtifacts(&ctx)
	assert.NoError(t, err)
}
