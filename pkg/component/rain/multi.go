package rain

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store/authbridge"
	"github.com/cloudposse/atmos/pkg/ui"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const wrapErrorFormat = "%w: %w"

// ExecuteAll executes a Rain command for all matching Rain components in dependency order.
func ExecuteAll(info *schema.ConfigAndStacksInfo) error {
	defer perf.Track(nil, "rain.ExecuteAll")()

	if info.ComponentFromArg != "" {
		return errUtils.ErrComponentWithAllFlagConflict
	}

	atmosConfig, err := cfg.InitCliConfig(*info, true)
	if err != nil {
		return fmt.Errorf(wrapErrorFormat, errUtils.ErrInitializeCLIConfig, err)
	}

	authManager, err := setupRainAuth(&atmosConfig, info)
	if err != nil {
		return err
	}
	if authManager != nil {
		resolver := authbridge.NewResolver(authManager, info)
		atmosConfig.Stores.SetAuthContextResolver(resolver)
	}

	stacks, err := e.ExecuteDescribeStacks(
		&atmosConfig,
		"",
		nil,
		[]string{cfg.RainComponentType},
		nil,
		false,
		info.ProcessTemplates,
		info.ProcessFunctions,
		false,
		info.Skip,
		authManager,
	)
	if err != nil {
		return fmt.Errorf(wrapErrorFormat, errUtils.ErrExecuteDescribeStacks, err)
	}

	graph, err := buildDependencyGraph(stacks)
	if err != nil {
		return err
	}
	graph = applyGraphFilters(graph, info)

	order, err := graph.TopologicalSort()
	if err != nil {
		return fmt.Errorf(wrapErrorFormat, errUtils.ErrTopologicalOrder, err)
	}

	return executeOrder(reverseOrderForRemove(order, info.SubCommand), info)
}

// ExecuteAffected executes a Rain command for Git-affected Rain components.
func ExecuteAffected(args *e.DescribeAffectedCmdArgs, info *schema.ConfigAndStacksInfo) error {
	defer perf.Track(nil, "rain.ExecuteAffected")()

	if args == nil || info == nil {
		return errUtils.ErrNilParam
	}

	if err := prepareAffectedAuth(args, info); err != nil {
		return err
	}

	affected, err := getAffected(args)
	if err != nil {
		return err
	}

	filtered := filterRainAffected(affected)
	if len(filtered) == 0 {
		ui.Success("No components affected")
		return nil
	}

	graph, err := buildAffectedDependencyGraph(args, filtered)
	if err != nil {
		return err
	}
	order, err := graph.TopologicalSort()
	if err != nil {
		return fmt.Errorf(wrapErrorFormat, errUtils.ErrTopologicalOrder, err)
	}

	return executeOrder(reverseOrderForRemove(order, info.SubCommand), info)
}

func prepareAffectedAuth(args *e.DescribeAffectedCmdArgs, info *schema.ConfigAndStacksInfo) error {
	authManager, err := setupRainAuth(args.CLIConfig, info)
	if err != nil {
		return err
	}
	args.AuthManager = authManager
	args.AuthDisabled = info.AuthDisabled
	if authManager != nil {
		resolver := authbridge.NewResolver(authManager, info)
		args.CLIConfig.Stores.SetAuthContextResolver(resolver)
	}
	return nil
}

func reverseOrderForRemove(order dependency.ExecutionOrder, command string) dependency.ExecutionOrder {
	if command != "rm" {
		return order
	}
	for i, j := 0, len(order)-1; i < j; i, j = i+1, j-1 {
		order[i], order[j] = order[j], order[i]
	}
	return order
}

