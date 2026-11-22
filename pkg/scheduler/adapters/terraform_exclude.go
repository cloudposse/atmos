package adapters

import (
	"fmt"
	"slices"
	"strings"

	"github.com/cloudposse/atmos/pkg/component/execution"
	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/scheduler"
	"github.com/cloudposse/atmos/pkg/schema"
)

// terraformExcludeSetting is where a configuration provider records the exclude block of a unit:
// `settings.terragrunt.exclude` with `actions` and `exclude_dependencies`.
const (
	terraformExcludeProvider = "terragrunt"
	terraformExcludeKey      = "exclude"
)

// terraformExclusions decides which nodes of a bulk run a provider's exclude block removes, with
// the reason for each. A node is excluded when its block lists the command being run, and so are
// its direct dependencies when the block sets exclude_dependencies. A component the user named
// explicitly is never excluded: bulk exclusion only narrows what a selection pulls in.
func terraformExclusions(graph *dependency.Graph, info *schema.ConfigAndStacksInfo) map[string]string {
	defer perf.Track(nil, "scheduler.adapters.terraformExclusions")()

	excluded := map[string]string{}
	if graph == nil || info == nil {
		return excluded
	}
	command := terraformBulkCommand(info)
	for _, id := range sortedGraphNodeIDs(graph) {
		node := graph.Nodes[id]
		block := terraformExcludeBlock(node)
		if block == nil || !execution.ExcludeMatches(stringList(block["actions"]), command) {
			continue
		}
		excluded[id] = fmt.Sprintf("it lists `%s`", command)
		if withDependencies, _ := block["exclude_dependencies"].(bool); withDependencies {
			excludeDependencies(graph, node, excluded)
		}
	}
	for id := range excluded {
		if node := graph.Nodes[id]; node != nil && slices.Contains(info.Components, node.Component) {
			delete(excluded, id)
		}
	}
	return excluded
}

// terraformBulkCommand is the command name exclude blocks list: deploy is apply, and apply with
// -destroy is destroy.
func terraformBulkCommand(info *schema.ConfigAndStacksInfo) string {
	command := info.SubCommand
	if command == terraformSubCommandDeploy {
		command = terraformSubCommandApply
	}
	return execution.CommandName(command, info.AdditionalArgsAndFlags)
}

// excludeDependencies adds the direct dependencies of a node whose block sets exclude_dependencies.
func excludeDependencies(graph *dependency.Graph, node *dependency.Node, excluded map[string]string) {
	for _, dependencyID := range sortedCopy(node.Dependencies) {
		if graph.Nodes[dependencyID] == nil {
			continue
		}
		if _, already := excluded[dependencyID]; !already {
			excluded[dependencyID] = fmt.Sprintf("a dependency of `%s` in `%s`, whose block sets exclude_dependencies", node.Component, node.Stack)
		}
	}
}

// terraformExcludeBlock returns the provider's exclude section of a node, or nil.
func terraformExcludeBlock(node *dependency.Node) map[string]any {
	if node == nil {
		return nil
	}
	settings, _ := node.Metadata["settings"].(map[string]any)
	provider, _ := settings[terraformExcludeProvider].(map[string]any)
	block, _ := provider[terraformExcludeKey].(map[string]any)
	return block
}

func stringList(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		list := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				list = append(list, text)
			}
		}
		return list
	}
	return nil
}

// skippedResultNames names the nodes the scheduler skipped, sorted, as "`component` in `stack`".
func skippedResultNames(result *scheduler.AggregateResult, graph *dependency.Graph) []string {
	if result == nil {
		return nil
	}
	var names []string
	for i := range result.Results {
		if result.Results[i].Status != scheduler.StatusSkipped {
			continue
		}
		id := result.Results[i].NodeID
		if node := graph.Nodes[id]; node != nil {
			id = fmt.Sprintf("`%s` in `%s`", node.Component, node.Stack)
		}
		names = append(names, id)
	}
	slices.Sort(names)
	return names
}

// describeExclusion is the line printed for a node a bulk run leaves out.
func describeExclusion(node *dependency.Node, reason string) string {
	return fmt.Sprintf("Skipped `%s` in `%s`: excluded by the Terragrunt `exclude` block (%s)", node.Component, node.Stack, strings.TrimSpace(reason))
}
