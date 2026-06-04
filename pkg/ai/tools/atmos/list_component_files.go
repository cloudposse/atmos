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

// ListComponentFilesTool lists files in a component directory.
type ListComponentFilesTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewListComponentFilesTool creates a new component file listing tool.
func NewListComponentFilesTool(atmosConfig *schema.AtmosConfiguration) *ListComponentFilesTool {
	return &ListComponentFilesTool{
		atmosConfig: atmosConfig,
	}
}

// Name returns the tool name.
func (t *ListComponentFilesTool) Name() string {
	return "list_component_files"
}

// Description returns the tool description.
func (t *ListComponentFilesTool) Description() string {
	return "List all files in a component directory. Use this to discover what files exist in a component, understand component structure, or find configuration files. Returns file paths relative to the component root."
}

// Parameters returns the tool parameters.
func (t *ListComponentFilesTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        "component_type",
			Description: "Type of component: 'terraform', 'helmfile', or 'packer'",
			Type:        tools.ParamTypeString,
			Required:    true,
		},
		{
			Name:        "component_path",
			Description: "Path to the component within the component type directory (e.g., 'vpc', 'eks', 'networking/vpc'). Use '.' to list all components.",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
		{
			Name:        "file_pattern",
			Description: "File pattern to filter (e.g., '*.tf', '*.yaml', '*.hcl'). Default is '*' (all files).",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
	}
}

// Execute lists files in the component directory.
func (t *ListComponentFilesTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
	// Extract component_type parameter.
	componentType, ok := params["component_type"].(string)
	if !ok || componentType == "" {
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("%w: component_type", errUtils.ErrAIToolParameterRequired),
		}, nil
	}

	// Extract optional component_path parameter.
	componentPath := "."
	if cp, ok := params["component_path"].(string); ok && cp != "" {
		componentPath = cp
	}

	// Extract optional file_pattern parameter.
	filePattern := "*"
	if fp, ok := params["file_pattern"].(string); ok && fp != "" {
		filePattern = fp
	}

	log.Debugf("Listing files in component: %s/%s with pattern '%s'", componentType, componentPath, filePattern)

	cleanPath, errResult := t.resolveComponentPath(componentType, componentPath)
	if errResult != nil {
		return errResult, nil
	}

	// Walk and collect files.
	walkResult, err := walkComponentFiles(cleanPath, filePattern)
	if err != nil {
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("failed to list files: %w", err),
		}, nil
	}

	return &tools.Result{
		Success: true,
		Output:  walkResult.format(componentType, componentPath, filePattern),
		Data: map[string]interface{}{
			"component_type": componentType,
			"component_path": componentPath,
			"files":          walkResult.files,
			"file_count":     walkResult.fileCount,
			"dir_count":      walkResult.dirCount,
		},
	}, nil
}

// componentFileResult holds the results of walking a component directory.
type componentFileResult struct {
	files     []string
	fileCount int
	dirCount  int
}

// walkComponentFiles walks the component directory and collects matching files.
func walkComponentFiles(rootPath, filePattern string) (*componentFileResult, error) {
	result := &componentFileResult{}

	err := filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || path == rootPath {
			return nil //nolint:nilerr // Skip files with errors and continue walking.
		}

		relPath, _ := filepath.Rel(rootPath, path)

		if info.IsDir() {
			result.dirCount++
			result.files = append(result.files, relPath+"/")
			return nil
		}

		if filePattern != "*" {
			matched, _ := filepath.Match(filePattern, filepath.Base(path))
			if !matched {
				return nil
			}
		}
		result.fileCount++
		result.files = append(result.files, relPath)

		return nil
	})

	return result, err
}

// formatComponentFileList formats the list of component files for output.
func (r *componentFileResult) format(componentType, componentPath, filePattern string) string {
	if len(r.files) == 0 {
		return fmt.Sprintf("No files found in %s/%s", componentType, componentPath)
	}

	header := fmt.Sprintf("Files in %s/%s", componentType, componentPath)
	if filePattern != "*" {
		header += fmt.Sprintf(" (pattern: %s)", filePattern)
	}
	header += fmt.Sprintf(":\n(%d files, %d directories)\n\n", r.fileCount, r.dirCount)
	return header + strings.Join(r.files, "\n")
}

// getComponentBasePath returns the base path for a component type.
func (t *ListComponentFilesTool) getComponentBasePath(componentType string) (string, error) {
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

// resolveComponentPath resolves and validates the component path.
func (t *ListComponentFilesTool) resolveComponentPath(componentType, componentPath string) (string, *tools.Result) {
	basePath, err := t.getComponentBasePath(componentType)
	if err != nil {
		return "", &tools.Result{
			Success: false,
			Error:   err,
		}
	}

	fullPath := filepath.Join(t.atmosConfig.BasePath, basePath, componentPath)
	cleanPath := filepath.Clean(fullPath)

	// Verify the path exists.
	info, err := os.Stat(cleanPath)
	if err != nil {
		return "", &tools.Result{
			Success: false,
			Error:   fmt.Errorf("%w: %s/%s", errUtils.ErrAIComponentPathNotFound, componentType, componentPath),
		}
	}

	if !info.IsDir() {
		return "", &tools.Result{
			Success: false,
			Error:   fmt.Errorf("%w: %s/%s", errUtils.ErrAIComponentPathNotDirectory, componentType, componentPath),
		}
	}

	return cleanPath, nil
}

// RequiresPermission returns true if this tool needs permission.
func (t *ListComponentFilesTool) RequiresPermission() bool {
	return false // Read-only operation, safe to execute.
}

// IsRestricted returns true if this tool is always restricted.
func (t *ListComponentFilesTool) IsRestricted() bool {
	return false
}
