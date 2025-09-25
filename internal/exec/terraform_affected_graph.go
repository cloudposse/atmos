package exec

import (
	"fmt"

	log "github.com/charmbracelet/log"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ExecuteTerraformAffectedWithGraph executes terraform commands for affected components using the dependency graph.
func ExecuteTerraformAffectedWithGraph(args *DescribeAffectedCmdArgs, info *schema.ConfigAndStacksInfo) error {
	// Get the list of affected components.
	affectedList, err := getAffectedComponents(args)
	if err != nil {
		return err
	}

	if len(affectedList) == 0 {
		log.Info("No affected components found")
		return nil
	}

	// Log affected components for debugging.
	if err := logAffectedComponents(affectedList); err != nil {
		return err
	}

	// Build and filter the dependency graph.
	filteredGraph, err := buildFilteredDependencyGraph(args, info, affectedList)
	if err != nil {
		return err
	}

	// Execute components in dependency order.
	return executeAffectedInOrder(filteredGraph, affectedList, args, info)
}

// getAffectedComponents retrieves the list of affected components based on the provided arguments.
func getAffectedComponents(args *DescribeAffectedCmdArgs) ([]schema.Affected, error) {
	switch {
	case args.RepoPath != "":
		return getAffectedWithRepoPath(args)
	case args.CloneTargetRef:
		return getAffectedWithClone(args)
	default:
		return getAffectedWithCheckout(args)
	}
}

// getAffectedWithRepoPath gets affected components using a target repository path.
func getAffectedWithRepoPath(args *DescribeAffectedCmdArgs) ([]schema.Affected, error) {
	affectedList, _, _, _, err := ExecuteDescribeAffectedWithTargetRepoPath(
		args.CLIConfig,
		args.RepoPath,
		args.IncludeSpaceliftAdminStacks,
		args.IncludeSettings,
		args.Stack,
		args.ProcessTemplates,
		args.ProcessYamlFunctions,
		args.Skip,
		args.ExcludeLocked,
	)
	return affectedList, err
}

// getAffectedWithClone gets affected components by cloning the target reference.
func getAffectedWithClone(args *DescribeAffectedCmdArgs) ([]schema.Affected, error) {
	affectedList, _, _, _, err := ExecuteDescribeAffectedWithTargetRefClone(
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
	)
	return affectedList, err
}

// getAffectedWithCheckout gets affected components by checking out the target reference.
func getAffectedWithCheckout(args *DescribeAffectedCmdArgs) ([]schema.Affected, error) {
	affectedList, _, _, _, err := ExecuteDescribeAffectedWithTargetRefCheckout(
		args.CLIConfig,
		args.Ref,
		args.SHA,
		args.IncludeSpaceliftAdminStacks,
		args.IncludeSettings,
		args.Stack,
		args.ProcessTemplates,
		args.ProcessYamlFunctions,
		args.Skip,
		args.ExcludeLocked,
	)
	return affectedList, err
}

// logAffectedComponents logs the affected components for debugging.
func logAffectedComponents(affectedList []schema.Affected) error {
	affectedYaml, err := u.ConvertToYAML(affectedList)
	if err != nil {
		return err
	}
	log.Debug("Affected", "components", affectedYaml)
	return nil
}

// buildFilteredDependencyGraph builds and filters the dependency graph for affected components.
func buildFilteredDependencyGraph(
	args *DescribeAffectedCmdArgs,
	info *schema.ConfigAndStacksInfo,
	affectedList []schema.Affected,
) (*dependency.Graph, error) {
	// Get all stacks.
	stacks, err := ExecuteDescribeStacks(
		args.CLIConfig,
		"",  // all stacks
		nil, // all components
		[]string{cfg.TerraformComponentType},
		nil,
		false,
		args.ProcessTemplates,
		args.ProcessYamlFunctions,
		false,
		args.Skip,
	)
	if err != nil {
		return nil, fmt.Errorf("error describing stacks: %w", err)
	}

	// Build the complete dependency graph.
	fullGraph, err := buildTerraformDependencyGraph(
		args.CLIConfig,
		stacks,
		info,
	)
	if err != nil {
		return nil, fmt.Errorf("error building dependency graph: %w", err)
	}

	// Create list of affected node IDs.
	affectedNodeIDs := extractAffectedNodeIDs(affectedList)

	// Filter the graph.
	return fullGraph.Filter(dependency.Filter{
		NodeIDs:             affectedNodeIDs,
		IncludeDependencies: true,                   // Include what affected components depend on.
		IncludeDependents:   args.IncludeDependents, // Include what depends on affected components.
	}), nil
}

// extractAffectedNodeIDs extracts node IDs from the affected components list.
func extractAffectedNodeIDs(affectedList []schema.Affected) []string {
	nodeIDs := make([]string, len(affectedList))
	for i := range affectedList {
		nodeIDs[i] = fmt.Sprintf("%s-%s", affectedList[i].Component, affectedList[i].Stack)
	}
	return nodeIDs
}

// executeAffectedInOrder executes the affected components in topological order.
func executeAffectedInOrder(
	graph *dependency.Graph,
	affectedList []schema.Affected,
	args *DescribeAffectedCmdArgs,
	info *schema.ConfigAndStacksInfo,
) error {
	// Get execution order.
	executionOrder, err := graph.TopologicalSort()
	if err != nil {
		return fmt.Errorf("error determining execution order: %w", err)
	}

	log.Info("Processing affected components in dependency order", "count", len(executionOrder))

	// Execute components in order.
	for i := range executionOrder {
		node := &executionOrder[i]

		// Log the component being processed.
		logComponentExecution(node, i+1, len(executionOrder),
			isDirectlyAffected(node, affectedList), args.IncludeDependents)

		// Execute terraform for the node.
		if err := executeTerraformForNode(node, info); err != nil {
			return fmt.Errorf("error executing terraform for component %s in stack %s: %w",
				node.Component, node.Stack, err)
		}
	}

	log.Info("Successfully processed affected components", "count", len(executionOrder))
	return nil
}

// isDirectlyAffected checks if a node is directly affected (not just a dependency/dependent).
func isDirectlyAffected(node *dependency.Node, affectedList []schema.Affected) bool {
	for i := range affectedList {
		if node.Component == affectedList[i].Component && node.Stack == affectedList[i].Stack {
			return true
		}
	}
	return false
}

// logComponentExecution logs information about the component being executed.
func logComponentExecution(node *dependency.Node, index, total int, directlyAffected, includeDependents bool) {
	logArgs := []any{
		"index", index,
		"total", total,
		"component", node.Component,
		"stack", node.Stack,
	}

	switch {
	case directlyAffected:
		log.Info("Processing affected component", logArgs...)
	case includeDependents:
		log.Info("Processing dependent component", logArgs...)
	default:
		log.Info("Processing dependency", logArgs...)
	}
}
