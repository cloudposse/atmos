package client

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cloudposse/atmos/pkg/lsp"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ManagerInterface defines the interface for LSP manager operations.
type ManagerInterface interface {
	GetClient(name string) (*Client, bool)
	GetClientForFile(filePath string) (*Client, bool)
	AnalyzeFile(filePath, content string) ([]lsp.Diagnostic, error)
	GetAllDiagnostics() map[string]map[string][]lsp.Diagnostic
	GetDiagnosticsForFile(filePath string) []lsp.Diagnostic
	Close() error
	IsEnabled() bool
	GetServerNames() []string
}

// Manager manages multiple LSP server clients.
type Manager struct {
	clients   map[string]*Client // name -> client
	clientsMu sync.RWMutex
	config    *schema.LSPSettings
	rootPath  string
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewManager creates a new LSP manager.
func NewManager(ctx context.Context, config *schema.LSPSettings, rootPath string) (*Manager, error) {
	if config == nil {
		return nil, fmt.Errorf("LSP config is nil")
	}

	// Create context with cancel
	managerCtx, cancel := context.WithCancel(ctx)

	manager := &Manager{
		clients:  make(map[string]*Client),
		config:   config,
		rootPath: rootPath,
		ctx:      managerCtx,
		cancel:   cancel,
	}

	// Start configured LSP servers
	if config.Enabled {
		if err := manager.startServers(); err != nil {
			cancel()
			return nil, err
		}
	}

	return manager, nil
}

// startServers starts all configured LSP servers.
func (m *Manager) startServers() error {
	m.clientsMu.Lock()
	defer m.clientsMu.Unlock()

	for name, serverConfig := range m.config.Servers {
		rootURI := "file://" + m.rootPath

		client, err := NewClient(m.ctx, name, serverConfig, rootURI)
		if err != nil {
			// Log error but continue with other servers
			continue
		}

		// Initialize the client
		if err := client.Initialize(); err != nil {
			_ = client.Close()
			// Log error but continue
			continue
		}

		m.clients[name] = client
	}

	return nil
}

// GetClient returns the LSP client for the specified server name.
func (m *Manager) GetClient(name string) (*Client, bool) {
	m.clientsMu.RLock()
	defer m.clientsMu.RUnlock()

	client, exists := m.clients[name]
	return client, exists
}

// GetClientForFile returns the LSP client that handles the given file.
func (m *Manager) GetClientForFile(filePath string) (*Client, bool) {
	ext := strings.TrimPrefix(filepath.Ext(filePath), ".")

	m.clientsMu.RLock()
	defer m.clientsMu.RUnlock()

	// Find client that supports this file type
	for _, client := range m.clients {
		for _, fileType := range client.config.FileTypes {
			if fileType == ext {
				return client, true
			}
		}
	}

	return nil, false
}

// AnalyzeFile opens a file in the appropriate LSP server and returns diagnostics.
// It waits up to 500ms for diagnostics to be published by the LSP server to handle
// asynchronous diagnostic publication.
func (m *Manager) AnalyzeFile(filePath, content string) ([]lsp.Diagnostic, error) {
	client, found := m.GetClientForFile(filePath)
	if !found {
		return nil, fmt.Errorf("no LSP server found for file: %s", filePath)
	}

	// Determine language ID from file extension
	ext := strings.TrimPrefix(filepath.Ext(filePath), ".")
	languageID := ext // Simple mapping, can be enhanced

	// Convert to file:// URI
	uri := "file://" + filePath

	// Open document
	if err := client.OpenDocument(uri, languageID, content); err != nil {
		return nil, fmt.Errorf("failed to open document: %w", err)
	}

	// Wait up to 500ms for diagnostics to be published.
	// LSP servers publish diagnostics asynchronously, so we need to poll
	// until diagnostics are available or timeout occurs.
	timeout := time.After(500 * time.Millisecond)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	var diagnostics []lsp.Diagnostic

	for {
		select {
		case <-timeout:
			// Return whatever diagnostics we have after timeout
			diagnostics = client.GetDiagnostics(uri)
			_ = client.CloseDocument(uri)
			return diagnostics, nil

		case <-ticker.C:
			// Check if diagnostics are available
			diagnostics = client.GetDiagnostics(uri)
			if len(diagnostics) > 0 {
				_ = client.CloseDocument(uri)
				return diagnostics, nil
			}
		}
	}
}

// GetAllDiagnostics returns all diagnostics from all LSP servers.
func (m *Manager) GetAllDiagnostics() map[string]map[string][]lsp.Diagnostic {
	m.clientsMu.RLock()
	defer m.clientsMu.RUnlock()

	result := make(map[string]map[string][]lsp.Diagnostic)
	for name, client := range m.clients {
		result[name] = client.GetAllDiagnostics()
	}

	return result
}

// GetDiagnosticsForFile returns diagnostics for a specific file from all servers.
func (m *Manager) GetDiagnosticsForFile(filePath string) []lsp.Diagnostic {
	uri := "file://" + filePath

	m.clientsMu.RLock()
	defer m.clientsMu.RUnlock()

	var allDiagnostics []lsp.Diagnostic
	for _, client := range m.clients {
		diagnostics := client.GetDiagnostics(uri)
		allDiagnostics = append(allDiagnostics, diagnostics...)
	}

	return allDiagnostics
}

// Close shuts down all LSP clients.
func (m *Manager) Close() error {
	m.cancel()

	m.clientsMu.Lock()
	defer m.clientsMu.Unlock()

	for _, client := range m.clients {
		_ = client.Close()
	}

	m.clients = make(map[string]*Client)
	return nil
}

// IsEnabled returns whether LSP is enabled.
func (m *Manager) IsEnabled() bool {
	return m.config != nil && m.config.Enabled
}

// GetServerNames returns list of configured server names.
func (m *Manager) GetServerNames() []string {
	m.clientsMu.RLock()
	defer m.clientsMu.RUnlock()

	names := make([]string, 0, len(m.clients))
	for name := range m.clients {
		names = append(names, name)
	}
	return names
}
