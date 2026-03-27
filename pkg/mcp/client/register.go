package client

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
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
// Servers that fail to start are reported as errors but do not prevent
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
		ui.Info(fmt.Sprintf("Registered %d tools from %d MCP server(s)", totalTools, len(mgr.List())))
	}

	return mgr, nil
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
			ui.Error(fmt.Sprintf("MCP server %q failed to start: %v", session.Name(), err))
			continue
		}

		bridged := BridgeTools(session)
		serverTools := 0
		for _, bt := range bridged {
			if regErr := registry.Register(bt); regErr != nil {
				ui.Error(fmt.Sprintf("Failed to register MCP tool %q: %v", bt.Name(), regErr))
				continue
			}
			serverTools++
		}
		totalTools += serverTools
		ui.Info(fmt.Sprintf("MCP server %q started (%d tools)", session.Name(), serverTools))
	}
	return totalTools
}
