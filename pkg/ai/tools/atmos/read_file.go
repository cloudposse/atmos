package atmos

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ReadFileTool reads any file from the Atmos repository.
type ReadFileTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewReadFileTool creates a new generic file reader tool.
func NewReadFileTool(atmosConfig *schema.AtmosConfiguration) *ReadFileTool {
	return &ReadFileTool{
		atmosConfig: atmosConfig,
	}
}

// Name returns the tool name.
func (t *ReadFileTool) Name() string {
	return "read_file"
}

// Description returns the tool description.
func (t *ReadFileTool) Description() string {
	return "Read any file from the Atmos repository. Use this to read atmos.yaml, workflow files, vendor manifests, documentation, or any other file. The file path should be relative to the Atmos base path or absolute."
}

// Parameters returns the tool parameters.
func (t *ReadFileTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        "file_path",
			Description: "Path to the file to read. Can be relative to Atmos base path (e.g., 'atmos.yaml', 'workflows/deploy.yaml') or absolute (e.g., '/full/path/to/file').",
			Type:        tools.ParamTypeString,
			Required:    true,
		},
	}
}

// Execute reads the file and returns its content.
func (t *ReadFileTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
	// Extract file_path parameter.
	filePath, ok := params["file_path"].(string)
	if !ok || filePath == "" {
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("%w: file_path", errUtils.ErrAIToolParameterRequired),
		}, nil
	}

	log.Debug(fmt.Sprintf("Reading file: %s", filePath))

	// Resolve to absolute path.
	absolutePath := filePath
	if !filepath.IsAbs(filePath) {
		absolutePath = filepath.Join(t.atmosConfig.BasePath, filePath)
	}

	// Clean the path to prevent traversal attacks.
	cleanPath := filepath.Clean(absolutePath)

	// Security check: ensure the path is within the Atmos base path.
	cleanBase := filepath.Clean(t.atmosConfig.BasePath)
	if !strings.HasPrefix(cleanPath, cleanBase+string(filepath.Separator)) && cleanPath != cleanBase {
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("access denied: file must be within Atmos base path"),
		}, nil
	}

	// Check if file exists.
	fileInfo, err := os.Stat(cleanPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &tools.Result{
				Success: false,
				Error:   fmt.Errorf("%w: %s", errUtils.ErrAIFileNotFound, filePath),
			}, nil
		}
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("failed to stat file %s: %w", filePath, err),
		}, nil
	}

	// Ensure it's not a directory.
	if fileInfo.IsDir() {
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("%w: %s", errUtils.ErrAIPathIsDirectory, filePath),
		}, nil
	}

	// Read file content.
	content, err := os.ReadFile(cleanPath)
	if err != nil {
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("failed to read file %s: %w", filePath, err),
		}, nil
	}

	output := fmt.Sprintf("File: %s\n\n%s", filePath, string(content))
	return &tools.Result{
		Success: true,
		Output:  output,
		Data: map[string]interface{}{
			"file_path": filePath,
			"content":   string(content),
			"size":      fileInfo.Size(),
		},
	}, nil
}

// RequiresPermission returns true if this tool needs permission.
func (t *ReadFileTool) RequiresPermission() bool {
	return false // Read-only operation, safe to execute.
}

// IsRestricted returns true if this tool is always restricted.
func (t *ReadFileTool) IsRestricted() bool {
	return false
}
