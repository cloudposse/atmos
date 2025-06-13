package list

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	"github.com/cloudposse/atmos/pkg/utils"
	"github.com/samber/lo"
	"gopkg.in/yaml.v3"
)

const (
	FormatTable = "table"
	FormatJSON  = "json"
	FormatYAML  = "yaml"
	FormatCSV   = "csv"
	FormatTSV   = "tsv"
)

// ValidateFormat checks if the given format is supported.
func ValidateFormat(format string) error {
	if format == "" {
		return nil
	}
	validFormats := []string{FormatTable, FormatJSON, FormatYAML, FormatCSV, FormatTSV}
	for _, f := range validFormats {
		if format == f {
			return nil
		}
	}
	return fmt.Errorf("invalid format '%s'. Supported formats are: %s", format, strings.Join(validFormats, ", "))
}

// WorkflowData represents workflow information for template processing
type WorkflowData struct {
	File        string
	Name        string
	Description string
}

// Extracts workflows from a workflow manifest
func getWorkflowsFromManifest(manifest schema.WorkflowManifest) ([]WorkflowData, error) {
	var workflows []WorkflowData
	if manifest.Workflows == nil {
		return workflows, nil
	}
	for workflowName, workflow := range manifest.Workflows {
		workflows = append(workflows, WorkflowData{
			File:        manifest.Name,
			Name:        workflowName,
			Description: workflow.Description,
		})
	}
	return workflows, nil
}

