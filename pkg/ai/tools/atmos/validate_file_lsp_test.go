package atmos

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/lsp"
	"github.com/cloudposse/atmos/pkg/lsp/client"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestValidateFileLSPTool_Name(t *testing.T) {
	tool := NewValidateFileLSPTool(nil, nil)
	assert.Equal(t, "validate_file_lsp", tool.Name())
}

func TestValidateFileLSPTool_Description(t *testing.T) {
	tool := NewValidateFileLSPTool(nil, nil)
	description := tool.Description()
	assert.Contains(t, description, "Language Server Protocol")
	assert.Contains(t, description, "Validate")
}

func TestValidateFileLSPTool_Parameters(t *testing.T) {
	tool := NewValidateFileLSPTool(nil, nil)
	params := tool.Parameters()

	require.Len(t, params, 1)
	assert.Equal(t, "file_path", params[0].Name)
	assert.Equal(t, tools.ParamTypeString, params[0].Type)
	assert.True(t, params[0].Required)
}

func TestValidateFileLSPTool_RequiresPermission(t *testing.T) {
	tool := NewValidateFileLSPTool(nil, nil)
	assert.False(t, tool.RequiresPermission())
}

func TestValidateFileLSPTool_IsRestricted(t *testing.T) {
	tool := NewValidateFileLSPTool(nil, nil)
	assert.False(t, tool.IsRestricted())
}

func TestValidateFileLSPTool_Execute_LSPDisabled(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: "/test",
	}

	// LSP manager is nil (not initialized)
	tool := NewValidateFileLSPTool(atmosConfig, nil)

	params := map[string]interface{}{
		"file_path": "test.yaml",
	}

	result, err := tool.Execute(context.Background(), params)

	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error.Error(), "LSP is not enabled")
}

func TestValidateFileLSPTool_Execute_MissingFilePath(t *testing.T) {
	ctx := context.Background()
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: "/test",
	}

	lspConfig := &schema.LSPSettings{
		Enabled: true,
		Servers: map[string]*schema.LSPServer{},
	}

	lspManager, err := client.NewManager(ctx, lspConfig, "/test")
	require.NoError(t, err)
	defer lspManager.Close()

	tool := NewValidateFileLSPTool(atmosConfig, lspManager)

	tests := []struct {
		name   string
		params map[string]interface{}
	}{
		{
			name:   "Missing file_path parameter",
			params: map[string]interface{}{},
		},
		{
			name: "Empty file_path",
			params: map[string]interface{}{
				"file_path": "",
			},
		},
		{
			name: "Non-string file_path",
			params: map[string]interface{}{
				"file_path": 123,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tool.Execute(context.Background(), tt.params)

			require.NoError(t, err)
			assert.False(t, result.Success)
			assert.Contains(t, result.Error.Error(), "file_path parameter is required")
		})
	}
}

func TestValidateFileLSPTool_Execute_FileNotFound(t *testing.T) {
	ctx := context.Background()
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	lspConfig := &schema.LSPSettings{
		Enabled: true,
		Servers: map[string]*schema.LSPServer{},
	}

	lspManager, err := client.NewManager(ctx, lspConfig, atmosConfig.BasePath)
	require.NoError(t, err)
	defer lspManager.Close()

	tool := NewValidateFileLSPTool(atmosConfig, lspManager)

	params := map[string]interface{}{
		"file_path": "nonexistent.yaml",
	}

	result, err := tool.Execute(context.Background(), params)

	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error.Error(), "file does not exist")
}

func TestValidateFileLSPTool_Execute_Success_NoLSPServer(t *testing.T) {
	ctx := context.Background()

	// Create temp directory and file
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.yaml")
	err := os.WriteFile(testFile, []byte("key: value\n"), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
	}

	// LSP manager with no servers configured
	lspConfig := &schema.LSPSettings{
		Enabled: true,
		Servers: map[string]*schema.LSPServer{},
	}

	lspManager, err := client.NewManager(ctx, lspConfig, tempDir)
	require.NoError(t, err)
	defer lspManager.Close()

	tool := NewValidateFileLSPTool(atmosConfig, lspManager)

	params := map[string]interface{}{
		"file_path": "test.yaml",
	}

	result, err := tool.Execute(context.Background(), params)

	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error.Error(), "no LSP server found")
}

func TestValidateFileLSPTool_Execute_AbsolutePath(t *testing.T) {
	ctx := context.Background()

	// Create temp directory and file
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.yaml")
	err := os.WriteFile(testFile, []byte("key: value\n"), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
	}

	// LSP manager with no servers (will fail to find server)
	lspConfig := &schema.LSPSettings{
		Enabled: true,
		Servers: map[string]*schema.LSPServer{},
	}

	lspManager, err := client.NewManager(ctx, lspConfig, tempDir)
	require.NoError(t, err)
	defer lspManager.Close()

	tool := NewValidateFileLSPTool(atmosConfig, lspManager)

	// Test with absolute path
	params := map[string]interface{}{
		"file_path": testFile,
	}

	result, err := tool.Execute(context.Background(), params)

	require.NoError(t, err)
	assert.False(t, result.Success)
	// Should fail because no LSP server configured, but file was found and read
	assert.Contains(t, result.Error.Error(), "no LSP server found")
}

