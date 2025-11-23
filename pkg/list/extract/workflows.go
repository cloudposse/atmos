package extract

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	perf "github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/utils"
)

// ExtractWorkflows transforms workflow manifests into structured data.
// Returns []map[string]any suitable for the renderer pipeline.
//
//nolint:gocognit,nestif,revive,funlen // Complexity and length from file handling and manifest parsing (unavoidable pattern).
func Workflows(atmosConfig *schema.AtmosConfiguration, fileFilter string) ([]map[string]any, error) {
	defer perf.Track(atmosConfig, "list.workflows.extract")()

	var workflows []map[string]any

	// If a specific file is provided, validate and load it.
	if fileFilter != "" {
		cleanPath := filepath.Clean(fileFilter)
		if !utils.IsYaml(cleanPath) {
			return nil, fmt.Errorf("%w: invalid workflow file extension: %s", errUtils.ErrParseFile, fileFilter)
		}

		if _, err := os.Stat(fileFilter); os.IsNotExist(err) {
			return nil, errors.Join(errUtils.ErrParseFile, fmt.Errorf("workflow file not found: %s", fileFilter))
		}

		// Read and parse the workflow file.
		data, err := os.ReadFile(fileFilter)
		if err != nil {
			return nil, errors.Join(errUtils.ErrParseFile, err)
		}

		var manifest schema.WorkflowManifest
		if err := yaml.Unmarshal(data, &manifest); err != nil {
			return nil, errors.Join(errUtils.ErrParseFile, err)
		}

		manifest.Name = fileFilter
		workflows = append(workflows, extractFromManifest(manifest)...)
		return workflows, nil
	}

	// Get the workflows directory.
	var workflowsDir string
	if utils.IsPathAbsolute(atmosConfig.Workflows.BasePath) {
		workflowsDir = atmosConfig.Workflows.BasePath
	} else {
		workflowsDir = filepath.Join(atmosConfig.BasePath, atmosConfig.Workflows.BasePath)
	}

	isDirectory, err := utils.IsDirectory(workflowsDir)
	if err != nil || !isDirectory {
		return nil, fmt.Errorf("%w: '%s'", errUtils.ErrWorkflowDirectoryDoesNotExist, workflowsDir)
	}

	files, err := utils.GetAllYamlFilesInDir(workflowsDir)
	if err != nil {
		return nil, errors.Join(errUtils.ErrWorkflowDirectoryDoesNotExist, err)
	}

	// Extract workflows from all manifests.
	for _, f := range files {
		var workflowPath string
		if utils.IsPathAbsolute(atmosConfig.Workflows.BasePath) {
			workflowPath = filepath.Join(atmosConfig.Workflows.BasePath, f)
		} else {
			workflowPath = filepath.Join(atmosConfig.BasePath, atmosConfig.Workflows.BasePath, f)
		}

		fileContent, err := os.ReadFile(workflowPath)
		if err != nil {
			continue // Skip files that can't be read.
		}

		var manifest schema.WorkflowManifest
		if err := yaml.Unmarshal(fileContent, &manifest); err != nil {
			continue // Skip invalid manifests.
		}

		manifest.Name = f
		workflows = append(workflows, extractFromManifest(manifest)...)
	}

	return workflows, nil
}

// extractFromManifest extracts workflow data from a single manifest.
func extractFromManifest(manifest schema.WorkflowManifest) []map[string]any {
	defer perf.Track(nil, "list.workflows.extractFromManifest")()

	var workflows []map[string]any

	if manifest.Workflows == nil {
		return workflows
	}

	for workflowName, workflow := range manifest.Workflows {
		w := map[string]any{
			"file":        manifest.Name,
			"workflow":    workflowName,
			"description": workflow.Description,
			// Add additional fields for advanced templates.
			"steps": len(workflow.Steps),
		}

		workflows = append(workflows, w)
	}

	return workflows
}