// FilterAndListWorkflows filters and lists workflows based on the given file
func FilterAndListWorkflows(fileFlag string, listConfig schema.ListConfig, format string, delimiter string) (string, error) {
	if err := ValidateFormat(format); err != nil {
		return "", err
	}

	if format == "" && listConfig.Format != "" {
		if err := ValidateFormat(listConfig.Format); err != nil {
			return "", err
		}
		format = listConfig.Format
	}

	// Parse columns configuration - use custom columns if provided, otherwise defaults
	columns := GetColumnsWithDefaults(listConfig.Columns, "workflows")
	header := ExtractHeaders(columns)

	// Get all workflows from manifests
	var workflowDatas []WorkflowData

	// If a specific file is provided, validate and load it
	if fileFlag != "" {
		// Validate file path
		cleanPath := filepath.Clean(fileFlag)
		if !utils.IsYaml(cleanPath) {
			return "", fmt.Errorf("invalid workflow file extension: %s", fileFlag)
		}
		if _, err := os.Stat(fileFlag); os.IsNotExist(err) {
			return "", fmt.Errorf("workflow file not found: %s", fileFlag)
		}

		// Read and parse the workflow file
		data, err := os.ReadFile(fileFlag)
		if err != nil {
			return "", fmt.Errorf("error reading workflow file: %w", err)
		}

		var manifest schema.WorkflowManifest
		if err := yaml.Unmarshal(data, &manifest); err != nil {
			return "", fmt.Errorf("error parsing workflow file: %w", err)
		}

		manifestWorkflows, err := getWorkflowsFromManifest(manifest)
		if err != nil {
			return "", fmt.Errorf("error processing manifest: %w", err)
		}
		workflowDatas = append(workflowDatas, manifestWorkflows...)
	} else {
		configAndStacksInfo := schema.ConfigAndStacksInfo{}
		atmosConfig, err := config.InitCliConfig(configAndStacksInfo, true)
		if err != nil {
			return "", fmt.Errorf("error initializing CLI config: %w", err)
		}

		// Get the workflows directory
		var workflowsDir string
		if utils.IsPathAbsolute(atmosConfig.Workflows.BasePath) {
			workflowsDir = atmosConfig.Workflows.BasePath
		} else {
			workflowsDir = filepath.Join(atmosConfig.BasePath, atmosConfig.Workflows.BasePath)
		}

		isDirectory, err := utils.IsDirectory(workflowsDir)
		if err != nil || !isDirectory {
			return "", fmt.Errorf("the workflow directory '%s' does not exist. Review 'workflows.base_path' in 'atmos.yaml'", workflowsDir)
		}

		files, err := utils.GetAllYamlFilesInDir(workflowsDir)
		if err != nil {
			return "", fmt.Errorf("error reading the directory '%s' defined in 'workflows.base_path' in 'atmos.yaml': %v",
				atmosConfig.Workflows.BasePath, err)
		}

		for _, f := range files {
			var workflowPath string
			if utils.IsPathAbsolute(atmosConfig.Workflows.BasePath) {
				workflowPath = filepath.Join(atmosConfig.Workflows.BasePath, f)
			} else {
				workflowPath = filepath.Join(atmosConfig.BasePath, atmosConfig.Workflows.BasePath, f)
			}

			fileContent, err := os.ReadFile(workflowPath)
			if err != nil {
				return "", err
			}

			var manifest schema.WorkflowManifest
			if err := yaml.Unmarshal(fileContent, &manifest); err != nil {
				return "", fmt.Errorf("error parsing the workflow manifest '%s': %v", f, err)
			}

			manifestWorkflows, err := getWorkflowsFromManifest(manifest)
			if err != nil {
				return "", fmt.Errorf("error processing manifest: %w", err)
			}
			workflowDatas = append(workflowDatas, manifestWorkflows...)
		}
	}

	// Remove duplicates and sort
	workflowDatas = lo.UniqBy(workflowDatas, func(w WorkflowData) string {
		return fmt.Sprintf("%s:%s", w.File, w.Name)
	})
	sort.Slice(workflowDatas, func(i, j int) bool {
		if workflowDatas[i].File != workflowDatas[j].File {
			return workflowDatas[i].File < workflowDatas[j].File
		}
		return workflowDatas[i].Name < workflowDatas[j].Name
	})

	if len(workflowDatas) == 0 {
		return "No workflows found", nil
	}

	// Process workflows with custom columns
	var rows [][]string
	for _, workflow := range workflowDatas {
		templateData := map[string]interface{}{
			"workflow_file":        workflow.File,
			"workflow_name":        workflow.Name,
			"workflow_description": workflow.Description,
		}
		
		var row []string
		for _, col := range columns {
			value, err := ProcessColumnTemplate(col.Value, templateData)
			if err != nil {
				value = ""
			}
			row = append(row, value)
		}
		rows = append(rows, row)
	}

	// Handle different output formats
	switch format {
	case "json":
		// Convert to JSON format using custom columns
		var jsonData []map[string]interface{}
		for _, workflow := range workflowDatas {
			// Create template data
			templateData := map[string]interface{}{
				"workflow_file":        workflow.File,
				"workflow_name":        workflow.Name,
				"workflow_description": workflow.Description,
			}
			
			row, err := ProcessCustomColumns(columns, templateData)
			if err == nil {
				jsonData = append(jsonData, row)
			}
		}
		jsonBytes, err := json.MarshalIndent(jsonData, "", "  ")
		if err != nil {
			return "", fmt.Errorf("error formatting JSON output: %w", err)
		}
		return string(jsonBytes), nil

	case "csv":
		// Use the provided delimiter for CSV output
		var output strings.Builder
		output.WriteString(strings.Join(header, delimiter) + utils.GetLineEnding())
		for _, row := range rows {
			output.WriteString(strings.Join(row, delimiter) + utils.GetLineEnding())
		}
		return output.String(), nil

	default:
		// If format is empty or "table", use table format
		if format == "" && term.IsTTYSupportForStdout() {
			// Create a styled table for TTY
			t := table.New().
				Border(lipgloss.ThickBorder()).
				BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorBorder))).
				StyleFunc(func(row, col int) lipgloss.Style {
					style := lipgloss.NewStyle().PaddingLeft(1).PaddingRight(1)
					if row == 0 {
						return style.Inherit(theme.Styles.CommandName).Align(lipgloss.Center)
					}
					// Use consistent style for all rows
					return style.Inherit(theme.Styles.Description)
				}).
				Headers(header...).
				Rows(rows...)

			return t.String() + utils.GetLineEnding(), nil
		}

		// Default to simple tabular format for non-TTY or when format is explicitly "table"
		var output strings.Builder
		output.WriteString(strings.Join(header, delimiter) + utils.GetLineEnding())
		for _, row := range rows {
			output.WriteString(strings.Join(row, delimiter) + utils.GetLineEnding())
		}
		return output.String(), nil
	}
}
