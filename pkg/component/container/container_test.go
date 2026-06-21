package container

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestProvider_GetType(t *testing.T) {
	assert.Equal(t, "container", (&ContainerComponentProvider{}).GetType())
}

func TestProvider_GetGroup(t *testing.T) {
	assert.Equal(t, "Containers", (&ContainerComponentProvider{}).GetGroup())
}

func TestProvider_IsRegistered(t *testing.T) {
	provider, ok := component.GetProvider("container")
	require.True(t, ok)
	assert.Equal(t, "container", provider.GetType())
}

func TestProvider_GetBasePath(t *testing.T) {
	p := &ContainerComponentProvider{}
	tests := []struct {
		name        string
		atmosConfig *schema.AtmosConfiguration
		want        string
	}{
		{name: "nil config", atmosConfig: nil, want: "components/container"},
		{
			name: "configured via plugins map",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{Plugins: map[string]any{
					"container": map[string]any{"base_path": "custom/c"},
				}},
			},
			want: "custom/c",
		},
		{
			name: "empty plugins falls back to default",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{Plugins: map[string]any{}},
			},
			want: "components/container",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, p.GetBasePath(tt.atmosConfig))
		})
	}
}

func TestProvider_ListComponents(t *testing.T) {
	p := &ContainerComponentProvider{}
	stackConfig := map[string]any{
		"components": map[string]any{
			"container": map[string]any{
				"web": map[string]any{},
				"api": map[string]any{},
			},
		},
	}
	names, err := p.ListComponents(context.Background(), "dev", stackConfig)
	require.NoError(t, err)
	assert.Equal(t, []string{"api", "web"}, names) // sorted

	// No container section.
	names, err = p.ListComponents(context.Background(), "dev", map[string]any{"components": map[string]any{}})
	require.NoError(t, err)
	assert.Empty(t, names)
}

func TestProvider_ValidateComponent(t *testing.T) {
	p := &ContainerComponentProvider{}

	require.NoError(t, p.ValidateComponent(nil))
	require.NoError(t, p.ValidateComponent(map[string]any{
		"metadata": map[string]any{"type": "abstract"},
	}))
	require.NoError(t, p.ValidateComponent(map[string]any{
		"image": "alpine",
	}))
	require.NoError(t, p.ValidateComponent(map[string]any{
		"build": map[string]any{"context": "."},
	}))

	// Real component with neither image nor build is invalid. Legacy vars.image
	// is no longer part of the contract — image/build are first-class keys.
	require.Error(t, p.ValidateComponent(map[string]any{"vars": map[string]any{"image": "alpine"}}))
	require.Error(t, p.ValidateComponent(map[string]any{}))
}

func TestProvider_GetAvailableCommands(t *testing.T) {
	commands := (&ContainerComponentProvider{}).GetAvailableCommands()
	assert.Contains(t, commands, "up")
	assert.Contains(t, commands, "down")
	assert.Contains(t, commands, "ps")
	assert.Len(t, commands, 12)
}

func TestProvider_Execute_UnknownSubcommand(t *testing.T) {
	p := &ContainerComponentProvider{}
	err := p.Execute(&component.ExecutionContext{SubCommand: "bogus"})
	require.Error(t, err)
}

func TestProvider_GenerateArtifacts(t *testing.T) {
	require.NoError(t, (&ContainerComponentProvider{}).GenerateArtifacts(&component.ExecutionContext{}))
}
