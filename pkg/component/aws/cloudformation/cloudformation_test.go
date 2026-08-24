package cloudformation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestComponentProvider_GetType(t *testing.T) {
	p := &ComponentProvider{}
	assert.Equal(t, cfg.CloudFormationComponentType, p.GetType())
	assert.Equal(t, "aws/cloudformation", p.GetType())
}

func TestComponentProvider_GetGroup(t *testing.T) {
	p := &ComponentProvider{}
	assert.Equal(t, "AWS", p.GetGroup())
}

func TestComponentProvider_GetBasePath(t *testing.T) {
	p := &ComponentProvider{}

	assert.Equal(t, DefaultConfig().BasePath, p.GetBasePath(nil))
	assert.Equal(t, DefaultConfig().BasePath, p.GetBasePath(&schema.AtmosConfiguration{}))

	configured := &schema.AtmosConfiguration{
		Components: schema.Components{
			CloudFormation: schema.AwsCloudFormation{BasePath: "custom/cfn"},
		},
	}
	assert.Equal(t, "custom/cfn", p.GetBasePath(configured))
}

func TestComponentProvider_ListComponents(t *testing.T) {
	p := &ComponentProvider{}

	names, err := p.ListComponents(context.Background(), "dev", map[string]any{
		"components": map[string]any{
			"aws/cloudformation": map[string]any{
				"vpc": map[string]any{},
				"dns": map[string]any{},
			},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"dns", "vpc"}, names)
}

func TestComponentProvider_ListComponents_NoComponents(t *testing.T) {
	p := &ComponentProvider{}

	names, err := p.ListComponents(context.Background(), "dev", map[string]any{})
	require.NoError(t, err)
	assert.Empty(t, names)
}

func TestComponentProvider_ValidateComponent(t *testing.T) {
	p := &ComponentProvider{}
	require.Error(t, p.ValidateComponent(map[string]any{}))
	require.NoError(t, p.ValidateComponent(map[string]any{"template": "t.yaml", "stack_name": "vpc"}))
}

func TestComponentProvider_GetAvailableCommands(t *testing.T) {
	p := &ComponentProvider{}
	commands := p.GetAvailableCommands()
	assert.Contains(t, commands, "apply")
	assert.Contains(t, commands, "delete")
	assert.Contains(t, commands, "output")
	assert.Contains(t, commands, "validate")
}

func TestComponentProvider_GenerateArtifacts_NoOp(t *testing.T) {
	p := &ComponentProvider{}
	require.NoError(t, p.GenerateArtifacts(nil))
}

func TestComponentProvider_Execute_UnsupportedSubcommand(t *testing.T) {
	p := &ComponentProvider{}
	err := p.Execute(&component.ExecutionContext{SubCommand: "not-a-real-subcommand"})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidSpecificAwsCloudFormationComponent)
}

func TestComponentProvider_IsRegistered(t *testing.T) {
	provider, ok := component.GetProvider(cfg.CloudFormationComponentType)
	require.True(t, ok, "aws/cloudformation provider should self-register via init()")
	assert.Equal(t, "aws/cloudformation", provider.GetType())
}
