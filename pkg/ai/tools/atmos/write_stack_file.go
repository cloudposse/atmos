package atmos

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

// WriteStackFileTool writes content to a file in the stacks directory.
type WriteStackFileTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewWriteStackFileTool creates a new stack file writer tool.
func NewWriteStackFileTool(atmosConfig *schema.AtmosConfiguration) *WriteStackFileTool {
	return &WriteStackFileTool{
		atmosConfig: atmosConfig,
	}
}

// Name returns the tool name.
func (t *WriteStackFileTool) Name() string {
	return "write_stack_file"
}

// Description returns the tool description.
func (t *WriteStackFileTool) Description() string {
	return "Write or modify a file in the stacks directory (Atmos stack configuration/settings). Requires user confirmation."
}

// Parameters returns the tool parameters.
func (t *WriteStackFileTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        "file_path",
			Description: "Relative path to the file within the stacks directory (e.g., 'catalog/vpc.yaml' or 'deploy/prod/us-east-1.yaml')",
			Type:        tools.ParamTypeString,
			Required:    true,
		},
		{
			Name:        "content",
			Description: "Content to write to the file",
			Type:        tools.ParamTypeString,
			Required:    true,
		},
	}
}

// Execute writes content to the stack file.
func (t *WriteStackFileTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
	filePath, err := extractFilePathParam(params)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	content, ok := params["content"].(string)
	if !ok {
		err := fmt.Errorf("%w: content", errUtils.ErrAIToolParameterRequired)
		return &tools.Result{Success: false, Error: err}, err
	}

	log.Debug(fmt.Sprintf("Writing stack file: %s", filePath))

	cleanPath, err := t.resolveAndValidateStackWritePath(filePath)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	if err := writeFileWithDirs(cleanPath, content); err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	output := fmt.Sprintf("Successfully wrote stack file: %s (%d bytes)", filePath, len(content))
	return &tools.Result{
		Success: true,
		Output:  output,
		Data: map[string]interface{}{
			"file_path":     filePath,
			"bytes_written": len(content),
		},
	}, nil
}

// resolveAndValidateStackWritePath resolves and validates the stack path for writing.
func (t *WriteStackFileTool) resolveAndValidateStackWritePath(filePath string) (string, error) {
	basePath := t.atmosConfig.Stacks.BasePath

	absoluteBasePath := basePath
	if !filepath.IsAbs(basePath) {
		absoluteBasePath = filepath.Join(t.atmosConfig.BasePath, basePath)
	}

	fullPath := filepath.Join(absoluteBasePath, filePath)
	cleanPath := filepath.Clean(fullPath)
	cleanBase := filepath.Clean(absoluteBasePath)

	if !strings.HasPrefix(cleanPath, cleanBase+string(filepath.Separator)) && cleanPath != cleanBase {
		return "", errUtils.ErrAIFileAccessDeniedStacks
	}

	return cleanPath, nil
}

// RequiresPermission returns true if this tool needs permission.
func (t *WriteStackFileTool) RequiresPermission() bool {
	return true // Writing files requires confirmation
}

// IsRestricted returns true if this tool is always restricted.
func (t *WriteStackFileTool) IsRestricted() bool {
	return false // User can allow via configuration
}
