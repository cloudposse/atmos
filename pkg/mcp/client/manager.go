package client

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestResult contains the results of testing an MCP server.
type TestResult struct {
	ServerStarted bool
	Initialized   bool
	ToolCount     int
	PingOK        bool
	Error         error
}

// Manager manages the lifecycle of external MCP server processes.
type Manager struct {
	sessions map[string]*Session
	mu       sync.RWMutex
}

// NewManager creates a Manager from atmos configuration.
func NewManager(servers map[string]schema.MCPServerConfig) (*Manager, error) {
	sessions := make(map[string]*Session, len(servers))

	for name, cfg := range servers {
		parsed, err := ParseConfig(name, cfg)
		if err != nil {
			return nil, err
		}
		sessions[name] = NewSession(parsed)
	}

	return &Manager{sessions: sessions}, nil
}

// Get returns a session by name.
func (m *Manager) Get(name string) (*Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, ok := m.sessions[name]
	if !ok {
		return nil, fmt.Errorf("%w: %q", errUtils.ErrMCPServerNotFound, name)
	}
	return session, nil
}

// List returns all sessions sorted by name.
func (m *Manager) List() []*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		result = append(result, s)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].name < result[j].name
	})
	return result
}

// Start starts a specific server with optional start options (e.g., auth).
func (m *Manager) Start(ctx context.Context, name string, opts ...StartOption) error {
	session, err := m.Get(name)
	if err != nil {
		return err
	}
	return session.Start(ctx, opts...)
}

// Stop stops a specific server.
func (m *Manager) Stop(name string) error {
	session, err := m.Get(name)
	if err != nil {
		return err
	}
	return session.Stop()
}

// StopAll stops all running servers.
func (m *Manager) StopAll() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var errs []error
	for _, s := range m.sessions {
		if s.Status() == StatusRunning {
			if err := s.Stop(); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

// Test tests connectivity to an MCP server by starting it,
// listing tools, and pinging the server.
func (m *Manager) Test(ctx context.Context, name string, opts ...StartOption) *TestResult {
	result := &TestResult{}

	session, err := m.Get(name)
	if err != nil {
		result.Error = err
		return result
	}

	// Start the server.
	if err := session.Start(ctx, opts...); err != nil {
		result.Error = err
		return result
	}
	result.ServerStarted = true
	result.Initialized = true

	// Count tools.
	tools := session.Tools()
	result.ToolCount = len(tools)

	// Ping.
	if err := session.Ping(ctx); err != nil {
		result.Error = err
		return result
	}
	result.PingOK = true

	return result
}
