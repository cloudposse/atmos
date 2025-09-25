package exec

import (
	"fmt"

	log "github.com/charmbracelet/log"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ExecuteTerraformAll executes terraform commands for all components in dependency order.
func ExecuteTerraformAll(info *schema.ConfigAndStacksInfo) error {
	atmosConfig, err := cfg.InitCliConfig(*info, true)
	if err != nil {
		return fmt.Errorf("error initializing CLI config: %w", err)
	}

	log.Debug("Executing terraform command for all components in dependency order", "command", info.SubCommand)

	// Get all stacks with terraform components
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
	)
	if err != nil {
		return fmt.Errorf("error describing stacks: %w", err)
	}

	// Build dependency graph
	graph, err := buildTerraformDependencyGraph(
		&atmosConfig,
		stacks,
		info,
	)
	if err != nil {
		return fmt.Errorf("error building dependency graph: %w", err)
	}

	// Apply filters if specified
	if info.Query != "" || len(info.Components) > 0 || info.Stack != "" {
		graph = applyFiltersToGraph(graph, stacks, info)
	}

	// Get execution order
	executionOrder, err := graph.TopologicalSort()
	if err != nil {
		return fmt.Errorf("error determining execution order: %w", err)
	}

	log.Info("Processing components in dependency order", "count", len(executionOrder))

	// Execute components in order
	for i, node := range executionOrder {
		log.Info("Processing component", "index", i+1, "total", len(executionOrder), "component", node.Component, "stack", node.Stack)

		if err := executeTerraformForNode(node, info); err != nil {
			return fmt.Errorf("error executing terraform for component %s in stack %s: %w", node.Component, node.Stack, err)
		}
	}

	log.Info("Successfully processed all components", "count", len(executionOrder))
	return nil
}

