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
	node dependency.Node,
	info *schema.ConfigAndStacksInfo,
) error {
	// Skip abstract components (double check even though they shouldn't be in the graph)
	if metadata, ok := node.Metadata[cfg.MetadataSectionName].(map[string]any); ok {
		if metadataType, ok := metadata["type"].(string); ok && metadataType == "abstract" {
			log.Debug("Skipping abstract component", "component", node.Component, "stack", node.Stack)
			return nil
		}

		// Skip disabled components
		if !isNodeComponentEnabled(metadata, node.Component) {
			log.Debug("Skipping disabled component", "component", node.Component, "stack", node.Stack)
			return nil
		}
	}

	// Set component and stack information
	info.Component = node.Component
	info.ComponentFromArg = node.Component
	info.Stack = node.Stack
	info.StackFromArg = node.Stack

	command := fmt.Sprintf("atmos terraform %s %s -s %s", info.SubCommand, node.Component, node.Stack)

	// Apply query filter if specified
	if info.Query != "" && node.Metadata != nil {
		atmosConfig, err := cfg.InitCliConfig(*info, true)
		if err != nil {
			return fmt.Errorf("error initializing CLI config: %w", err)
		}

		queryResult, err := u.EvaluateYqExpression(&atmosConfig, node.Metadata, info.Query)
		if err != nil {
			return fmt.Errorf("error evaluating query expression: %w", err)
		}

		if queryPassed, ok := queryResult.(bool); !ok || !queryPassed {
			log.Debug("Skipping component due to query criteria", "command", command, "query", info.Query)
			return nil
		}
	}

	if info.DryRun {
		log.Info("Would execute", "command", command)
		return nil
	}

	log.Debug("Executing", "command", command)

	// Execute the terraform command
	if err := ExecuteTerraform(*info); err != nil {
		return fmt.Errorf("terraform execution failed: %w", err)
	}

	return nil
}

// isNodeComponentEnabled checks if a component is enabled based on metadata.
func isNodeComponentEnabled(metadata map[string]any, componentName string) bool {
	// Check if explicitly disabled
	if enabled, ok := metadata["enabled"].(bool); ok && !enabled {
		return false
	}

	// Component is enabled by default
	return true
}
