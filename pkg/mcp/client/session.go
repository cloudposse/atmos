package client

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/version"
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
	name           string
	config         *ParsedConfig
	client         *mcpsdk.Client
	session        *mcpsdk.ClientSession
	tools          []*mcpsdk.Tool
	status         SessionStatus
	lastError      error
	suppressStderr bool
	mu             sync.RWMutex
}

// SetSuppressStderr controls whether subprocess stderr is forwarded to os.Stderr.
// When true, MCP server log output is suppressed (used during AI commands).
func (s *Session) SetSuppressStderr(suppress bool) {
	s.suppressStderr = suppress
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

	env, err := prepareEnv(ctx, s.config, opts)
	if err != nil {
		s.status = StatusError
		s.lastError = err
		return fmt.Errorf("%w: %s: %w", errUtils.ErrMCPServerStartFailed, s.name, err)
	}

	if err := s.connectAndDiscover(ctx, env); err != nil {
		return err
	}

	log.Debug("MCP server started", logFieldName, s.name, "tools", len(s.tools))
	return nil
}

// prepareEnv builds the subprocess environment with optional auth credential injection.
// Returns an error if any start option fails (e.g., auth credential resolution).
func prepareEnv(ctx context.Context, config *ParsedConfig, opts []StartOption) ([]string, error) {
	env := buildEnv(config.Env)
	for _, opt := range opts {
		var err error
		env, err = opt(ctx, config, env)
		if err != nil {
			return nil, fmt.Errorf("auth setup failed for %q: %w", config.Name, err)
		}
	}
	return env, nil
}

// connectAndDiscover spawns the MCP server, performs the handshake, and lists tools.
func (s *Session) connectAndDiscover(ctx context.Context, env []string) error {
	// Resolve the command using the subprocess environment PATH, not the parent process PATH.
	// This is necessary because toolchain binaries (uvx, npx) may only exist in the
	// toolchain PATH prepended by WithToolchain, not in the system PATH.
	command := resolveCommandInEnv(s.config.Command, env)
	cmd := exec.CommandContext(ctx, command, s.config.Args...)
	cmd.Env = env
	if !s.suppressStderr {
		cmd.Stderr = os.Stderr
	}

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
// the server has identity configured.
//
// If authMgr also implements PerServerAuthProvider, ForServer is called first
// to obtain a server-scoped AuthEnvProvider. This allows the auth manager to
// be (re)constructed per-server with the server's `env:` block applied — so
// ATMOS_PROFILE, ATMOS_CLI_CONFIG_PATH, etc. influence identity resolution.
func WithAuthManager(authMgr AuthEnvProvider) StartOption {
	return func(ctx context.Context, config *ParsedConfig, env []string) ([]string, error) {
		if config.Identity == "" {
			return env, nil
		}
		if authMgr == nil {
			return nil, fmt.Errorf("%w: server `%s`, identity `%s`", errUtils.ErrMCPServerAuthUnavailable, config.Name, config.Identity)
		}

		provider := authMgr
		if perServer, ok := authMgr.(PerServerAuthProvider); ok {
			scoped, err := perServer.ForServer(ctx, config)
			if err != nil {
				// ForServer returns errors already wrapped (pkg/auth wraps with
				// ErrAuthManager). prepareEnv adds "auth setup failed for X" and
				// Session.Start adds ErrMCPServerStartFailed + the server name,
				// so further wrapping here would only duplicate context.
				return nil, err
			}
			if scoped == nil {
				return nil, fmt.Errorf("%w: server `%s`, identity `%s`", errUtils.ErrMCPServerAuthUnavailable, config.Name, config.Identity)
			}
			provider = scoped
		}

		log.Debug("Injecting auth credentials for MCP server",
			logFieldName, config.Name, "identity", config.Identity)
		return provider.PrepareShellEnvironment(ctx, config.Identity, env)
	}
}

// AuthEnvProvider is the subset of auth.AuthManager needed for MCP credential injection.
type AuthEnvProvider interface {
	PrepareShellEnvironment(ctx context.Context, identityName string, currentEnv []string) ([]string, error)
}

// PerServerAuthProvider is an optional extension of AuthEnvProvider that
// returns a new AuthEnvProvider scoped to a specific server's configuration.
//
// Implementations should apply the server's `env:` block (specifically ATMOS_*
// variables) before constructing the underlying auth manager so that
// ATMOS_PROFILE, ATMOS_CLI_CONFIG_PATH, ATMOS_BASE_PATH, etc. influence atmos
// config loading and identity resolution.
//
// See ApplyAtmosEnvOverrides for the recommended env-application helper.
type PerServerAuthProvider interface {
	ForServer(ctx context.Context, config *ParsedConfig) (AuthEnvProvider, error)
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
// On Windows, also probes PATHEXT extensions (.exe, .cmd, etc.).
// If the command is already absolute or not found, it returns the original.
func resolveCommandInEnv(command string, env []string) string {
	// Absolute paths and relative paths with separators (./server, bin/server)
	// are returned as-is — only bare command names are searched in PATH.
	if filepath.IsAbs(command) || strings.ContainsAny(command, "/\\") {
		return command
	}

	envPATH, envPATHEXT := extractEnvVars(env)
	if envPATH == "" {
		return command
	}

	extensions := buildExtensions(envPATHEXT)

	// Search each PATH directory for the command (with extensions on Windows).
	for _, dir := range filepath.SplitList(envPATH) {
		if found := findExecutable(dir, command, extensions); found != "" {
			return found
		}
	}

	return command
}

// extractEnvVars extracts PATH and PATHEXT from an env list (last entry wins).
func extractEnvVars(env []string) (string, string) {
	var envPATH, envPATHEXT string
	for _, e := range env {
		if strings.HasPrefix(e, "PATH=") {
			envPATH = e[len("PATH="):]
		} else if strings.HasPrefix(e, "PATHEXT=") {
			envPATHEXT = e[len("PATHEXT="):]
		}
	}
	return envPATH, envPATHEXT
}

// buildExtensions builds a list of file extensions to try when resolving commands.
// Always includes empty string (bare name) first, then PATHEXT entries.
func buildExtensions(pathext string) []string {
	extensions := []string{""}
	if pathext == "" {
		return extensions
	}
	for _, ext := range strings.Split(pathext, ";") {
		if ext != "" {
			extensions = append(extensions, ext)
		}
	}
	return extensions
}

// findExecutable checks a directory for a command with any of the given extensions.
func findExecutable(dir, command string, extensions []string) string {
	for _, ext := range extensions {
		candidate := filepath.Join(dir, command+ext)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
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
