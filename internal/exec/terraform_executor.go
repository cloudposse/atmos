package exec

import (
	"bytes"
	"fmt"

	log "github.com/charmbracelet/log"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// execTerraformFn is the function used to execute a Terraform command.
// Package-level variable to allow test injection without gomonkey.
var execTerraformFn = ExecuteTerraform

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
	if isAbstractMetadata(metadata) {
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

// isAbstractMetadata checks if metadata indicates an abstract component.
func isAbstractMetadata(metadata map[string]any) bool {
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
		return false, fmt.Errorf("%w: %w", errUtils.ErrInitializeCLIConfig, err)
	}

	queryResult, err := u.EvaluateYqExpression(&atmosConfig, node.Metadata, info.Query)
	if err != nil {
		return false, fmt.Errorf("%w: %w", errUtils.ErrQueryEvaluation, err)
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
		// Match the user-facing dry-run message format emitted by ExecuteTerraformQuery
		// so callers see a consistent "Would <subcmd> `<component>` in `<stack>` (dry run)"
		// line for both multi-component execution paths.
		ui.Successf("Would %s `%s` in `%s` (dry run)", info.SubCommand, node.Component, node.Stack)
		log.Debug("Dry-run", "command", command)
		return nil
	}

	log.Debug("Executing", "command", command)

	// When a per-component hook is registered, capture this component's output
	// and invoke the hook immediately after execution so each component receives
	// its own CI summary entry rather than sharing the final global call.
	if info.PerComponentHook != nil {
		var stdoutBuf, stderrBuf bytes.Buffer
		execErr := execTerraformFn(*info, WithStdoutCapture(&stdoutBuf), WithStderrCapture(&stderrBuf))
		combined := stdoutBuf.String()
		if s := stderrBuf.String(); s != "" {
			combined += "\n" + s
		}
		compInfo := *info // snapshot with this component's Component/Stack values set.
		info.PerComponentHook(&compInfo, combined, execErr)
		if execErr != nil {
			return fmt.Errorf("%w: %w", errUtils.ErrTerraformExecFailed, execErr)
		}
		return nil
	}

	// Execute the terraform command.
	if err := execTerraformFn(*info); err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrTerraformExecFailed, err)
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
