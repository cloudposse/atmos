package cmd

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	atmosTools "github.com/cloudposse/atmos/pkg/ai/tools/atmos"
	"github.com/cloudposse/atmos/pkg/ai/tools/permission"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

// initializeAIToolsAndExecutor initializes the AI tool registry and executor.
// This is shared by both 'atmos ai chat' and 'atmos mcp-server' commands.
func initializeAIToolsAndExecutor(atmosConfig *schema.AtmosConfiguration) (*tools.Registry, *tools.Executor, error) {
	if !atmosConfig.Settings.AI.Tools.Enabled {
		return nil, nil, errUtils.ErrAIToolsDisabled
	}

	log.Debug("Initializing AI tools")

	// Create tool registry.
	registry := tools.NewRegistry()

	// Register Atmos tools.
	if err := registry.Register(atmosTools.NewDescribeComponentTool(atmosConfig)); err != nil {
		log.Warn(fmt.Sprintf("Failed to register describe_component tool: %v", err))
	}
	if err := registry.Register(atmosTools.NewListStacksTool(atmosConfig)); err != nil {
		log.Warn(fmt.Sprintf("Failed to register list_stacks tool: %v", err))
	}
	if err := registry.Register(atmosTools.NewValidateStacksTool(atmosConfig)); err != nil {
		log.Warn(fmt.Sprintf("Failed to register validate_stacks tool: %v", err))
	}

	// Register file access tools (read/write for components and stacks).
	if err := registry.Register(atmosTools.NewReadComponentFileTool(atmosConfig)); err != nil {
		log.Warn(fmt.Sprintf("Failed to register read_component_file tool: %v", err))
	}
	if err := registry.Register(atmosTools.NewReadStackFileTool(atmosConfig)); err != nil {
		log.Warn(fmt.Sprintf("Failed to register read_stack_file tool: %v", err))
	}
	if err := registry.Register(atmosTools.NewWriteComponentFileTool(atmosConfig)); err != nil {
		log.Warn(fmt.Sprintf("Failed to register write_component_file tool: %v", err))
	}
	if err := registry.Register(atmosTools.NewWriteStackFileTool(atmosConfig)); err != nil {
		log.Warn(fmt.Sprintf("Failed to register write_stack_file tool: %v", err))
	}

	log.Debug(fmt.Sprintf("Registered %d tools", registry.Count()))

	// Create permission checker.
	permConfig := &permission.Config{
		Mode:            getPermissionMode(atmosConfig),
		AllowedTools:    atmosConfig.Settings.AI.Tools.AllowedTools,
		RestrictedTools: atmosConfig.Settings.AI.Tools.RestrictedTools,
		BlockedTools:    atmosConfig.Settings.AI.Tools.BlockedTools,
		YOLOMode:        atmosConfig.Settings.AI.Tools.YOLOMode,
	}
	permChecker := permission.NewChecker(permConfig, permission.NewCLIPrompter())

	// Create tool executor.
	executor := tools.NewExecutor(registry, permChecker, tools.DefaultTimeout)
	log.Debug("Tool executor initialized")

	return registry, executor, nil
}
