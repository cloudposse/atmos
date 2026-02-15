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

// WriteComponentFileTool writes content to a file in the components directory.
type WriteComponentFileTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewWriteComponentFileTool creates a new component file writer tool.
func NewWriteComponentFileTool(atmosConfig *schema.AtmosConfiguration) *WriteComponentFileTool {
	return &WriteComponentFileTool{
		atmosConfig: atmosConfig,
	}
}

// Name returns the tool name.
func (t *WriteComponentFileTool) Name() string {
	return "write_component_file"
}

// Description returns the tool description.
func (t *WriteComponentFileTool) Description() string {
	return "Write or modify a file in the components directory (Terraform, Helmfile, or Packer code/logic). Requires user confirmation."
}

// Parameters returns the tool parameters.
func (t *WriteComponentFileTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        "component_type",
			Description: "Type of component: 'terraform', 'helmfile', or 'packer'",
			Type:        tools.ParamTypeString,
			Required:    true,
		},
		{
			Name:        "file_path",
			Description: "Relative path to the file within the component type directory (e.g., 'vpc/main.tf')",
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

// Execute writes content to the component file.
func (t *WriteComponentFileTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
	componentType, filePath, err := extractComponentParams(params)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	content, ok := params["content"].(string)
	if !ok {
		err := fmt.Errorf("%w: content", errUtils.ErrAIToolParameterRequired)
		return &tools.Result{Success: false, Error: err}, err
	}

	log.Debug(fmt.Sprintf("Writing component file: %s/%s", componentType, filePath))

	cleanPath, err := t.resolveAndValidateComponentWritePath(componentType, filePath)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	if err := writeFileWithDirs(cleanPath, content); err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	output := fmt.Sprintf("Successfully wrote component file: %s/%s (%d bytes)", componentType, filePath, len(content))
	return &tools.Result{
		Success: true,
		Output:  output,
		Data: map[string]interface{}{
			"component_type": componentType,
			"file_path":      filePath,
			"bytes_written":  len(content),
		},
	}, nil
}

// resolveAndValidateComponentWritePath resolves and validates the component path for writing.
func (t *WriteComponentFileTool) resolveAndValidateComponentWritePath(componentType, filePath string) (string, error) {
	basePath, err := t.getComponentBasePath(componentType)
	if err != nil {
		return "", err
	}

	absoluteBasePath := basePath
	if !filepath.IsAbs(basePath) {
		absoluteBasePath = filepath.Join(t.atmosConfig.BasePath, basePath)
	}

	fullPath := filepath.Join(absoluteBasePath, filePath)
	cleanPath := filepath.Clean(fullPath)
	cleanBase := filepath.Clean(absoluteBasePath)

	if !strings.HasPrefix(cleanPath, cleanBase+string(filepath.Separator)) && cleanPath != cleanBase {
		return "", errUtils.ErrAIFileAccessDeniedComponents
	}

	return cleanPath, nil
}

// getComponentBasePath returns the base path for a component type.
func (t *WriteComponentFileTool) getComponentBasePath(componentType string) (string, error) {
	switch componentType {
	case "terraform":
		return t.atmosConfig.Components.Terraform.BasePath, nil
	case "helmfile":
		return t.atmosConfig.Components.Helmfile.BasePath, nil
	case "packer":
		return t.atmosConfig.Components.Packer.BasePath, nil
	default:
		return "", fmt.Errorf("%w: %s (must be terraform, helmfile, or packer)", errUtils.ErrAIUnsupportedComponentType, componentType)
	}
}

// writeFileWithDirs writes a file, creating parent directories if needed.
func writeFileWithDirs(cleanPath, content string) error {
	dir := filepath.Dir(cleanPath)
	if err := os.MkdirAll(dir, dirPermissions); err != nil {
		return fmt.Errorf("failed to create parent directories: %w", err)
	}

	if err := os.WriteFile(cleanPath, []byte(content), filePermissions); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// RequiresPermission returns true if this tool needs permission.
func (t *WriteComponentFileTool) RequiresPermission() bool {
	return true // Writing files requires confirmation
}

// IsRestricted returns true if this tool is always restricted.
func (t *WriteComponentFileTool) IsRestricted() bool {
	return false // User can allow via configuration
}
