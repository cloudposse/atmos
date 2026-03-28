package ai

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	atmosTools "github.com/cloudposse/atmos/pkg/ai/tools/atmos"
	"github.com/cloudposse/atmos/pkg/ai/tools/permission"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/dependencies"
	log "github.com/cloudposse/atmos/pkg/logger"
	mcpclient "github.com/cloudposse/atmos/pkg/mcp/client"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// aiToolsResult holds the result of AI tools initialization.
type aiToolsResult struct {
	Registry *tools.Registry
	Executor *tools.Executor
	MCPMgr   *mcpclient.Manager
}

// initializeAIToolsAndExecutor initializes the AI tool registry and executor.
// This is shared by both 'atmos ai chat' and 'atmos mcp start' commands.
func initializeAIToolsAndExecutor(atmosConfig *schema.AtmosConfiguration) (*aiToolsResult, error) {
	if !atmosConfig.AI.Tools.Enabled {
		return nil, errUtils.ErrAIToolsDisabled
	}

	log.Debug("Initializing AI tools")

	// Create tool registry.
	registry := tools.NewRegistry()

	// Register all Atmos tools (components, stacks, validation, etc.).
	// Pass nil for LSP manager as it's not initialized in the command layer.
	if err := atmosTools.RegisterTools(registry, atmosConfig, nil); err != nil {
		log.Warnf("Failed to register Atmos tools: %v", err)
	}

	// Register external MCP server tools (if configured).
	mcpMgr := registerMCPServerTools(registry, atmosConfig)

	ui.Info(fmt.Sprintf("AI tools initialized: %d total", registry.Count()))

	// Initialize permission cache for persistent decisions.
	permCache, err := permission.NewPermissionCache(atmosConfig.BasePath)
	if err != nil {
		log.Warnf("Failed to initialize permission cache: %v", err)
		// Continue without cache - will prompt every time.
		permCache = nil
	}

	// Create permission checker with cache-aware prompter.
	permConfig := &permission.Config{
		Mode:            getPermissionMode(atmosConfig),
		AllowedTools:    atmosConfig.AI.Tools.AllowedTools,
		RestrictedTools: atmosConfig.AI.Tools.RestrictedTools,
		BlockedTools:    atmosConfig.AI.Tools.BlockedTools,
		YOLOMode:        atmosConfig.AI.Tools.YOLOMode,
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

	return &aiToolsResult{
		Registry: registry,
		Executor: executor,
		MCPMgr:   mcpMgr,
	}, nil
}

// initializeAIReadOnlyTools initializes a tool executor with only read-only, in-process tools.
// This is used by non-interactive commands like 'ask' where subprocess tools and write tools
// are not appropriate.
func initializeAIReadOnlyTools(atmosConfig *schema.AtmosConfiguration) (*tools.Registry, *tools.Executor, error) {
	if !atmosConfig.AI.Tools.Enabled {
		return nil, nil, errUtils.ErrAIToolsDisabled
	}

	log.Debug("Initializing read-only AI tools")

	// Create tool registry with only read-only tools.
	registry := tools.NewRegistry()
	if err := atmosTools.RegisterReadOnlyTools(registry, atmosConfig); err != nil {
		log.Warnf("Failed to register read-only Atmos tools: %v", err)
	}

	// Register external MCP server tools (all configured servers).
	registerMCPServerTools(registry, atmosConfig)

	ui.Info(fmt.Sprintf("AI tools initialized: %d", registry.Count()))

	// Read-only tools don't require permissions, but create a permissive checker just in case.
	permConfig := &permission.Config{
		Mode: permission.ModeAllow,
	}
	permChecker := permission.NewChecker(permConfig, nil)

	executor := tools.NewExecutor(registry, permChecker, tools.DefaultTimeout)
	log.Debug("Read-only tool executor initialized")

	return registry, executor, nil
}

// registerMCPServerTools registers external MCP server tools with toolchain resolution
// and auth credential injection.
func registerMCPServerTools(registry *tools.Registry, atmosConfig *schema.AtmosConfiguration) *mcpclient.Manager {
	if len(atmosConfig.MCP.Servers) == 0 {
		return nil
	}

	toolchain := resolveToolchain(atmosConfig)
	authProvider := resolveAuthProvider(atmosConfig)

	mgr, err := mcpclient.RegisterMCPTools(registry, atmosConfig, authProvider, toolchain)
	if err != nil {
		ui.Error(fmt.Sprintf("Failed to initialize MCP servers: %v", err))
	}
	return mgr
}

// resolveToolchain attempts to create a toolchain resolver from .tool-versions or component deps.
func resolveToolchain(atmosConfig *schema.AtmosConfiguration) mcpclient.ToolchainResolver {
	// Load tool dependencies from .tool-versions so uvx/npx are resolved from the toolchain.
	deps, depsErr := dependencies.LoadToolVersionsDependencies(atmosConfig)
	if depsErr == nil && len(deps) > 0 {
		tenv, tenvErr := dependencies.NewEnvironmentFromDeps(atmosConfig, deps)
		if tenvErr == nil && tenv != nil {
			return tenv
		}
	}
	// Fall back to component-based resolution.
	tenv, tenvErr := dependencies.ForComponent(atmosConfig, "terraform", nil, nil)
	if tenvErr == nil && tenv != nil {
		return tenv
	}
	return nil
}

// resolveAuthProvider creates an auth provider if any MCP server needs credentials.
func resolveAuthProvider(atmosConfig *schema.AtmosConfiguration) mcpclient.AuthEnvProvider {
	if !serversNeedAuth(atmosConfig.MCP.Servers) {
		return nil
	}
	mgr, err := auth.CreateAndAuthenticateManagerWithAtmosConfig(
		"", &atmosConfig.Auth, cfg.IdentityFlagSelectValue, atmosConfig,
	)
	if err != nil {
		ui.Error(fmt.Sprintf("Failed to create auth manager for MCP servers: %v", err))
		return nil
	}
	return mgr
}

// serversNeedAuth returns true if any configured MCP server has auth_identity set.
func serversNeedAuth(servers map[string]schema.MCPServerConfig) bool {
	for _, s := range servers {
		if s.AuthIdentity != "" {
			return true
		}
	}
	return false
}
