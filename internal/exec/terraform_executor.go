package exec

import (
	"fmt"

	log "github.com/charmbracelet/log"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// executeTerraformForNode executes terraform for a single dependency graph node.
func executeTerraformForNode(
	node *dependency.Node,
	info *schema.ConfigAndStacksInfo,
) error {
	// Validate the node.
	if shouldSkipNode(node) {
		return nil
	}

	// Apply query filter if specified.
	if shouldSkipByQuery(node, info) {
		return nil
	}

	// Update info with node details.
	updateInfoFromNode(info, node)

	// Execute the command.
	return executeNodeCommand(node, info)
}

// shouldSkipNode checks if a node should be skipped based on its metadata.
func shouldSkipNode(node *dependency.Node) bool {
	metadata, ok := node.Metadata[cfg.MetadataSectionName].(map[string]any)
	if !ok {
		return false
	}

	// Skip abstract components.
	if isAbstractComponent(metadata) {
		log.Debug("Skipping abstract component", "component", node.Component, "stack", node.Stack)
		return true
	}

	// Skip disabled components.
	if !isNodeComponentEnabled(metadata, node.Component) {
		log.Debug("Skipping disabled component", "component", node.Component, "stack", node.Stack)
		return true
	}

	return false
}

// isAbstractComponent checks if metadata indicates an abstract component.
func isAbstractComponent(metadata map[string]any) bool {
	metadataType, ok := metadata["type"].(string)
	return ok && metadataType == "abstract"
}

// shouldSkipByQuery checks if a node should be skipped based on query filter.
func shouldSkipByQuery(node *dependency.Node, info *schema.ConfigAndStacksInfo) bool {
	if info.Query == "" || node.Metadata == nil {
		return false
	}

	passed, err := evaluateNodeQueryFilter(node, info)
	if err != nil {
		log.Debug("Error evaluating query", "error", err, "component", node.Component, "stack", node.Stack)
		return true
	}

	if !passed {
		command := formatNodeCommand(node, info)
		log.Debug("Skipping component due to query criteria", "command", command, "query", info.Query)
		return true
	}

	return false
}

// evaluateNodeQueryFilter evaluates a query filter against a node's metadata.
func evaluateNodeQueryFilter(node *dependency.Node, info *schema.ConfigAndStacksInfo) (bool, error) {
	atmosConfig, err := cfg.InitCliConfig(*info, true)
	if err != nil {
		return false, fmt.Errorf("error initializing CLI config: %w", err)
	}

	queryResult, err := u.EvaluateYqExpression(&atmosConfig, node.Metadata, info.Query)
	if err != nil {
		return false, fmt.Errorf("error evaluating query expression: %w", err)
	}

	queryPassed, ok := queryResult.(bool)
	return ok && queryPassed, nil
}

// updateInfoFromNode updates the ConfigAndStacksInfo with node details.
func updateInfoFromNode(info *schema.ConfigAndStacksInfo, node *dependency.Node) {
	info.Component = node.Component
	info.ComponentFromArg = node.Component
	info.Stack = node.Stack
	info.StackFromArg = node.Stack
}

// executeNodeCommand executes the terraform command for a node.
func executeNodeCommand(node *dependency.Node, info *schema.ConfigAndStacksInfo) error {
	command := formatNodeCommand(node, info)

	if info.DryRun {
		log.Info("Would execute", "command", command)
		return nil
	}

	log.Debug("Executing", "command", command)

	// Execute the terraform command.
	if err := ExecuteTerraform(*info); err != nil {
		return fmt.Errorf("terraform execution failed: %w", err)
	}

	return nil
}

// formatNodeCommand formats the terraform command string for a node.
func formatNodeCommand(node *dependency.Node, info *schema.ConfigAndStacksInfo) string {
	return fmt.Sprintf("atmos terraform %s %s -s %s", info.SubCommand, node.Component, node.Stack)
}

// isNodeComponentEnabled checks if a component is enabled based on metadata.
func isNodeComponentEnabled(metadata map[string]any, _ string) bool {
	// Check if explicitly disabled.
	if enabled, ok := metadata["enabled"].(bool); ok && !enabled {
		return false
	}

	// Component is enabled by default.
	return true
}