// buildTerraformDependencyGraph builds the complete dependency graph from stacks.
func buildTerraformDependencyGraph(
	atmosConfig *schema.AtmosConfiguration,
	stacks map[string]any,
	info *schema.ConfigAndStacksInfo,
) (*dependency.Graph, error) {
	builder := dependency.NewBuilder()
	nodeMap := make(map[string]string) // Maps component-stack to node ID

	// First pass: add all nodes
	err := walkTerraformComponents(stacks, func(stackName, componentName string, componentSection map[string]any) error {
		// Skip abstract components
		if metadataSection, ok := componentSection[cfg.MetadataSectionName].(map[string]any); ok {
			if metadataType, ok := metadataSection["type"].(string); ok && metadataType == "abstract" {
				return nil
			}
		}

		// Skip disabled components
		if metadataSection, ok := componentSection[cfg.MetadataSectionName].(map[string]any); ok {
			if !isComponentEnabled(metadataSection, componentName) {
				return nil
			}
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
	if err != nil {
		return nil, fmt.Errorf("error adding nodes to dependency graph: %w", err)
	}

	// Second pass: build dependencies using settings.depends_on
	err = walkTerraformComponents(stacks, func(stackName, componentName string, componentSection map[string]any) error {
		// Skip abstract components
		if metadataSection, ok := componentSection[cfg.MetadataSectionName].(map[string]any); ok {
			if metadataType, ok := metadataSection["type"].(string); ok && metadataType == "abstract" {
				return nil
			}
		}

		// Skip disabled components
		if metadataSection, ok := componentSection[cfg.MetadataSectionName].(map[string]any); ok {
			if !isComponentEnabled(metadataSection, componentName) {
				return nil
			}
		}

		fromID := fmt.Sprintf("%s-%s", componentName, stackName)

		// Check for dependencies in settings.depends_on
		if settingsSection, ok := componentSection[cfg.SettingsSectionName].(map[string]any); ok {
			if dependsOn, ok := settingsSection["depends_on"].([]any); ok {
				for _, dep := range dependsOn {
					if depMap, ok := dep.(map[string]any); ok {
						if depComponent, ok := depMap["component"].(string); ok {
							// Default to same stack if not specified
							depStack := stackName
							if depStackVal, ok := depMap["stack"].(string); ok {
								depStack = depStackVal
							}

							toID := fmt.Sprintf("%s-%s", depComponent, depStack)

							// Only add dependency if the target node exists
							if _, exists := nodeMap[toID]; exists {
								if err := builder.AddDependency(fromID, toID); err != nil {
									log.Warn("Failed to add dependency", "from", fromID, "to", toID, "error", err)
								}
							} else {
								log.Warn("Dependency target not found", "from", fromID, "to", toID)
							}
						}
					} else if depMap, ok := dep.(map[any]any); ok {
						// Handle map[any]any case
						if depComponent, ok := depMap["component"].(string); ok {
							depStack := stackName
							if depStackVal, ok := depMap["stack"].(string); ok {
								depStack = depStackVal
							}

							toID := fmt.Sprintf("%s-%s", depComponent, depStack)

							if _, exists := nodeMap[toID]; exists {
								if err := builder.AddDependency(fromID, toID); err != nil {
									log.Warn("Failed to add dependency", "from", fromID, "to", toID, "error", err)
								}
							} else {
								log.Warn("Dependency target not found", "from", fromID, "to", toID)
							}
						}
					}
				}
			} else if dependsOn, ok := settingsSection["depends_on"].(map[string]any); ok {
				// Handle map format: depends_on: { "1": { component: "vpc" } }
				for _, dep := range dependsOn {
					if depMap, ok := dep.(map[string]any); ok {
						if depComponent, ok := depMap["component"].(string); ok {
							depStack := stackName
							if depStackVal, ok := depMap["stack"].(string); ok {
								depStack = depStackVal
							}

							toID := fmt.Sprintf("%s-%s", depComponent, depStack)

							if _, exists := nodeMap[toID]; exists {
								if err := builder.AddDependency(fromID, toID); err != nil {
									log.Warn("Failed to add dependency", "from", fromID, "to", toID, "error", err)
								}
							} else {
								log.Warn("Dependency target not found", "from", fromID, "to", toID)
							}
						}
					}
				}
			}
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("error building dependencies: %w", err)
	}

	// Build the final graph
	graph, err := builder.Build()
	if err != nil {
		return nil, fmt.Errorf("error finalizing dependency graph: %w", err)
	}

	log.Debug("Dependency graph built", "nodes", graph.Size(), "roots", len(graph.Roots))
	return graph, nil
}

// applyFiltersToGraph applies query and component filters to the graph.
func applyFiltersToGraph(graph *dependency.Graph, stacks map[string]any, info *schema.ConfigAndStacksInfo) *dependency.Graph {
	nodeIDs := []string{}

	// If specific components are specified, filter to those
	if len(info.Components) > 0 {
		for _, node := range graph.Nodes {
			for _, comp := range info.Components {
				if node.Component == comp {
					if info.Stack == "" || node.Stack == info.Stack {
						nodeIDs = append(nodeIDs, node.ID)
					}
				}
			}
		}
	} else if info.Stack != "" {
		// Filter to specific stack
		for _, node := range graph.Nodes {
			if node.Stack == info.Stack {
				nodeIDs = append(nodeIDs, node.ID)
			}
		}
	}

	// Apply YQ query if specified
	if info.Query != "" {
		filteredNodeIDs := []string{}
		for _, nodeID := range nodeIDs {
			node := graph.Nodes[nodeID]
			if node.Metadata != nil {
				queryResult, err := u.EvaluateYqExpression(&schema.AtmosConfiguration{}, node.Metadata, info.Query)
				if err == nil {
					if queryPassed, ok := queryResult.(bool); ok && queryPassed {
						filteredNodeIDs = append(filteredNodeIDs, nodeID)
					}
				}
			}
		}
		nodeIDs = filteredNodeIDs
	}

	// If no specific filters, include all
	if len(info.Components) == 0 && info.Stack == "" && info.Query == "" {
		for id := range graph.Nodes {
			nodeIDs = append(nodeIDs, id)
		}
	}

	// Filter the graph
	return graph.Filter(dependency.Filter{
		NodeIDs:             nodeIDs,
		IncludeDependencies: true,  // Include what these components depend on
		IncludeDependents:   false, // Don't include what depends on these
	})
}
