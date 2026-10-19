package client

import (
	"context"
	"errors"
	"fmt"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/spinner"
)

const registrationTimeout = 60 * time.Second

// RegisterMCPTools starts configured MCP servers and registers their tools
// in the Atmos AI tool registry. Returns the Manager so the caller can stop
// all servers on exit.
//
// Optional providers:
//   - authProvider: injects credentials for servers with identity configured.
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
	// Suppress subprocess stderr in AI commands — MCP server log output pollutes AI responses.
	totalTools := startAndRegisterTools(mgr, registry, startOpts, true)

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
// When suppressStderr is true, MCP server subprocess output is not forwarded to os.Stderr.
func startAndRegisterTools(mgr *Manager, registry *tools.Registry, startOpts []StartOption, suppressStderr ...bool) int {
	ctx, cancel := context.WithTimeout(context.Background(), registrationTimeout)
	defer cancel()

	suppress := len(suppressStderr) > 0 && suppressStderr[0]

	var totalTools int
	for _, session := range mgr.List() {
		if suppress {
			session.SetSuppressStderr(true)
		}
		totalTools += startAndRegisterSession(ctx, session, registry, startOpts)
	}
	return totalTools
}

// shouldSkipOptionalServer reports whether a server start failure should be downgraded to a
// warn-and-skip instead of an error: the server is marked optional and failed specifically
// because its identity could not be resolved in the active auth context
// (errUtils.ErrMCPServerAuthUnavailable). Any other failure of an optional server, and any
// failure of a non-optional server, is still an error.
func shouldSkipOptionalServer(cfg *ParsedConfig, err error) bool {
	return cfg != nil && cfg.Optional && errors.Is(err, errUtils.ErrMCPServerAuthUnavailable)
}

// startAndRegisterSession starts a single MCP server session and registers its bridged
// tools, showing a spinner while the handshake is in flight — server startup (e.g. an
// npx/uvx cold start) can take several seconds with no other visible progress.
func startAndRegisterSession(ctx context.Context, session *Session, registry *tools.Registry, startOpts []StartOption) int {
	sp := spinner.New(fmt.Sprintf("Starting MCP server `%s`...", session.Name()))
	sp.Start()

	if err := session.Start(ctx, startOpts...); err != nil {
		// An optional server whose identity cannot be resolved in the active auth context is
		// skipped with a warning instead of erroring. This supports shared `mcp.servers` config
		// consumed across repos where a server's identity is not defined in every consumer.
		if shouldSkipOptionalServer(session.Config(), err) {
			sp.Stop()
			ui.Warning(fmt.Sprintf("Skipping optional MCP server `%s`: %v", session.Name(), err))
			return 0
		}
		sp.Error(fmt.Sprintf("MCP server `%s` failed to start: %v", session.Name(), err))
		return 0
	}

	bridged := BridgeTools(session)
	serverTools := 0
	for _, bt := range bridged {
		if regErr := registry.Register(bt); regErr != nil {
			ui.Error(fmt.Sprintf("Failed to register MCP tool `%s`: %v", bt.Name(), regErr))
			continue
		}
		serverTools++
	}
	sp.Success(fmt.Sprintf("MCP server `%s` started (%d tools)", session.Name(), serverTools))
	return serverTools
}