func getAffected(args *e.DescribeAffectedCmdArgs) ([]schema.Affected, error) {
	switch {
	case args.RepoPath != "":
		affectedList, _, _, _, err := e.ExecuteDescribeAffectedWithTargetRepoPath(
			args.CLIConfig,
			args.RepoPath,
			args.IncludeSpaceliftAdminStacks,
			args.IncludeSettings,
			args.Stack,
			args.ProcessTemplates,
			args.ProcessYamlFunctions,
			args.Skip,
			args.ExcludeLocked,
			args.AuthManager,
			args.AuthDisabled,
		)
		return affectedList, err
	case args.CloneTargetRef:
		affectedList, _, _, _, err := e.ExecuteDescribeAffectedWithTargetRefClone(
			args.CLIConfig,
			args.Ref,
			args.SHA,
			args.SSHKeyPath,
			args.SSHKeyPassword,
			args.IncludeSpaceliftAdminStacks,
			args.IncludeSettings,
			args.Stack,
			args.ProcessTemplates,
			args.ProcessYamlFunctions,
			args.Skip,
			args.ExcludeLocked,
			args.AuthManager,
			args.AuthDisabled,
		)
		return affectedList, err
	default:
		affectedList, _, _, _, err := e.ExecuteDescribeAffectedWithTargetRefCheckout(
			args.CLIConfig,
			args.Ref,
			args.SHA,
			args.TargetBranch,
			args.IncludeSpaceliftAdminStacks,
			args.IncludeSettings,
			args.Stack,
			args.ProcessTemplates,
			args.ProcessYamlFunctions,
			args.Skip,
			args.ExcludeLocked,
			args.AuthManager,
			args.AuthDisabled,
		)
		return affectedList, err
	}
}

