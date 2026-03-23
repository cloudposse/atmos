package client

import (
	"context"
	"time"

	"github.com/cloudposse/atmos/pkg/ai/tools"
	log "github.com/cloudposse/atmos/pkg/logger"
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
	if len(atmosConfig.MCP.Servers) == 0 {
		return nil, nil
	}

	mgr, err := NewManager(atmosConfig.MCP.Servers)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), registrationTimeout)
	defer cancel()

	// Build start options.
	var startOpts []StartOption
	if toolchain != nil {
		startOpts = append(startOpts, WithToolchain(toolchain))
	}
	if authProvider != nil {
		startOpts = append(startOpts, WithAuthManager(authProvider))
	}

	var totalTools int

	for _, session := range mgr.List() {
		if err := session.Start(ctx, startOpts...); err != nil {
			log.Warnf("MCP server %q failed to start: %v", session.Name(), err)
			continue
		}

		bridged := BridgeTools(session)
		for _, bt := range bridged {
			if regErr := registry.Register(bt); regErr != nil {
				log.Warnf("Failed to register MCP tool %q: %v", bt.Name(), regErr)
				continue
			}
			totalTools++
		}
	}

	if totalTools > 0 {
		log.Debugf("Registered %d MCP server tools", totalTools)
	}

	return mgr, nil
}
