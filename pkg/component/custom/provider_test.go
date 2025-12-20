package custom

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewProvider(t *testing.T) {
	p := NewProvider("script", "components/script")

	assert.Equal(t, "script", p.GetType())
	assert.Equal(t, "Custom", p.GetGroup())
	assert.Equal(t, "components/script", p.GetBasePath(nil))
}

func TestProvider_GetBasePath_Default(t *testing.T) {
	// When basePath is empty, it should default to components/<type>.
	p := NewProvider("ansible", "")

	assert.Equal(t, "components/ansible", p.GetBasePath(nil))
}

func TestProvider_GetBasePath_Custom(t *testing.T) {
	p := NewProvider("ansible", "custom/path/ansible")

	assert.Equal(t, "custom/path/ansible", p.GetBasePath(nil))
}

func TestProvider_ListComponents(t *testing.T) {
	p := NewProvider("script", "components/script")

	tests := []struct {
		name        string
		stackConfig map[string]any
		expected    []string
	}{
		{
			name:        "empty stack config",
			stackConfig: map[string]any{},
			expected:    []string{},
		},
		{
			name: "no components section",
			stackConfig: map[string]any{
				"vars": map[string]any{"foo": "bar"},
			},
			expected: []string{},
		},
		{
			name: "no script components",
			stackConfig: map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc": map[string]any{},
					},
				},
			},
			expected: []string{},
		},
		{
			name: "has script components",
			stackConfig: map[string]any{
				"components": map[string]any{
					"script": map[string]any{
						"deploy-app": map[string]any{},
						"backup":     map[string]any{},
					},
				},
			},
			expected: []string{"backup", "deploy-app"}, // Sorted.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			components, err := p.ListComponents(context.Background(), "dev", tt.stackConfig)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, components)
		})
	}
}

func TestProvider_ValidateComponent(t *testing.T) {
	p := NewProvider("script", "components/script")

	// Custom components have no specific validation requirements.
	err := p.ValidateComponent(nil)
	assert.NoError(t, err)

	err = p.ValidateComponent(map[string]any{"foo": "bar"})
	assert.NoError(t, err)
}

func TestProvider_Execute(t *testing.T) {
	p := NewProvider("script", "components/script")

	// Execute is a no-op for custom components.
	err := p.Execute(&component.ExecutionContext{
		AtmosConfig:   &schema.AtmosConfiguration{},
		ComponentType: "script",
		Component:     "deploy-app",
		Stack:         "dev",
	})
	assert.NoError(t, err)
}

func TestProvider_GenerateArtifacts(t *testing.T) {
	p := NewProvider("script", "components/script")

	// GenerateArtifacts is a no-op for custom components.
	err := p.GenerateArtifacts(&component.ExecutionContext{
		AtmosConfig:   &schema.AtmosConfiguration{},
		ComponentType: "script",
		Component:     "deploy-app",
		Stack:         "dev",
	})
	assert.NoError(t, err)
}

func TestProvider_GetAvailableCommands(t *testing.T) {
	p := NewProvider("script", "components/script")

	// Custom components don't define commands - they're defined in the custom command config.
	commands := p.GetAvailableCommands()
	assert.Empty(t, commands)
}

func TestEnsureRegistered(t *testing.T) {
	// Test registering a new type.
	err := EnsureRegistered("test-custom-type", "components/test-custom-type")
	require.NoError(t, err)

	// Verify it was registered.
	provider, ok := component.GetProvider("test-custom-type")
	require.True(t, ok)
	assert.Equal(t, "test-custom-type", provider.GetType())

	// Test idempotency - registering the same type again should succeed.
	err = EnsureRegistered("test-custom-type", "components/test-custom-type")
	require.NoError(t, err)
}

func TestEnsureRegistered_EmptyType(t *testing.T) {
	err := EnsureRegistered("", "components/empty")
	assert.Error(t, err)
}
