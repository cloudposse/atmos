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

// ReadComponentFileTool reads a file from the components directory.
type ReadComponentFileTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewReadComponentFileTool creates a new component file reader tool.
func NewReadComponentFileTool(atmosConfig *schema.AtmosConfiguration) *ReadComponentFileTool {
	return &ReadComponentFileTool{
		atmosConfig: atmosConfig,
	}
}

// Name returns the tool name.
func (t *ReadComponentFileTool) Name() string {
	return "read_component_file"
}

// Description returns the tool description.
func (t *ReadComponentFileTool) Description() string {
	return "Read a file from the components directory (Terraform, Helmfile, or Packer code/logic)"
}

// Parameters returns the tool parameters.
func (t *ReadComponentFileTool) Parameters() []tools.Parameter {
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
	}
}

// Execute reads the component file and returns its content.
func (t *ReadComponentFileTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
	componentType, filePath, err := extractComponentParams(params)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	log.Debug(fmt.Sprintf("Reading component file: %s/%s", componentType, filePath))

	cleanPath, err := t.resolveAndValidateComponentPath(componentType, filePath)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	content, err := readAndValidateFile(cleanPath, filePath)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	output := fmt.Sprintf("Component file: %s/%s\n\n%s", componentType, filePath, string(content))
	return &tools.Result{
		Success: true,
		Output:  output,
		Data: map[string]interface{}{
			"component_type": componentType,
			"file_path":      filePath,
			"content":        string(content),
		},
	}, nil
}

// extractComponentParams extracts and validates component_type and file_path parameters.
func extractComponentParams(params map[string]interface{}) (componentType, filePath string, err error) {
	componentType, ok := params["component_type"].(string)
	if !ok || componentType == "" {
		return "", "", fmt.Errorf("%w: component_type", errUtils.ErrAIToolParameterRequired)
	}

	filePath, ok = params["file_path"].(string)
	if !ok || filePath == "" {
		return "", "", fmt.Errorf("%w: file_path", errUtils.ErrAIToolParameterRequired)
	}

	return componentType, filePath, nil
}

// resolveAndValidateComponentPath resolves the component path and validates security.
func (t *ReadComponentFileTool) resolveAndValidateComponentPath(componentType, filePath string) (string, error) {
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
func (t *ReadComponentFileTool) getComponentBasePath(componentType string) (string, error) {
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

// readAndValidateFile reads a file after validating it exists and is not a directory.
func readAndValidateFile(cleanPath, filePath string) ([]byte, error) {
	fileInfo, err := os.Stat(cleanPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", errUtils.ErrAIFileNotFound, filePath)
		}
		return nil, err
	}

	if fileInfo.IsDir() {
		return nil, fmt.Errorf("%w: %s", errUtils.ErrAIPathIsDirectory, filePath)
	}

	return os.ReadFile(cleanPath)
}

// RequiresPermission returns true if this tool needs permission.
func (t *ReadComponentFileTool) RequiresPermission() bool {
	return false // Read-only operation, safe to execute
}

// IsRestricted returns true if this tool is always restricted.
func (t *ReadComponentFileTool) IsRestricted() bool {
	return false
}
