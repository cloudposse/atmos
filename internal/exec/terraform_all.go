package exec

import (
	"fmt"

	log "github.com/charmbracelet/log"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ExecuteTerraformAll executes terraform commands for all components in dependency order.
func ExecuteTerraformAll(info *schema.ConfigAndStacksInfo) error {
	// Validate inputs for --all flag usage.
	if info.Stack == "" {
		return errUtils.ErrStackRequiredWithAllFlag
	}
	if info.ComponentFromArg != "" {
		return errUtils.ErrComponentWithAllFlagConflict
	}

	atmosConfig, err := cfg.InitCliConfig(*info, true)
	if err != nil {
		return fmt.Errorf("%w: %v", errUtils.ErrInitializeCLIConfig, err)
	}

	log.Debug("Executing terraform command for all components in dependency order", "command", info.SubCommand)

	// Get all stacks with terraform components.
	stacks, err := ExecuteDescribeStacks(
		&atmosConfig,
		"",  // all stacks
		nil, // all components
		[]string{cfg.TerraformComponentType},
		nil,
		false,
		info.ProcessTemplates,
		info.ProcessFunctions,
		false,
		info.Skip,
		nil, // authManager
	)
	if err != nil {
		return fmt.Errorf("%w: %v", errUtils.ErrExecuteDescribeStacks, err)
	}

	// Build dependency graph.
	graph, err := buildTerraformDependencyGraph(
		&atmosConfig,
		stacks,
		info,
	)
	if err != nil {
		return fmt.Errorf("%w: %v", errUtils.ErrBuildDepGraph, err)
	}

	// Apply filters if specified.
	if info.Query != "" || len(info.Components) > 0 || info.Stack != "" {
		graph = applyFiltersToGraph(graph, stacks, info)
	}

	// Execute components in dependency order.
	return executeInDependencyOrder(graph, info)
}

// executeInDependencyOrder executes terraform commands in dependency order.
func executeInDependencyOrder(graph *dependency.Graph, info *schema.ConfigAndStacksInfo) error {
	// Get execution order.
	executionOrder, err := graph.TopologicalSort()
	if err != nil {
		return fmt.Errorf("%w: %v", errUtils.ErrTopologicalOrder, err)
	}

	// For destroy command, reverse the execution order to destroy dependents before dependencies.
	if info.SubCommand == "destroy" {
		for i, j := 0, len(executionOrder)-1; i < j; i, j = i+1, j-1 {
			executionOrder[i], executionOrder[j] = executionOrder[j], executionOrder[i]
		}
		log.Info("Processing components in reverse dependency order for destroy", "count", len(executionOrder))
	} else {
		log.Info("Processing components in dependency order", "count", len(executionOrder))
	}

	// Execute components in order.
	for i := range executionOrder {
		node := &executionOrder[i]
		log.Info("Processing component", "index", i+1, "total", len(executionOrder), "component", node.Component, "stack", node.Stack)

		if err := executeTerraformForNode(node, info); err != nil {
			return fmt.Errorf("%w: component=%s stack=%s: %v", errUtils.ErrTerraformExecFailed, node.Component, node.Stack, err)
		}
	}

	log.Info("Successfully processed all components", "count", len(executionOrder))
	return nil
}

// buildTerraformDependencyGraph builds the complete dependency graph from stacks.
func buildTerraformDependencyGraph(
	_ *schema.AtmosConfiguration,
	stacks map[string]any,
	_ *schema.ConfigAndStacksInfo,
) (*dependency.Graph, error) {
	builder := dependency.NewBuilder()
	nodeMap := make(map[string]string) // Maps component-stack to node ID

	// First pass: add all nodes.
	if err := addNodesToGraph(stacks, builder, nodeMap); err != nil {
		return nil, fmt.Errorf("%w: adding nodes: %v", errUtils.ErrBuildDepGraph, err)
	}

	// Second pass: build dependencies using settings.depends_on.
	if err := buildGraphDependencies(stacks, builder, nodeMap); err != nil {
		return nil, fmt.Errorf("%w: building dependencies: %v", errUtils.ErrBuildDepGraph, err)
	}

	// Build the final graph.
	graph, err := builder.Build()
	if err != nil {
		return nil, fmt.Errorf("%w: finalizing graph: %v", errUtils.ErrBuildDepGraph, err)
	}

	log.Debug("Dependency graph built", "nodes", graph.Size(), "roots", len(graph.Roots))
	return graph, nil
}

// addNodesToGraph adds all component nodes to the dependency graph.
func addNodesToGraph(
	stacks map[string]any,
	builder *dependency.GraphBuilder,
	nodeMap map[string]string,
) error {
	return walkTerraformComponents(stacks, func(stackName, componentName string, componentSection map[string]any) error {
		if shouldSkipComponentForGraph(componentSection, componentName) {
			return nil
		}

		nodeID := fmt.Sprintf("%s-%s", componentName, stackName)
		node := &dependency.Node{
			ID:        nodeID,
			Component: componentName,
			Stack:     stackName,
			Type:      cfg.TerraformComponentType,
			Metadata:  componentSection,
		}

		nodeMap[nodeID] = nodeID
		return builder.AddNode(node)
	})
}

// buildGraphDependencies builds dependencies between nodes in the graph.
func buildGraphDependencies(
	stacks map[string]any,
	builder *dependency.GraphBuilder,
	nodeMap map[string]string,
) error {
	parser := NewDependencyParser(builder, nodeMap)

	return walkTerraformComponents(stacks, func(stackName, componentName string, componentSection map[string]any) error {
		if shouldSkipComponentForGraph(componentSection, componentName) {
			return nil
		}

		return parser.ParseComponentDependencies(stackName, componentName, componentSection)
	})
}

// shouldSkipComponentForGraph determines if a component should be skipped when building the graph.
func shouldSkipComponentForGraph(componentSection map[string]any, componentName string) bool {
	metadataSection, ok := componentSection[cfg.MetadataSectionName].(map[string]any)
	if !ok {
		return false
	}

	// Skip abstract components.
	if metadataType, ok := metadataSection["type"].(string); ok && metadataType == "abstract" {
		return true
	}

	// Skip disabled components.
	return !isComponentEnabled(metadataSection, componentName)
}

// applyFiltersToGraph applies query and component filters to the graph.
func applyFiltersToGraph(graph *dependency.Graph, _ map[string]any, info *schema.ConfigAndStacksInfo) *dependency.Graph {
	// Determine base set: components/stack if provided; otherwise all nodes.
	nodeIDs := collectFilteredNodeIDs(graph, info)

	// If no nodes collected from filters, use all nodes as the base set
	if len(nodeIDs) == 0 {
		// Check if we have any filters specified
		if len(info.Components) == 0 && info.Stack == "" && info.Query == "" {
			// No filters at all - return the original graph
			return graph
		}
		// We have filters but no nodes collected (query-only case or empty filter result)
		nodeIDs = getAllNodeIDs(graph)
	}

	// Apply query filter if specified.
	if info.Query != "" {
		nodeIDs = filterNodesByQuery(graph, nodeIDs, info.Query)
	}

	// Filter the graph.
	return graph.Filter(dependency.Filter{
		NodeIDs:             nodeIDs,
		IncludeDependencies: true,  // Include prerequisites.
		IncludeDependents:   false, // Exclude reverse deps.
	})
}

// collectFilteredNodeIDs collects node IDs based on component and stack filters.
func collectFilteredNodeIDs(graph *dependency.Graph, info *schema.ConfigAndStacksInfo) []string {
	if len(info.Components) > 0 {
		return filterNodesByComponents(graph, info.Components, info.Stack)
	}

	if info.Stack != "" {
		return filterNodesByStack(graph, info.Stack)
	}

	return []string{}
}

// filterNodesByComponents filters nodes by component names and optionally by stack.
func filterNodesByComponents(graph *dependency.Graph, components []string, stack string) []string {
	var nodeIDs []string

	for _, node := range graph.Nodes {
		if isNodeInComponents(node, components) && isNodeInStack(node, stack) {
			nodeIDs = append(nodeIDs, node.ID)
		}
	}

	return nodeIDs
}

// filterNodesByStack filters nodes by stack name.
func filterNodesByStack(graph *dependency.Graph, stack string) []string {
	var nodeIDs []string

	for _, node := range graph.Nodes {
		if node.Stack == stack {
			nodeIDs = append(nodeIDs, node.ID)
		}
	}

	return nodeIDs
}

// filterNodesByQuery filters nodes using a YQ query expression.
func filterNodesByQuery(graph *dependency.Graph, nodeIDs []string, query string) []string {
	var filteredNodeIDs []string

	for _, nodeID := range nodeIDs {
		node := graph.Nodes[nodeID]
		if evaluateNodeQuery(node, query) {
			filteredNodeIDs = append(filteredNodeIDs, nodeID)
		}
	}

	return filteredNodeIDs
}

// evaluateNodeQuery evaluates a YQ query expression against a node's metadata.
func evaluateNodeQuery(node *dependency.Node, query string) bool {
	if node.Metadata == nil {
		return false
	}

	queryResult, err := u.EvaluateYqExpression(&schema.AtmosConfiguration{}, node.Metadata, query)
	if err != nil {
		return false
	}

	queryPassed, ok := queryResult.(bool)
	return ok && queryPassed
}

// isNodeInComponents checks if a node's component is in the list of components.
func isNodeInComponents(node *dependency.Node, components []string) bool {
	for _, comp := range components {
		if node.Component == comp {
			return true
		}
	}
	return false
}

// isNodeInStack checks if a node is in the specified stack (or if no stack is specified).
func isNodeInStack(node *dependency.Node, stack string) bool {
	return stack == "" || node.Stack == stack
}

// getAllNodeIDs returns all node IDs from the graph.
func getAllNodeIDs(graph *dependency.Graph) []string {
	var nodeIDs []string
	for id := range graph.Nodes {
		nodeIDs = append(nodeIDs, id)
	}
	return nodeIDs
}
