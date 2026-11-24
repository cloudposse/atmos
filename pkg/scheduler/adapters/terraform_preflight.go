package adapters

import (
	"context"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component/configuration"
	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/schema"
)

// preflightTerraformGraph rejects selected components that their configuration provider could not
// evaluate, or whose stack configuration overrides a provider-owned backend, before any component runs. Without it a bulk run would apply the healthy components
// that sort first and only then fail on the broken one. Every broken component is reported.
func preflightTerraformGraph(ctx context.Context, atmosConfig *schema.AtmosConfiguration, graph *dependency.Graph, excluded map[string]string) error {
	checker, ok := atmosConfig.ComponentConfigurations.(schema.ComponentConfigurationFailureRuntime)
	if !ok {
		return nil
	}
	var failures []error
	for _, id := range sortedGraphNodeIDs(graph) {
		if _, skip := excluded[id]; skip {
			continue
		}
		node := graph.Nodes[id]
		if node == nil {
			continue
		}
		settings, _ := node.Metadata["settings"].(map[string]any)
		if setting := schema.RejectedConfigurationOverride(settings); setting != "" {
			failures = append(failures, configuration.BackendOverrideError(node.Component, node.Stack, setting))
			continue
		}
		if err := checker.ComponentFailure(ctx, node.Stack, node.Component); err != nil {
			failures = append(failures, err)
		}
	}
	if len(failures) == 0 {
		return nil
	}
	return errUtils.Build(errUtils.ErrPluginComponentsFailed).WithCause(errUtils.JoinPreservingHints(failures...)).Err()
}
