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

// ReadStackFileTool reads a file from the stacks directory.
type ReadStackFileTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewReadStackFileTool creates a new stack file reader tool.
func NewReadStackFileTool(atmosConfig *schema.AtmosConfiguration) *ReadStackFileTool {
	return &ReadStackFileTool{
		atmosConfig: atmosConfig,
	}
}

// Name returns the tool name.
func (t *ReadStackFileTool) Name() string {
	return "read_stack_file"
}

// Description returns the tool description.
func (t *ReadStackFileTool) Description() string {
	return "Read a file from the stacks directory (Atmos stack configuration/settings)"
}

// Parameters returns the tool parameters.
func (t *ReadStackFileTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        "file_path",
			Description: "Relative path to the file within the stacks directory (e.g., 'catalog/vpc.yaml' or 'deploy/prod/us-east-1.yaml')",
			Type:        tools.ParamTypeString,
			Required:    true,
		},
	}
}

// Execute reads the stack file and returns its content.
func (t *ReadStackFileTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
	filePath, err := extractFilePathParam(params)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	log.Debug(fmt.Sprintf("Reading stack file: %s", filePath))

	cleanPath, err := t.resolveAndValidateStackPath(filePath)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	content, err := readAndValidateFile(cleanPath, filePath)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	output := fmt.Sprintf("Stack file: %s\n\n%s", filePath, string(content))
	return &tools.Result{
		Success: true,
		Output:  output,
		Data: map[string]interface{}{
			"file_path": filePath,
			"content":   string(content),
		},
	}, nil
}

// extractFilePathParam extracts and validates the file_path parameter.
func extractFilePathParam(params map[string]interface{}) (string, error) {
	filePath, ok := params["file_path"].(string)
	if !ok || filePath == "" {
		return "", fmt.Errorf("%w: file_path", errUtils.ErrAIToolParameterRequired)
	}
	return filePath, nil
}

// resolveAndValidateStackPath resolves the stack path and validates security.
func (t *ReadStackFileTool) resolveAndValidateStackPath(filePath string) (string, error) {
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
func (t *ReadStackFileTool) RequiresPermission() bool {
	return false // Read-only operation, safe to execute
}

// IsRestricted returns true if this tool is always restricted.
func (t *ReadStackFileTool) IsRestricted() bool {
	return false
}
