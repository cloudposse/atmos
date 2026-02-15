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

	log.Debug(fmt.Sprintf("Listing files in component: %s/%s with pattern '%s'", componentType, componentPath, filePattern))

	// Get component base path.
	basePath, err := t.getComponentBasePath(componentType)
	if err != nil {
		//nolint:nilerr // Tool errors are returned in Result.Error, not in err return value.
		return &tools.Result{
			Success: false,
			Error:   err,
		}, nil
	}

	// Resolve to absolute path.
	absoluteBasePath := basePath
	if !filepath.IsAbs(basePath) {
		absoluteBasePath = filepath.Join(t.atmosConfig.BasePath, basePath)
	}

	fullPath := filepath.Join(absoluteBasePath, componentPath)
	cleanPath := filepath.Clean(fullPath)

	// Security check.
	cleanBase := filepath.Clean(absoluteBasePath)
	if !strings.HasPrefix(cleanPath, cleanBase+string(filepath.Separator)) && cleanPath != cleanBase {
		return &tools.Result{
			Success: false,
			Error:   errUtils.ErrAIFileAccessDeniedComponents,
		}, nil
	}

	// Check if path exists.
	if _, err := os.Stat(cleanPath); err != nil {
		if os.IsNotExist(err) {
			return &tools.Result{
				Success: false,
				Error:   fmt.Errorf("%w: %s/%s", errUtils.ErrAIFileNotFound, componentType, componentPath),
			}, nil
		}
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("failed to access path: %w", err),
		}, nil
	}

	// List files.
	var files []string
	fileCount := 0
	dirCount := 0

	err = filepath.Walk(cleanPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil //nolint:nilerr // Skip files with errors and continue walking.
		}

		// Skip the root directory itself.
		if path == cleanPath {
			return nil
		}

		// Get relative path from component path.
		relPath, _ := filepath.Rel(cleanPath, path)

		if info.IsDir() {
			dirCount++
			files = append(files, relPath+"/")
		} else {
			// Check file pattern.
			if filePattern != "*" {
				matched, _ := filepath.Match(filePattern, filepath.Base(path))
				if !matched {
					return nil
				}
			}
			fileCount++
			files = append(files, relPath)
		}

		return nil
	})
	if err != nil {
		//nolint:nilerr // Tool errors are returned in Result.Error, not in err return value.
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("failed to list files: %w", err),
		}, nil
	}

	// Format output.
	var output string
	if len(files) == 0 {
		output = fmt.Sprintf("No files found in %s/%s", componentType, componentPath)
	} else {
		header := fmt.Sprintf("Files in %s/%s", componentType, componentPath)
		if filePattern != "*" {
			header += fmt.Sprintf(" (pattern: %s)", filePattern)
		}
		header += fmt.Sprintf(":\n(%d files, %d directories)\n\n", fileCount, dirCount)
		output = header + strings.Join(files, "\n")
	}

	return &tools.Result{
		Success: true,
		Output:  output,
		Data: map[string]interface{}{
			"component_type": componentType,
			"component_path": componentPath,
			"files":          files,
			"file_count":     fileCount,
			"dir_count":      dirCount,
		},
	}, nil
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

// RequiresPermission returns true if this tool needs permission.
func (t *ListComponentFilesTool) RequiresPermission() bool {
	return false // Read-only operation, safe to execute.
}

// IsRestricted returns true if this tool is always restricted.
func (t *ListComponentFilesTool) IsRestricted() bool {
	return false
}