func filterRainAffected(affected []schema.Affected) []schema.Affected {
	filtered := make([]schema.Affected, 0, len(affected))
	for i := range affected {
		item := affected[i]
		if item.ComponentType != cfg.RainComponentType || item.Deleted {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func buildDependencyGraph(stacks map[string]any) (*dependency.Graph, error) {
	builder := dependency.NewBuilder()
	nodeMap := map[string]string{}

	if err := walkRainComponents(stacks, func(stack, componentName string, componentSection map[string]any) error {
		if shouldSkip(componentSection) {
			return nil
		}
		nodeID := fmt.Sprintf("%s-%s", componentName, stack)
		nodeMap[nodeID] = nodeID
		return builder.AddNode(&dependency.Node{
			ID:        nodeID,
			Component: componentName,
			Stack:     stack,
			Type:      cfg.RainComponentType,
			Metadata:  componentSection,
		})
	}); err != nil {
		return nil, err
	}

	parser := e.NewDependencyParser(builder, nodeMap)
	if err := walkRainComponents(stacks, func(stack, componentName string, componentSection map[string]any) error {
		if shouldSkip(componentSection) {
			return nil
		}
		return parser.ParseComponentDependencies(stack, componentName, componentSection)
	}); err != nil {
		return nil, err
	}

	graph, err := builder.Build()
	if err != nil {
		return nil, fmt.Errorf(wrapErrorFormat, errUtils.ErrBuildDepGraph, err)
	}
	return graph, nil
}

func buildAffectedDependencyGraph(args *e.DescribeAffectedCmdArgs, affected []schema.Affected) (*dependency.Graph, error) {
	stacks, err := e.ExecuteDescribeStacksWithAuthDisabled(
		args.CLIConfig,
		args.Stack,
		nil,
		[]string{cfg.RainComponentType},
		nil,
		false,
		args.ProcessTemplates,
		args.ProcessYamlFunctions,
		false,
		args.Skip,
		args.AuthManager,
		args.AuthDisabled,
	)
	if err != nil {
		return nil, fmt.Errorf(wrapErrorFormat, errUtils.ErrExecuteDescribeStacks, err)
	}

	graph, err := buildDependencyGraph(stacks)
	if err != nil {
		return nil, err
	}

	nodeIDs := make([]string, 0, len(affected))
	for i := range affected {
		item := affected[i]
		nodeIDs = append(nodeIDs, fmt.Sprintf("%s-%s", item.Component, item.Stack))
	}

	return graph.Filter(dependency.Filter{
		NodeIDs:             nodeIDs,
		IncludeDependencies: true,
		IncludeDependents:   args.IncludeDependents,
	}), nil
}

func walkRainComponents(stacks map[string]any, fn func(stack, componentName string, componentSection map[string]any) error) error {
	for stackName, stackData := range stacks {
		stackMap, ok := stackData.(map[string]any)
		if !ok {
			continue
		}
		componentsMap, ok := stackMap[cfg.ComponentsSectionName].(map[string]any)
		if !ok {
			continue
		}
		rainMap, ok := componentsMap[cfg.RainComponentType].(map[string]any)
		if !ok {
			continue
		}
		for componentName, raw := range rainMap {
			componentSection, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if err := fn(stackName, componentName, componentSection); err != nil {
				return err
			}
		}
	}
	return nil
}

func shouldSkip(componentSection map[string]any) bool {
	metadata, ok := componentSection[cfg.MetadataSectionName].(map[string]any)
	if !ok {
		return false
	}
	if metadataType, ok := metadata["type"].(string); ok && metadataType == "abstract" {
		return true
	}
	if enabled, ok := metadata["enabled"].(bool); ok && !enabled {
		return true
	}
	return false
}

func applyGraphFilters(graph *dependency.Graph, info *schema.ConfigAndStacksInfo) *dependency.Graph {
	nodeIDs := matchingNodeIDs(graph, info)
	if info.Query != "" {
		nodeIDs = filterNodesByQuery(graph, nodeIDs, info.Query)
	}
	return graph.Filter(dependency.Filter{NodeIDs: nodeIDs})
}

func matchingNodeIDs(graph *dependency.Graph, info *schema.ConfigAndStacksInfo) []string {
	var nodeIDs []string
	for _, node := range graph.Nodes {
		if nodeMatchesFilters(node, info) {
			nodeIDs = append(nodeIDs, node.ID)
		}
	}
	if len(nodeIDs) > 0 || info.Stack != "" || len(info.Components) > 0 {
		return nodeIDs
	}
	if info.Query == "" {
		return nil
	}
	for id := range graph.Nodes {
		nodeIDs = append(nodeIDs, id)
	}
	return nodeIDs
}

func nodeMatchesFilters(node *dependency.Node, info *schema.ConfigAndStacksInfo) bool {
	if info.Stack != "" && node.Stack != info.Stack {
		return false
	}
	return len(info.Components) == 0 || contains(info.Components, node.Component)
}

func filterNodesByQuery(graph *dependency.Graph, nodeIDs []string, query string) []string {
	filtered := make([]string, 0, len(nodeIDs))
	for _, nodeID := range nodeIDs {
		node := graph.Nodes[nodeID]
		if node == nil || node.Metadata == nil {
			continue
		}
		queryResult, err := u.EvaluateYqExpression(&schema.AtmosConfiguration{}, node.Metadata, query)
		if err != nil {
			continue
		}
		if passed, ok := queryResult.(bool); ok && passed {
			filtered = append(filtered, nodeID)
		}
	}
	return filtered
}

func executeOrder(order dependency.ExecutionOrder, info *schema.ConfigAndStacksInfo) error {
	for i := range order {
		node := order[i]
		next := *info
		next.ComponentFromArg = node.Component
		next.Component = node.Component
		next.Stack = node.Stack
		next.All = false
		next.Affected = false

		provider := component.MustGetProvider(cfg.RainComponentType)
		if err := provider.Execute(&component.ExecutionContext{
			ComponentType:       cfg.RainComponentType,
			Component:           node.Component,
			Stack:               node.Stack,
			Command:             cfg.RainComponentType,
			SubCommand:          info.SubCommand,
			ConfigAndStacksInfo: next,
			Args:                info.AdditionalArgsAndFlags,
		}); err != nil {
			return fmt.Errorf("%w: component=%s stack=%s: %w", errUtils.ErrComponentExecutionFailed, node.Component, node.Stack, err)
		}
	}
	return nil
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
