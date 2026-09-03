package cloudformation

import (
	"context"
	"sort"
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

// GetAvailableCommands must stay in sync with subCommandOperations — every
// dispatchable subcommand (including aliases like "destroy"/"outputs") must be
// reported, so callers like pkg/composition's verb-allowlist check don't
// reject a subcommand Execute actually supports.
func TestComponentProvider_GetAvailableCommands(t *testing.T) {
	p := &ComponentProvider{}
	commands := p.GetAvailableCommands()

	want := make([]string, 0, len(subCommandOperations))
	for name := range subCommandOperations {
		want = append(want, name)
	}
	sort.Strings(want)
	assert.Equal(t, want, commands)
	assert.Contains(t, commands, "destroy")
	assert.Contains(t, commands, "outputs")
	assert.Contains(t, commands, "fmt")
	// Phase 3: stacksets and nested-stack observability.
	assert.Contains(t, commands, "stackset-create")
	assert.Contains(t, commands, "stackset-update")
	assert.Contains(t, commands, "stackset-delete")
	assert.Contains(t, commands, "stackset-instances")
	assert.Contains(t, commands, "tree")
	assert.Contains(t, commands, "logs")
	assert.Contains(t, commands, "watch")
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

// Execute must map every supported subcommand (including its aliases) to the
// correct Operation before dispatching through executeOperation.
func TestComponentProvider_Execute_MapsSubcommandsToOperations(t *testing.T) {
	tests := []struct {
		subCommand string
		want       Operation
	}{
		{"render", OperationRender},
		{"diff", OperationDiff},
		{"plan", OperationDiff},
		{"apply", OperationApply},
		{"deploy", OperationApply},
		{"delete", OperationDelete},
		{"destroy", OperationDelete},
		{"validate", OperationValidate},
		{"output", OperationOutput},
		{"outputs", OperationOutput},
		{"changeset-create", OperationChangesetCreate},
		{"changeset-execute", OperationChangesetExecute},
		{"changeset-list", OperationChangesetList},
		{"changeset-delete", OperationChangesetDelete},
		{"drift-detect", OperationDriftDetect},
		{"drift-describe", OperationDriftDescribe},
		{"get-template", OperationGetTemplate},
		{"get-policy", OperationGetPolicy},
		{"fmt", OperationFmt},
		{"stackset-create", OperationStackSetCreate},
		{"stackset-update", OperationStackSetUpdate},
		{"stackset-delete", OperationStackSetDelete},
		{"stackset-instances", OperationStackSetInstances},
		{"tree", OperationTree},
		{"logs", OperationLogs},
		{"watch", OperationWatch},
	}

	original := executeOperation
	t.Cleanup(func() { executeOperation = original })

	for _, tt := range tests {
		t.Run(tt.subCommand, func(t *testing.T) {
			var got Operation
			executeOperation = func(_ *component.ExecutionContext, op Operation) error {
				got = op
				return nil
			}

			p := &ComponentProvider{}
			err := p.Execute(&component.ExecutionContext{SubCommand: tt.subCommand})
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestComponentProvider_IsRegistered(t *testing.T) {
	provider, ok := component.GetProvider(cfg.CloudFormationComponentType)
	require.True(t, ok, "aws/cloudformation provider should self-register via init()")
	assert.Equal(t, "aws/cloudformation", provider.GetType())
}
