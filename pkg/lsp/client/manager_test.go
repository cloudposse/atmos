package client

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/lsp"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewManager(t *testing.T) {
	tests := []struct {
		name      string
		config    *schema.LSPSettings
		expectErr bool
	}{
		{
			name:      "Nil config",
			config:    nil,
			expectErr: true,
		},
		{
			name: "Disabled LSP",
			config: &schema.LSPSettings{
				Enabled: false,
				Servers: map[string]*schema.LSPServer{},
			},
			expectErr: false,
		},
		{
			name: "Enabled with no servers",
			config: &schema.LSPSettings{
				Enabled: true,
				Servers: map[string]*schema.LSPServer{},
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			manager, err := NewManager(ctx, tt.config, "/test/root")

			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, manager)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, manager)
				if manager != nil {
					_ = manager.Close()
				}
			}
		})
	}
}

func TestManager_IsEnabled(t *testing.T) {
	tests := []struct {
		name     string
		config   *schema.LSPSettings
		expected bool
	}{
		{
			name:     "Nil config",
			config:   nil,
			expected: false,
		},
		{
			name: "Enabled",
			config: &schema.LSPSettings{
				Enabled: true,
			},
			expected: true,
		},
		{
			name: "Disabled",
			config: &schema.LSPSettings{
				Enabled: false,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := &Manager{
				config: tt.config,
			}
			result := manager.IsEnabled()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestManager_GetClient(t *testing.T) {
	ctx := context.Background()
	config := &schema.LSPSettings{
		Enabled: false, // Don't actually start servers
		Servers: map[string]*schema.LSPServer{
			"yaml-ls": {
				Command:   "yaml-language-server",
				Args:      []string{"--stdio"},
				FileTypes: []string{"yaml", "yml"},
			},
		},
	}

	manager, err := NewManager(ctx, config, "/test/root")
	require.NoError(t, err)
	defer manager.Close()

	// Test getting non-existent client
	client, found := manager.GetClient("nonexistent")
	assert.False(t, found)
	assert.Nil(t, client)

	// Test getting yaml-ls (won't exist since we didn't start servers)
	client, found = manager.GetClient("yaml-ls")
	assert.False(t, found)
	assert.Nil(t, client)
}

func TestManager_GetClientForFile(t *testing.T) {
	ctx := context.Background()

	// Create a manager with mock client setup
	manager := &Manager{
		clients: map[string]*Client{
			"yaml-ls": {
				name: "yaml-ls",
				config: &schema.LSPServer{
					FileTypes: []string{"yaml", "yml"},
				},
			},
			"terraform-ls": {
				name: "terraform-ls",
				config: &schema.LSPServer{
					FileTypes: []string{"tf", "tfvars"},
				},
			},
		},
		config: &schema.LSPSettings{
			Enabled: true,
		},
		rootPath: "/test/root",
		ctx:      ctx,
	}

	tests := []struct {
		name         string
		filePath     string
		expectFound  bool
		expectedName string
	}{
		{
			name:         "YAML file",
			filePath:     "/test/config.yaml",
			expectFound:  true,
			expectedName: "yaml-ls",
		},
		{
			name:         "YML file",
			filePath:     "/test/stack.yml",
			expectFound:  true,
			expectedName: "yaml-ls",
		},
		{
			name:         "Terraform file",
			filePath:     "/test/main.tf",
			expectFound:  true,
			expectedName: "terraform-ls",
		},
		{
			name:         "Terraform vars",
			filePath:     "/test/vars.tfvars",
			expectFound:  true,
			expectedName: "terraform-ls",
		},
		{
			name:        "Unsupported file",
			filePath:    "/test/script.sh",
			expectFound: false,
		},
		{
			name:        "No extension",
			filePath:    "/test/Makefile",
			expectFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, found := manager.GetClientForFile(tt.filePath)

			assert.Equal(t, tt.expectFound, found)

			if tt.expectFound {
				require.NotNil(t, client)
				assert.Equal(t, tt.expectedName, client.name)
			} else {
				assert.Nil(t, client)
			}
		})
	}
}

func TestManager_GetServerNames(t *testing.T) {
	ctx := context.Background()

	manager := &Manager{
		clients: map[string]*Client{
			"yaml-ls": {
				name: "yaml-ls",
			},
			"terraform-ls": {
				name: "terraform-ls",
			},
			"hcl-ls": {
				name: "hcl-ls",
			},
		},
		config: &schema.LSPSettings{
			Enabled: true,
		},
		rootPath: "/test/root",
		ctx:      ctx,
	}

	names := manager.GetServerNames()

	assert.Len(t, names, 3)
	assert.Contains(t, names, "yaml-ls")
	assert.Contains(t, names, "terraform-ls")
	assert.Contains(t, names, "hcl-ls")
}

func TestManager_GetAllDiagnostics(t *testing.T) {
	ctx := context.Background()

	// Create mock clients with diagnostics
	client1 := &Client{
		name: "yaml-ls",
		diagnostics: map[string][]lsp.Diagnostic{
			"file:///test/file1.yaml": {
				{
					Severity: lsp.DiagnosticSeverityError,
					Message:  "Error in file1",
				},
			},
		},
	}

	client2 := &Client{
		name: "terraform-ls",
		diagnostics: map[string][]lsp.Diagnostic{
			"file:///test/main.tf": {
				{
					Severity: lsp.DiagnosticSeverityWarning,
					Message:  "Warning in main.tf",
				},
			},
		},
	}

	manager := &Manager{
		clients: map[string]*Client{
			"yaml-ls":      client1,
			"terraform-ls": client2,
		},
		config: &schema.LSPSettings{
			Enabled: true,
		},
		rootPath: "/test/root",
		ctx:      ctx,
	}

	allDiagnostics := manager.GetAllDiagnostics()

	assert.Len(t, allDiagnostics, 2)
	assert.Contains(t, allDiagnostics, "yaml-ls")
	assert.Contains(t, allDiagnostics, "terraform-ls")

	yamlDiags := allDiagnostics["yaml-ls"]
	assert.Len(t, yamlDiags, 1)
	assert.Contains(t, yamlDiags, "file:///test/file1.yaml")

	tfDiags := allDiagnostics["terraform-ls"]
	assert.Len(t, tfDiags, 1)
	assert.Contains(t, tfDiags, "file:///test/main.tf")
}

func TestManager_GetDiagnosticsForFile(t *testing.T) {
	ctx := context.Background()

	// Create mock clients with diagnostics
	client1 := &Client{
		name: "yaml-ls",
		diagnostics: map[string][]lsp.Diagnostic{
			"file:///test/config.yaml": {
				{
					Severity: lsp.DiagnosticSeverityError,
					Message:  "Error 1",
				},
				{
					Severity: lsp.DiagnosticSeverityWarning,
					Message:  "Warning 1",
				},
			},
		},
	}

	client2 := &Client{
		name: "terraform-ls",
		diagnostics: map[string][]lsp.Diagnostic{
			"file:///test/config.yaml": {
				{
					Severity: lsp.DiagnosticSeverityError,
					Message:  "Error 2",
				},
			},
		},
	}

	manager := &Manager{
		clients: map[string]*Client{
			"yaml-ls":      client1,
			"terraform-ls": client2,
		},
		config: &schema.LSPSettings{
			Enabled: true,
		},
		rootPath: "/test/root",
		ctx:      ctx,
	}

	// Get diagnostics for file that has diagnostics from both clients
	diagnostics := manager.GetDiagnosticsForFile("/test/config.yaml")

	assert.Len(t, diagnostics, 3) // 2 from yaml-ls + 1 from terraform-ls
	assert.Equal(t, "Error 1", diagnostics[0].Message)
	assert.Equal(t, "Warning 1", diagnostics[1].Message)
	assert.Equal(t, "Error 2", diagnostics[2].Message)

	// Get diagnostics for file that doesn't exist
	emptyDiagnostics := manager.GetDiagnosticsForFile("/test/nonexistent.yaml")
	assert.Len(t, emptyDiagnostics, 0)
}

func TestManager_Close(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	config := &schema.LSPSettings{
		Enabled: false,
		Servers: map[string]*schema.LSPServer{},
	}

	manager, err := NewManager(ctx, config, "/test/root")
	require.NoError(t, err)

	err = manager.Close()
	assert.NoError(t, err)

	// Verify clients map is cleared
	assert.Empty(t, manager.clients)
}

func TestManager_AnalyzeFile_NoClientFound(t *testing.T) {
	ctx := context.Background()

	manager := &Manager{
		clients: map[string]*Client{},
		config: &schema.LSPSettings{
			Enabled: true,
		},
		rootPath: "/test/root",
		ctx:      ctx,
	}

	// Try to analyze file with no supporting client
	diagnostics, err := manager.AnalyzeFile("/test/file.unknown", "content")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no LSP server found")
	assert.Nil(t, diagnostics)
}
