package client

import (
	"context"
	"time"

	"github.com/cloudposse/atmos/pkg/ai/tools"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const registrationTimeout = 60 * time.Second

// RegisterMCPTools starts configured MCP servers and registers their tools
// in the Atmos AI tool registry. Returns the Manager so the caller can stop
// all servers on exit.
//
// Optional providers:
//   - authProvider: injects credentials for servers with auth_identity configured.
//   - toolchain: resolves command binaries and provides toolchain PATH for
//     prerequisites like uvx/npx that may be managed by Atmos toolchain.
//
// Servers that fail to start are logged as warnings but do not prevent
// other servers from registering.
func RegisterMCPTools(
	registry *tools.Registry,
	atmosConfig *schema.AtmosConfiguration,
	authProvider AuthEnvProvider,
	toolchain ToolchainResolver,
) (*Manager, error) {
	defer perf.Track(atmosConfig, "mcp.client.RegisterMCPTools")()
	if len(atmosConfig.MCP.Servers) == 0 {
		return nil, nil
	}

	mgr, err := NewManager(atmosConfig.MCP.Servers)
	if err != nil {
		return nil, err
	}

	startOpts := buildStartOptions(authProvider, toolchain)
	totalTools := startAndRegisterTools(mgr, registry, startOpts)

	if totalTools > 0 {
		log.Debugf("Registered %d MCP server tools", totalTools)
	}

	return mgr, nil
}

// RegisterReadOnlyMCPTools starts only MCP servers marked as read_only and registers
// their tools. This is used by non-interactive commands like 'atmos ai ask' where
// only data-retrieval tools (docs, pricing, etc.) are appropriate.
func RegisterReadOnlyMCPTools(
	registry *tools.Registry,
	atmosConfig *schema.AtmosConfiguration,
	authProvider AuthEnvProvider,
	toolchain ToolchainResolver,
) error {
	defer perf.Track(atmosConfig, "mcp.client.RegisterReadOnlyMCPTools")()

	// Filter to read-only servers only.
	readOnlyServers := make(map[string]schema.MCPServerConfig)
	for name, cfg := range atmosConfig.MCP.Servers {
		if cfg.ReadOnly {
			readOnlyServers[name] = cfg
		}
	}
	if len(readOnlyServers) == 0 {
		return nil
	}

	mgr, err := NewManager(readOnlyServers)
	if err != nil {
		return err
	}

	startOpts := buildStartOptions(authProvider, toolchain)
	totalTools := startAndRegisterTools(mgr, registry, startOpts)

	if totalTools > 0 {
		log.Debugf("Registered %d read-only MCP server tools", totalTools)
	}

	return nil
}

// buildStartOptions creates StartOption slice from optional providers.
func buildStartOptions(authProvider AuthEnvProvider, toolchain ToolchainResolver) []StartOption {
	var opts []StartOption
	if toolchain != nil {
		opts = append(opts, WithToolchain(toolchain))
	}
	if authProvider != nil {
		opts = append(opts, WithAuthManager(authProvider))
	}
	return opts
}

// startAndRegisterTools starts all sessions and registers their bridged tools.
func startAndRegisterTools(mgr *Manager, registry *tools.Registry, startOpts []StartOption) int {
	ctx, cancel := context.WithTimeout(context.Background(), registrationTimeout)
	defer cancel()

	var totalTools int
	for _, session := range mgr.List() {
		if err := session.Start(ctx, startOpts...); err != nil {
			log.Warnf("MCP server %q failed to start: %v", session.Name(), err)
			continue
		}

		for _, bt := range BridgeTools(session) {
			if regErr := registry.Register(bt); regErr != nil {
				log.Warnf("Failed to register MCP tool %q: %v", bt.Name(), regErr)
				continue
			}
			totalTools++
		}
	}
	return totalTools
}
