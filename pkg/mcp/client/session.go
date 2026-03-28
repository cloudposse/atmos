package client

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/version"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// SessionStatus represents the state of an MCP client session.
type SessionStatus string

const (
	// StatusStopped indicates the session is not running.
	StatusStopped SessionStatus = "stopped"
	// StatusStarting indicates the session is being initialized.
	StatusStarting SessionStatus = "starting"
	// StatusRunning indicates the session is connected and ready.
	StatusRunning SessionStatus = "running"
	// StatusError indicates the session failed to start or lost connection.
	StatusError SessionStatus = "error"
)

// Session wraps an MCP client connection to an external server.
type Session struct {
	name      string
	config    *ParsedConfig
	client    *mcpsdk.Client
	session   *mcpsdk.ClientSession
	tools     []*mcpsdk.Tool
	status    SessionStatus
	lastError error
	mu        sync.RWMutex
}

// NewSession creates a new session in stopped state.
func NewSession(config *ParsedConfig) *Session {
	return &Session{
		name:   config.Name,
		config: config,
		status: StatusStopped,
	}
}

// Name returns the server name.
func (s *Session) Name() string {
	return s.name
}

// Config returns the parsed configuration.
func (s *Session) Config() *ParsedConfig {
	return s.config
}

// Status returns the current session status.
func (s *Session) Status() SessionStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status
}

// LastError returns the last error that occurred.
func (s *Session) LastError() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastError
}

// Tools returns the cached list of tools from the MCP server.
func (s *Session) Tools() []*mcpsdk.Tool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.tools
}

const logFieldName = "name"

// Start connects to the external MCP server subprocess.
// StartOptions (e.g., WithAuthManager) can modify the subprocess environment.
func (s *Session) Start(ctx context.Context, opts ...StartOption) error {
	defer perf.Track(nil, "mcp.client.Session.Start")()
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.status == StatusRunning {
		return nil
	}

	s.status = StatusStarting
	log.Debug("Starting MCP server", logFieldName, s.name, "command", s.config.Command)

	env := prepareEnv(ctx, s.config, opts)

	if err := s.connectAndDiscover(ctx, env); err != nil {
		return err
	}

	log.Debug("MCP server started", logFieldName, s.name, "tools", len(s.tools))
	return nil
}

// prepareEnv builds the subprocess environment with optional auth credential injection.
func prepareEnv(ctx context.Context, config *ParsedConfig, opts []StartOption) []string {
	env := buildEnv(config.Env)
	for _, opt := range opts {
		var err error
		env, err = opt(ctx, config, env)
		if err != nil {
			log.Warnf("MCP server %q: auth setup failed: %v", config.Name, err)
		}
	}
	return env
}

// connectAndDiscover spawns the MCP server, performs the handshake, and lists tools.
func (s *Session) connectAndDiscover(ctx context.Context, env []string) error {
	// Resolve the command using the subprocess environment PATH, not the parent process PATH.
	// This is necessary because toolchain binaries (uvx, npx) may only exist in the
	// toolchain PATH prepended by WithToolchain, not in the system PATH.
	command := resolveCommandInEnv(s.config.Command, env)
	cmd := exec.CommandContext(ctx, command, s.config.Args...)
	cmd.Env = env
	cmd.Stderr = os.Stderr

	s.client = mcpsdk.NewClient(&mcpsdk.Implementation{
		Name:    "atmos",
		Version: version.Version,
	}, nil)

	session, err := s.client.Connect(ctx, &mcpsdk.CommandTransport{Command: cmd}, nil)
	if err != nil {
		s.status = StatusError
		s.lastError = fmt.Errorf("%w: %s: %w", errUtils.ErrMCPServerStartFailed, s.name, err)
		return s.lastError
	}
	s.session = session

	toolsResult, err := s.session.ListTools(ctx, nil)
	if err != nil {
		s.status = StatusError
		s.lastError = fmt.Errorf("%w: %s: %w", errUtils.ErrMCPServerToolListFailed, s.name, err)
		_ = s.session.Close()
		s.session = nil
		return s.lastError
	}

	s.tools = toolsResult.Tools
	s.status = StatusRunning
	s.lastError = nil
	return nil
}

