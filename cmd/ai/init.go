package ai

import (
	"context"
	"fmt"
	"sort"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	atmosTools "github.com/cloudposse/atmos/pkg/ai/tools/atmos"
	"github.com/cloudposse/atmos/pkg/ai/tools/permission"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/dependencies"
	log "github.com/cloudposse/atmos/pkg/logger"
	mcpclient "github.com/cloudposse/atmos/pkg/mcp/client"
	"github.com/cloudposse/atmos/pkg/mcp/router"
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
// Passing mcpServerNames filters which MCP servers to start (empty or nil = auto-route or all).
// The question parameter is used for automatic routing when mcpServerNames is empty or nil.
func initializeAIToolsAndExecutor(atmosConfig *schema.AtmosConfiguration, mcpServerNames []string, question string) (*aiToolsResult, error) {
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

	// Register external MCP server tools (filtered by routing).
	mcpMgr := registerMCPServerTools(registry, atmosConfig, mcpServerNames, question)

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

// aiReadOnlyResult holds the result of read-only AI tools initialization.
type aiReadOnlyResult struct {
	Registry *tools.Registry
	Executor *tools.Executor
	MCPMgr   *mcpclient.Manager
}

// initializeAIReadOnlyTools initializes a tool executor with only read-only, in-process tools.
// Passing mcpServerNames filters which MCP servers to start (empty or nil = auto-route or all).
// The question parameter is used for automatic routing when mcpServerNames is empty or nil.
func initializeAIReadOnlyTools(atmosConfig *schema.AtmosConfiguration, mcpServerNames []string, question string) (*aiReadOnlyResult, error) {
	if !atmosConfig.AI.Tools.Enabled {
		return nil, errUtils.ErrAIToolsDisabled
	}

	log.Debug("Initializing read-only AI tools")

	// Create tool registry with only read-only tools.
	registry := tools.NewRegistry()
	if err := atmosTools.RegisterReadOnlyTools(registry, atmosConfig); err != nil {
		log.Warnf("Failed to register read-only Atmos tools: %v", err)
	}

	// Register external MCP server tools (filtered by routing).
	mcpMgr := registerMCPServerTools(registry, atmosConfig, mcpServerNames, question)

	ui.Info(fmt.Sprintf("AI tools initialized: %d", registry.Count()))

	// Read-only tools don't require permissions, but create a permissive checker just in case.
	permConfig := &permission.Config{
		Mode: permission.ModeAllow,
	}
	permChecker := permission.NewChecker(permConfig, nil)

	executor := tools.NewExecutor(registry, permChecker, tools.DefaultTimeout)
	log.Debug("Read-only tool executor initialized")

	return &aiReadOnlyResult{
		Registry: registry,
		Executor: executor,
		MCPMgr:   mcpMgr,
	}, nil
}

// registerMCPServerTools registers external MCP server tools with toolchain resolution,
// auth credential injection, and optional server routing.
func registerMCPServerTools(registry *tools.Registry, atmosConfig *schema.AtmosConfiguration, mcpServerNames []string, question string) *mcpclient.Manager {
	if len(atmosConfig.MCP.Servers) == 0 {
		return nil
	}

	// Select which servers to start.
	selectedServers := selectMCPServers(atmosConfig, mcpServerNames, question)
	if len(selectedServers) == 0 {
		return nil
	}

	// Create a filtered copy of the config for RegisterMCPTools.
	filteredConfig := *atmosConfig
	filteredConfig.MCP.Servers = selectedServers

	toolchain := resolveToolchain(atmosConfig)
	authProvider := resolveAuthProvider(&filteredConfig)

	mgr, err := mcpclient.RegisterMCPTools(registry, &filteredConfig, authProvider, toolchain)
	if err != nil {
		ui.Error(fmt.Sprintf("Failed to initialize MCP servers: %v", err))
	}
	return mgr
}

// selectMCPServers determines which MCP servers to start based on:
// 1. Manual override via --mcp flag (mcpServerNames).
// 2. Two-pass AI routing using a fast model.
// 3. All servers (fallback).
func selectMCPServers(atmosConfig *schema.AtmosConfiguration, mcpServerNames []string, question string) map[string]schema.MCPServerConfig {
	servers := atmosConfig.MCP.Servers

	// Manual override via --mcp flag.
	if len(mcpServerNames) > 0 {
		filtered := filterServersByName(servers, mcpServerNames)
		// Warn about unrecognized server names (typos, removed servers).
		for _, name := range mcpServerNames {
			if _, ok := servers[name]; !ok {
				ui.Warning(fmt.Sprintf("MCP server %q not found in configuration (available: %s)",
					name, strings.Join(sortedServerNames(servers), ", ")))
			}
		}
		if len(filtered) > 0 {
			ui.Info(fmt.Sprintf("MCP servers selected via --mcp flag: %s", strings.Join(mcpServerNames, ", ")))
		}
		return filtered
	}

	// Single server — no routing needed.
	if len(servers) <= 1 {
		return servers
	}

	// Routing disabled in config.
	if !atmosConfig.MCP.Routing.IsEnabled() {
		return servers
	}

	// No question available (e.g., chat mode) — start all.
	if question == "" {
		return servers
	}

	// Two-pass routing with fast model.
	selected := routeWithAI(atmosConfig, question)
	if len(selected) == 0 {
		return servers
	}

	ui.Info(fmt.Sprintf("MCP routing selected %d of %d servers: %s",
		len(selected), len(servers), strings.Join(selected, ", ")))

	return filterServersByName(servers, selected)
}

// routeWithAI uses a fast model to select relevant MCP servers for a question.
func routeWithAI(atmosConfig *schema.AtmosConfiguration, question string) []string {
	client, err := createRoutingClient(atmosConfig)
	if err != nil {
		log.Debug("Failed to create routing client, starting all servers", "error", err)
		return nil
	}

	// Build server info list for routing.
	var serverInfos []router.ServerInfo
	for name, cfg := range atmosConfig.MCP.Servers {
		serverInfos = append(serverInfos, router.ServerInfo{
			Name:        name,
			Description: cfg.Description,
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), router.DefaultTimeout)
	defer cancel()

	return router.Route(ctx, client, question, serverInfos)
}

// createRoutingClient creates an AI client for the routing step.
// Uses the same provider and model the user already configured — no extra model config needed.
// Only overrides max_tokens to keep routing responses small.
func createRoutingClient(atmosConfig *schema.AtmosConfiguration) (router.MessageSender, error) {
	routingConfig := *atmosConfig

	// Override max_tokens for routing (responses are just a JSON array of server names).
	provider := atmosConfig.AI.DefaultProvider
	if provider == "" {
		provider = "anthropic"
	}

	// Deep-copy the provider map to avoid mutating the original config.
	if atmosConfig.AI.Providers != nil {
		routingConfig.AI.Providers = make(map[string]*schema.AIProviderConfig, len(atmosConfig.AI.Providers))
		for k, v := range atmosConfig.AI.Providers {
			if v != nil {
				copied := *v
				routingConfig.AI.Providers[k] = &copied
			}
		}
		if existing, ok := routingConfig.AI.Providers[provider]; ok && existing != nil {
			existing.MaxTokens = router.DefaultMaxTokens()
		}
	}

	return ai.NewClient(&routingConfig)
}

// sortedServerNames returns server names sorted alphabetically.
func sortedServerNames(servers map[string]schema.MCPServerConfig) []string {
	names := make([]string, 0, len(servers))
	for name := range servers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// filterServersByName returns only servers whose names are in the given list.
func filterServersByName(servers map[string]schema.MCPServerConfig, names []string) map[string]schema.MCPServerConfig {
	filtered := make(map[string]schema.MCPServerConfig, len(names))
	for _, name := range names {
		if cfg, ok := servers[name]; ok {
			filtered[name] = cfg
		}
	}
	return filtered
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
