package client

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
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

// Name returns the integration name.
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

// Start connects to the external MCP server subprocess.
func (s *Session) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.status == StatusRunning {
		return nil
	}

	s.status = StatusStarting
	log.Debug("Starting MCP integration", "name", s.name, "command", s.config.Command)

	// Build the subprocess command.
	cmd := exec.CommandContext(ctx, s.config.Command, s.config.Args...) //nolint:gosec // Command comes from atmos.yaml config, not user input.
	cmd.Env = buildEnv(s.config.Env)
	cmd.Stderr = os.Stderr

	// Create the MCP client and transport.
	s.client = mcpsdk.NewClient(&mcpsdk.Implementation{
		Name:    "atmos",
		Version: version.Version,
	}, nil)

	transport := &mcpsdk.CommandTransport{
		Command: cmd,
	}

	// Connect to the server (performs initialization handshake).
	session, err := s.client.Connect(ctx, transport, nil)
	if err != nil {
		s.status = StatusError
		s.lastError = fmt.Errorf("%w: %s: %w", errUtils.ErrMCPIntegrationStartFailed, s.name, err)
		return s.lastError
	}
	s.session = session

	// List available tools.
	toolsResult, err := s.session.ListTools(ctx, nil)
	if err != nil {
		s.status = StatusError
		s.lastError = fmt.Errorf("failed to list tools from %q: %w", s.name, err)
		// Clean up the session.
		_ = s.session.Close()
		s.session = nil
		return s.lastError
	}

	s.tools = toolsResult.Tools
	s.status = StatusRunning
	s.lastError = nil

	log.Debug("MCP integration started", "name", s.name, "tools", len(s.tools))
	return nil
}

// Stop gracefully shuts down the MCP server subprocess.
func (s *Session) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.session == nil {
		s.status = StatusStopped
		return nil
	}

	log.Debug("Stopping MCP integration", "name", s.name)
	err := s.session.Close()
	s.session = nil
	s.client = nil
	s.tools = nil
	s.status = StatusStopped
	return err
}

// CallTool calls a tool on the MCP server.
func (s *Session) CallTool(ctx context.Context, toolName string, args map[string]any) (*mcpsdk.CallToolResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.status != StatusRunning || s.session == nil {
		return nil, fmt.Errorf("%w: %s", errUtils.ErrMCPIntegrationNotRunning, s.name)
	}

	return s.session.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      toolName,
		Arguments: args,
	})
}

// Ping checks connectivity to the MCP server.
func (s *Session) Ping(ctx context.Context) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.status != StatusRunning || s.session == nil {
		return fmt.Errorf("%w: %s", errUtils.ErrMCPIntegrationNotRunning, s.name)
	}

	return s.session.Ping(ctx, nil)
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
