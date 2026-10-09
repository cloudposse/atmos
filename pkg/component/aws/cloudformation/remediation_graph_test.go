package cloudformation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

type remediationGraphProvider struct {
	ComponentProvider
	calls []string
}

func (p *remediationGraphProvider) Execute(ctx *component.ExecutionContext) error {
	p.calls = append(p.calls, ctx.Component)
	return nil
}

// Keep the real graph construction, selection and execution; replace only stack
// discovery and the final provider dispatch so this contract needs no AWS account.
func TestBulkDependencyOrderWithSelectors(t *testing.T) {
	for _, tt := range []struct {
		name  string
		info  schema.ConfigAndStacksInfo
		flags map[string]any
	}{
		{name: "all", info: schema.ConfigAndStacksInfo{All: true}},
		{name: "components", info: schema.ConfigAndStacksInfo{Components: []string{"producer", "consumer"}}},
		{name: "affected", info: schema.ConfigAndStacksInfo{Affected: true}},
		{name: "affected with dependents", info: schema.ConfigAndStacksInfo{Affected: true}, flags: map[string]any{"include-dependents": true}},
	} {
		for _, operation := range []Operation{OperationApply, OperationDelete} {
			t.Run(tt.name+"/"+string(operation), func(t *testing.T) {
				originalDescribe, originalGraph, originalAffected := executeDescribeStacks, executeGraph, affectedCloudFormationComponentsFunc
				t.Cleanup(func() {
					executeDescribeStacks = originalDescribe
					executeGraph = originalGraph
					affectedCloudFormationComponentsFunc = originalAffected
				})
				executeDescribeStacks = func(_ *schema.AtmosConfiguration, _ string, _, _, _ []string, _, _, _, _ bool, _ []string, _ auth.AuthManager) (map[string]any, error) {
					return map[string]any{"dev": map[string]any{"components": map[string]any{cfg.CloudFormationComponentType: map[string]any{
						"producer": map[string]any{},
						"consumer": map[string]any{"settings": map[string]any{"depends_on": []any{"producer"}}},
					}}}}, nil
				}
				affectedCloudFormationComponentsFunc = func(_ *component.ExecutionContext, _ *schema.AtmosConfiguration, _ *schema.ConfigAndStacksInfo) ([]schema.Affected, error) {
					selected := "consumer"
					if tt.flags["include-dependents"] == true {
						selected = "producer"
					}
					return []schema.Affected{{Component: selected, Stack: "dev", ComponentType: cfg.CloudFormationComponentType}}, nil
				}
				provider := &remediationGraphProvider{}
				executeGraph = func(ctx context.Context, opts *component.GraphExecutionOptions) error {
					opts.Provider = provider
					return component.ExecuteGraph(ctx, opts)
				}
				info := tt.info
				err := executeBulk(&component.ExecutionContext{Flags: tt.flags}, &schema.AtmosConfiguration{}, &info, operation)
				require.NoError(t, err)
				expected := []string{"producer", "consumer"}
				if operation == OperationDelete {
					expected = []string{"consumer", "producer"}
				}
				assert.Equal(t, expected, provider.calls)
			})
		}
	}
}
