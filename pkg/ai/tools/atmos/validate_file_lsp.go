package atmos

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/lsp/client"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ValidateFileLSPTool provides LSP-based file validation.
type ValidateFileLSPTool struct {
	atmosConfig *schema.AtmosConfiguration
	lspManager  client.ManagerInterface
}

// NewValidateFileLSPTool creates a new LSP validation tool.
func NewValidateFileLSPTool(atmosConfig *schema.AtmosConfiguration, lspManager client.ManagerInterface) *ValidateFileLSPTool {
	return &ValidateFileLSPTool{
		atmosConfig: atmosConfig,
		lspManager:  lspManager,
	}
}

// Name returns the tool name.
func (t *ValidateFileLSPTool) Name() string {
	return "validate_file_lsp"
}

// Description returns the tool description.
func (t *ValidateFileLSPTool) Description() string {
	return "Validate a YAML, Terraform, or HCL file using Language Server Protocol for real-time diagnostics and error detection. Returns detailed errors, warnings, and suggestions with line numbers."
}

// Parameters returns the tool parameters.
func (t *ValidateFileLSPTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        "file_path",
			Description: "Absolute or relative path to the file to validate (e.g., 'stacks/prod/vpc.yaml', 'components/terraform/vpc/main.tf')",
			Type:        tools.ParamTypeString,
			Required:    true,
		},
	}
}

// Execute validates a file using LSP and returns diagnostics.
func (t *ValidateFileLSPTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
	// Check if LSP is enabled
	if t.lspManager == nil || !t.lspManager.IsEnabled() {
		return &tools.Result{
			Success: false,
			Output:  "",
			Error:   fmt.Errorf("LSP is not enabled - configure settings.lsp in atmos.yaml to use this tool"),
		}, nil
	}

	// Extract parameters
	filePath, ok := params["file_path"].(string)
	if !ok || filePath == "" {
		return &tools.Result{
			Success: false,
			Output:  "",
			Error:   fmt.Errorf("file_path parameter is required and must be a string"),
		}, nil
	}

	// Resolve to absolute path
	absPath := filePath
	if !filepath.IsAbs(filePath) {
		absPath = filepath.Join(t.atmosConfig.BasePath, filePath)
	}

	// Check if file exists
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return &tools.Result{
			Success: false,
			Output:  "",
			Error:   fmt.Errorf("file does not exist: %s", filePath),
		}, nil
	}

	// Read file content
	content, err := os.ReadFile(absPath)
	if err != nil {
		return &tools.Result{
			Success: false,
			Output:  "",
			Error:   fmt.Errorf("failed to read file %s: %w", filePath, err),
		}, nil
	}

	// Analyze file with LSP
	diagnostics, err := t.lspManager.AnalyzeFile(absPath, string(content))
	if err != nil {
		return &tools.Result{
			Success: false,
			Output:  "",
			Error:   fmt.Errorf("LSP validation failed for %s: %w", filePath, err),
		}, nil
	}

	// Format diagnostics for AI
	formatter := client.NewDiagnosticFormatter()
	uri := "file://" + absPath

	output := formatter.FormatForAI(uri, diagnostics)

	// Add context about LSP server used
	_, found := t.lspManager.GetClientForFile(absPath)
	if found {
		ext := filepath.Ext(absPath)
		output = fmt.Sprintf("Validation via LSP (%s file):\n\n%s", ext, output)
	}

	return &tools.Result{
		Success: true,
		Output:  output,
		Data: map[string]interface{}{
			"diagnostics_count": len(diagnostics),
			"has_errors":        client.HasErrors(diagnostics),
			"has_warnings":      client.HasWarnings(diagnostics),
		},
	}, nil
}

// RequiresPermission returns whether this tool requires user permission.
func (t *ValidateFileLSPTool) RequiresPermission() bool {
	return false // Read-only operation, safe to execute without confirmation
}

// IsRestricted returns whether this tool is always restricted.
func (t *ValidateFileLSPTool) IsRestricted() bool {
	return false // Read-only validation is not restricted
}