func TestValidateFileLSPTool_Execute_RelativePath(t *testing.T) {
	ctx := context.Background()

	// Create temp directory and file
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.yaml")
	err := os.WriteFile(testFile, []byte("key: value\n"), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
	}

	// LSP manager with no servers (will fail to find server)
	lspConfig := &schema.LSPSettings{
		Enabled: true,
		Servers: map[string]*schema.LSPServer{},
	}

	lspManager, err := client.NewManager(ctx, lspConfig, tempDir)
	require.NoError(t, err)
	defer lspManager.Close()

	tool := NewValidateFileLSPTool(atmosConfig, lspManager)

	// Test with relative path
	params := map[string]interface{}{
		"file_path": "test.yaml",
	}

	result, err := tool.Execute(context.Background(), params)

	require.NoError(t, err)
	assert.False(t, result.Success)
	// Should fail because no LSP server configured, but file was found and read
	assert.Contains(t, result.Error.Error(), "no LSP server found")
}

// mockLSPManager implements client.ManagerInterface for testing.
type mockLSPManager struct {
	enabled      bool
	diagnostics  []lsp.Diagnostic
	analyzeError error
}

func (m *mockLSPManager) GetClient(name string) (*client.Client, bool) {
	return nil, false
}

func (m *mockLSPManager) GetClientForFile(filePath string) (*client.Client, bool) {
	return nil, m.enabled
}

func (m *mockLSPManager) AnalyzeFile(filePath, content string) ([]lsp.Diagnostic, error) {
	if m.analyzeError != nil {
		return nil, m.analyzeError
	}
	return m.diagnostics, nil
}

func (m *mockLSPManager) GetAllDiagnostics() map[string]map[string][]lsp.Diagnostic {
	return make(map[string]map[string][]lsp.Diagnostic)
}

func (m *mockLSPManager) GetDiagnosticsForFile(filePath string) []lsp.Diagnostic {
	return m.diagnostics
}

func (m *mockLSPManager) Close() error {
	return nil
}

func (m *mockLSPManager) IsEnabled() bool {
	return m.enabled
}

func (m *mockLSPManager) GetServerNames() []string {
	return []string{}
}

func TestValidateFileLSPTool_Execute_WithDiagnostics(t *testing.T) {
	// Create temp directory and file
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.yaml")
	err := os.WriteFile(testFile, []byte("key: value\n"), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
	}

	mockManager := &mockLSPManager{
		enabled: true,
		diagnostics: []lsp.Diagnostic{
			{
				Range: lsp.Range{
					Start: lsp.Position{Line: 0, Character: 0},
				},
				Severity: lsp.DiagnosticSeverityError,
				Message:  "Test error message",
			},
			{
				Range: lsp.Range{
					Start: lsp.Position{Line: 1, Character: 0},
				},
				Severity: lsp.DiagnosticSeverityWarning,
				Message:  "Test warning message",
			},
		},
	}

	// Create tool with mock manager
	tool := &ValidateFileLSPTool{
		atmosConfig: atmosConfig,
		lspManager:  mockManager,
	}

	params := map[string]interface{}{
		"file_path": "test.yaml",
	}

	result, err := tool.Execute(context.Background(), params)

	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Contains(t, result.Output, "Found 2 issue(s)")
	assert.Contains(t, result.Output, "Test error message")
	assert.Contains(t, result.Output, "Test warning message")

	// Check metadata
	assert.Equal(t, 2, result.Data["diagnostics_count"])
	assert.True(t, result.Data["has_errors"].(bool))
	assert.True(t, result.Data["has_warnings"].(bool))
}

func TestValidateFileLSPTool_Execute_NoDiagnostics(t *testing.T) {
	// Create temp directory and file
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.yaml")
	err := os.WriteFile(testFile, []byte("key: value\n"), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
	}

	mockManager := &mockLSPManager{
		enabled:     true,
		diagnostics: []lsp.Diagnostic{},
	}

	tool := &ValidateFileLSPTool{
		atmosConfig: atmosConfig,
		lspManager:  mockManager,
	}

	params := map[string]interface{}{
		"file_path": "test.yaml",
	}

	result, err := tool.Execute(context.Background(), params)

	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Contains(t, result.Output, "No issues found")

	// Check metadata
	assert.Equal(t, 0, result.Data["diagnostics_count"])
	assert.False(t, result.Data["has_errors"].(bool))
	assert.False(t, result.Data["has_warnings"].(bool))
}
