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
// This is shared by both 'atmos ai chat' and 'atmos mcp start' commands.
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

	// Initialize permission cache for persistent decisions.
	permCache, err := permission.NewPermissionCache(atmosConfig.BasePath)
	if err != nil {
		log.Warn(fmt.Sprintf("Failed to initialize permission cache: %v", err))
		// Continue without cache - will prompt every time.
		permCache = nil
	}

	// Create permission checker with cache-aware prompter.
	permConfig := &permission.Config{
		Mode:            getPermissionMode(atmosConfig),
		AllowedTools:    atmosConfig.Settings.AI.Tools.AllowedTools,
		RestrictedTools: atmosConfig.Settings.AI.Tools.RestrictedTools,
		BlockedTools:    atmosConfig.Settings.AI.Tools.BlockedTools,
		YOLOMode:        atmosConfig.Settings.AI.Tools.YOLOMode,
	}
	var prompter permission.Prompter
	if permCache != nil {
		prompter = permission.NewCLIPrompterWithCache(permCache)
	} else {
		prompter = permission.NewCLIPrompter()
	}
	permChecker := permission.NewChecker(permConfig, prompter)

	// Create tool executor.
	executor := tools.NewExecutor(registry, permChecker, tools.DefaultTimeout)
	log.Debug("Tool executor initialized")

	return registry, executor, nil
}

// initializeAIReadOnlyTools initializes a tool executor with only read-only, in-process tools.
// This is used by non-interactive commands like 'ask' where subprocess tools and write tools
// are not appropriate.
func initializeAIReadOnlyTools(atmosConfig *schema.AtmosConfiguration) (*tools.Registry, *tools.Executor, error) {
	if !atmosConfig.Settings.AI.Tools.Enabled {
		return nil, nil, errUtils.ErrAIToolsDisabled
	}

	log.Debug("Initializing read-only AI tools")

	// Create tool registry with only read-only tools.
	registry := tools.NewRegistry()
	if err := atmosTools.RegisterReadOnlyTools(registry, atmosConfig); err != nil {
		log.Warn(fmt.Sprintf("Failed to register read-only Atmos tools: %v", err))
	}

	log.Debug(fmt.Sprintf("Registered %d read-only tools", registry.Count()))

	// Read-only tools don't require permissions, but create a permissive checker just in case.
	permConfig := &permission.Config{
		Mode: permission.ModeAllow,
	}
	permChecker := permission.NewChecker(permConfig, nil)

	executor := tools.NewExecutor(registry, permChecker, tools.DefaultTimeout)
	log.Debug("Read-only tool executor initialized")

	return registry, executor, nil
}
