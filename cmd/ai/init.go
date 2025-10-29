package ai

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

	// Register all Atmos tools (components, stacks, validation, etc.).
	// Pass nil for LSP manager as it's not initialized in the command layer.
	if err := atmosTools.RegisterTools(registry, atmosConfig, nil); err != nil {
		log.Warn(fmt.Sprintf("Failed to register Atmos tools: %v", err))
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
