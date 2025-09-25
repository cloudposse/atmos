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
	var affectedList []schema.Affected
	var err error

	// Get the list of affected components (existing logic)
	switch {
	case args.RepoPath != "":
		affectedList, _, _, _, err = ExecuteDescribeAffectedWithTargetRepoPath(
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
	case args.CloneTargetRef:
		affectedList, _, _, _, err = ExecuteDescribeAffectedWithTargetRefClone(
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
	default:
		affectedList, _, _, _, err = ExecuteDescribeAffectedWithTargetRefCheckout(
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
	}
	if err != nil {
		return err
	}

	if len(affectedList) == 0 {
		log.Info("No affected components found")
		return nil
	}

	affectedYaml, err := u.ConvertToYAML(affectedList)
	if err != nil {
		return err
	}
	log.Debug("Affected", "components", affectedYaml)

	// Get all stacks to build the complete dependency graph
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
		return fmt.Errorf("error describing stacks: %w", err)
	}

	// Build the complete dependency graph
	fullGraph, err := buildTerraformDependencyGraph(
		args.CLIConfig,
		stacks,
		info,
	)
	if err != nil {
		return fmt.Errorf("error building dependency graph: %w", err)
	}

	// Create list of affected node IDs
	affectedNodeIDs := []string{}
	for _, affected := range affectedList {
		nodeID := fmt.Sprintf("%s-%s", affected.Component, affected.Stack)
		affectedNodeIDs = append(affectedNodeIDs, nodeID)
	}

	// Filter the graph to include affected components and their dependencies/dependents
	filteredGraph := fullGraph.Filter(dependency.Filter{
		NodeIDs:             affectedNodeIDs,
		IncludeDependencies: true,                   // Include what affected components depend on
		IncludeDependents:   args.IncludeDependents, // Include what depends on affected components
	})

	// Get execution order
	executionOrder, err := filteredGraph.TopologicalSort()
	if err != nil {
		return fmt.Errorf("error determining execution order: %w", err)
	}

	log.Info("Processing affected components in dependency order", "count", len(executionOrder))

	// Execute components in order
	for i, node := range executionOrder {
		// Check if this node is directly affected or just a dependency/dependent
		isDirectlyAffected := false
		for _, affected := range affectedList {
			if node.Component == affected.Component && node.Stack == affected.Stack {
				isDirectlyAffected = true
				break
			}
		}

		if isDirectlyAffected {
			log.Info("Processing affected component", "index", i+1, "total", len(executionOrder),
				"component", node.Component, "stack", node.Stack)
		} else if args.IncludeDependents {
			log.Info("Processing dependent component", "index", i+1, "total", len(executionOrder),
				"component", node.Component, "stack", node.Stack)
		} else {
			log.Info("Processing dependency", "index", i+1, "total", len(executionOrder),
				"component", node.Component, "stack", node.Stack)
		}

		if err := executeTerraformForNode(node, info); err != nil {
			return fmt.Errorf("error executing terraform for component %s in stack %s: %w",
				node.Component, node.Stack, err)
		}
	}

	log.Info("Successfully processed affected components", "count", len(executionOrder))
	return nil
}