// Stop gracefully shuts down the MCP server subprocess.
func (s *Session) Stop() error {
	defer perf.Track(nil, "mcp.client.Session.Stop")()
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.session == nil {
		s.status = StatusStopped
		return nil
	}

	log.Debug("Stopping MCP server", logFieldName, s.name)
	err := s.session.Close()
	s.session = nil
	s.client = nil
	s.tools = nil
	s.status = StatusStopped
	return err
}

// CallTool calls a tool on the MCP server.
func (s *Session) CallTool(ctx context.Context, toolName string, args map[string]any) (*mcpsdk.CallToolResult, error) {
	defer perf.Track(nil, "mcp.client.Session.CallTool")()
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.status != StatusRunning || s.session == nil {
		return nil, fmt.Errorf("%w: %s", errUtils.ErrMCPServerNotRunning, s.name)
	}

	return s.session.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      toolName,
		Arguments: args,
	})
}

// Ping checks connectivity to the MCP server.
func (s *Session) Ping(ctx context.Context) error {
	defer perf.Track(nil, "mcp.client.Session.Ping")()
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.status != StatusRunning || s.session == nil {
		return fmt.Errorf("%w: %s", errUtils.ErrMCPServerNotRunning, s.name)
	}

	return s.session.Ping(ctx, nil)
}

// StartOption is a function that modifies the subprocess environment before starting.
type StartOption func(ctx context.Context, config *ParsedConfig, env []string) ([]string, error)

// WithAuthManager returns a StartOption that injects auth credentials when
// the server has auth_identity configured.
func WithAuthManager(authMgr AuthEnvProvider) StartOption {
	return func(ctx context.Context, config *ParsedConfig, env []string) ([]string, error) {
		if config.AuthIdentity == "" || authMgr == nil {
			return env, nil
		}
		log.Debug("Injecting auth credentials for MCP server",
			logFieldName, config.Name, "identity", config.AuthIdentity)
		return authMgr.PrepareShellEnvironment(ctx, config.AuthIdentity, env)
	}
}

// AuthEnvProvider is the subset of auth.AuthManager needed for MCP credential injection.
type AuthEnvProvider interface {
	PrepareShellEnvironment(ctx context.Context, identityName string, currentEnv []string) ([]string, error)
}

// ToolchainResolver resolves command binary paths and provides toolchain PATH.
type ToolchainResolver interface {
	Resolve(command string) string
	EnvVars() []string
}

// WithToolchain returns a StartOption that resolves the MCP server command
// binary via the Atmos toolchain and prepends toolchain PATH to the subprocess
// environment. This ensures prerequisites like uvx/npx are available even if
// not on the system PATH.
func WithToolchain(resolver ToolchainResolver) StartOption {
	return func(_ context.Context, config *ParsedConfig, env []string) ([]string, error) {
		if resolver == nil {
			return env, nil
		}

		// Resolve the command binary (auto-installs if managed by toolchain).
		resolved := resolver.Resolve(config.Command)
		if resolved != config.Command {
			log.Debug("Resolved MCP server command via toolchain",
				logFieldName, config.Name,
				"original", config.Command,
				"resolved", resolved)
			config.Command = resolved
		}

		// Prepend toolchain PATH so the subprocess can find toolchain binaries.
		toolchainEnv := resolver.EnvVars()
		if len(toolchainEnv) > 0 {
			env = append(env, toolchainEnv...)
		}

		return env, nil
	}
}

// resolveCommandInEnv looks up a command using the PATH from the given env list.
// If the command is already absolute or not found, it returns the original.
func resolveCommandInEnv(command string, env []string) string {
	if filepath.IsAbs(command) {
		return command
	}

	// Extract PATH from the env list (last entry wins).
	const pathPrefix = "PATH="
	var envPATH string
	for _, e := range env {
		if strings.HasPrefix(e, pathPrefix) {
			envPATH = e[len(pathPrefix):]
		}
	}
	if envPATH == "" {
		return command
	}

	// Search each PATH directory for the command.
	for _, dir := range filepath.SplitList(envPATH) {
		candidate := filepath.Join(dir, command)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}

	return command
}

// buildEnv creates the environment variable list for the subprocess.
// It starts with the current process environment and appends the configured vars.
func buildEnv(env map[string]string) []string {
	result := os.Environ()
	for k, v := range env {
		result = append(result, fmt.Sprintf("%s=%s", k, v))
	}
	return result
}
