package exec

import (
	"context"
	"errors"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/dependency"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	scheduleradapters "github.com/cloudposse/atmos/pkg/scheduler/adapters"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/secrets"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/spinner"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const errWrapFmt = "%w: %w"

// ExecuteTerraformAll executes terraform commands for all components in dependency order.
func ExecuteTerraformAll(info *schema.ConfigAndStacksInfo) error {
	defer perf.Track(nil, "exec.ExecuteTerraformAll")()
	return ExecuteTerraformAllWithContext(context.Background(), info)
}

// ExecuteTerraformAllWithContext executes all selected Terraform components through
// the graph-backed scheduler using the provided cancellation context.
func ExecuteTerraformAllWithContext(ctx context.Context, info *schema.ConfigAndStacksInfo) error {
	defer perf.Track(nil, "exec.ExecuteTerraformAllWithContext")()
	// Validate inputs for --all flag usage.
	// When no stack is given, --all processes every stack — matching the documented
	// behavior of `atmos terraform apply --all` (see website/docs/cli/commands/terraform).
	if info.ComponentFromArg != "" {
		return errUtils.ErrComponentWithAllFlagConflict
	}

	var atmosConfig schema.AtmosConfiguration
	var stacks map[string]any
	preflight := spinner.New("Loading stack configuration and resolving templates")
	preflight.Start()

	atmosConfig, err := cfg.InitCliConfig(*info, true)
	if err != nil {
		preflight.Error("Failed to load Terraform stack configuration")
		return fmt.Errorf(errWrapFmt, errUtils.ErrInitializeCLIConfig, err)
	}

	log.Debug("Executing terraform command for all components in dependency order", "command", info.SubCommand)

	// Create auth manager so YAML functions (e.g. !terraform.state) can use authenticated
	// credentials when ExecuteDescribeStacks processes stack configurations under --all.
	// Mirrors the behavior added for --query/--components in ExecuteTerraformQuery (#2081).
	preflight.Update("Resolving Terraform identity")
	authManager, err := createQueryAuthManager(info, &atmosConfig)
	if err != nil {
		preflight.Error("Failed to resolve Terraform identity")
		return err
	}
	if authManager != nil {
		injectTerraformStoreAuthResolver(&atmosConfig, info, authManager)
	}

	preflight.Update("Resolving Terraform component instances, secrets, and state references")
	stacks, err = ExecuteDescribeStacksWithMocks(
		&atmosConfig,
		info.Stack,
		nil, // all components
		[]string{cfg.TerraformComponentType},
		nil,
		false,
		info.ProcessTemplates,
		info.ProcessFunctions,
		false,
		info.Skip,
		authManager,
		info.UseMocks,
	)
	if err != nil {
		preflight.Error("Failed to resolve Terraform component instances")
		return terraformPreflightDescribeError(err)
	}
	preflight.Success("Resolved Terraform stacks and dependencies")

	if info.SubCommand == "destroy" {
		ui.Info("Processing components in reverse dependency order for destroy")
	} else {
		ui.Info("Processing components in dependency order")
	}

	return scheduleradapters.ExecuteTerraform(ctx, scheduleradapters.TerraformOptions{
		AtmosConfig: &atmosConfig,
		Info:        info,
		Stacks:      stacks,
		Executor:    executeTerraformQueryComponent,
	})
}

// terraformPreflightDescribeError preserves structured errors from stack resolution
// and explains why graph Terraform commands stop before the scheduler starts.
func terraformPreflightDescribeError(cause error) error {
	builder := errUtils.Build(errUtils.ErrExecuteDescribeStacks).
		WithTitle("Terraform preflight failed").
		WithCause(cause)

	if errors.Is(cause, secrets.ErrSecretMissing) {
		return builder.
			WithExplanation("A required `!secret` could not be resolved before Terraform started.").
			WithHint("Initialize the reported secret, then rerun the Terraform command.").
			Err()
	}

	return builder.
		WithExplanation("Terraform component instances could not be resolved before Terraform started.").
		Err()
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
		return nil, fmt.Errorf("%w: adding nodes: %w", errUtils.ErrBuildDepGraph, err)
	}

	// Second pass: build dependencies using settings.depends_on.
	if err := buildGraphDependencies(stacks, builder, nodeMap); err != nil {
		return nil, fmt.Errorf("%w: building dependencies: %w", errUtils.ErrBuildDepGraph, err)
	}

	// Build the final graph.
	graph, err := builder.Build()
	if err != nil {
		return nil, fmt.Errorf("%w: finalizing graph: %w", errUtils.ErrBuildDepGraph, err)
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
//
// Deprecated: production Terraform bulk filtering uses the scheduler adapter.
// Keep this helper only while legacy graph-filter tests still cover it.
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
	// IncludeDependencies is false to preserve the historical scope of `--all -s <stack>`:
	// only components in the requested stack are processed. Cross-stack prerequisites are
	// retained as graph edges within the requested stack (where both endpoints are present)
	// but components outside the requested stack are not pulled in. A future flag may
	// opt users in to cross-stack dependency execution.
	return graph.Filter(dependency.Filter{
		NodeIDs:             nodeIDs,
		IncludeDependencies: false,
		IncludeDependents:   false,
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
