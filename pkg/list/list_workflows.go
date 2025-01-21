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
	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	"github.com/cloudposse/atmos/pkg/utils"
	"github.com/samber/lo"

	"github.com/cloudposse/atmos/pkg/schema"
	"gopkg.in/yaml.v3"
)

const (
	FormatTable = "table"
	FormatJSON  = "json"
	FormatCSV   = "csv"
)

// ValidateFormat checks if the given format is supported
func ValidateFormat(format string) error {
	if format == "" {
		return nil
	}
	validFormats := []string{FormatTable, FormatJSON, FormatCSV}
	for _, f := range validFormats {
		if format == f {
			return nil
		}
	}
	return fmt.Errorf("invalid format '%s'. Supported formats are: %s", format, strings.Join(validFormats, ", "))
}

// Extracts workflows from a workflow manifest
func getWorkflowsFromManifest(manifest schema.WorkflowManifest) ([][]string, error) {
	var rows [][]string
	if manifest.Workflows == nil {
		return rows, nil
	}
	for workflowName, workflow := range manifest.Workflows {
		rows = append(rows, []string{
			manifest.Name,
			workflowName,
			workflow.Description,
		})
	}
	return rows, nil
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

	// Parse columns configuration
	header := []string{"File", "Workflow", "Description"}

	// Get all workflows from manifests
	var rows [][]string

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

		manifestRows, err := getWorkflowsFromManifest(manifest)
		if err != nil {
			return "", fmt.Errorf("error processing manifest: %w", err)
		}
		rows = append(rows, manifestRows...)
	} else {
		// Use example data for empty fileFlag
		manifest := schema.WorkflowManifest{
			Name: "example",
			Workflows: schema.WorkflowConfig{
				"test-1": schema.WorkflowDefinition{
					Description: "Test workflow",
					Steps: []schema.WorkflowStep{
						{Command: "echo Command 1", Name: "step1", Type: "shell"},
						{Command: "echo Command 2", Name: "step2", Type: "shell"},
						{Command: "echo Command 3", Name: "step3", Type: "shell"},
						{Command: "echo Command 4", Type: "shell"},
					},
				},
			},
		}

		manifestRows, err := getWorkflowsFromManifest(manifest)
		if err != nil {
			return "", fmt.Errorf("error processing manifest: %w", err)
		}
		rows = append(rows, manifestRows...)
	}

	// Remove duplicates and sort
	rows = lo.UniqBy(rows, func(row []string) string {
		return strings.Join(row, delimiter)
	})
	sort.Slice(rows, func(i, j int) bool {
		return strings.Join(rows[i], delimiter) < strings.Join(rows[j], delimiter)
	})

	if len(rows) == 0 {
		return "No workflows found", nil
	}

	// Handle different output formats
	switch format {
	case "json":
		// Convert to JSON format
		type workflow struct {
			File        string `json:"file"`
			Name        string `json:"name"`
			Description string `json:"description"`
		}
		var workflows []workflow
		for _, row := range rows {
			workflows = append(workflows, workflow{
				File:        row[0],
				Name:        row[1],
				Description: row[2],
			})
		}
		jsonBytes, err := json.MarshalIndent(workflows, "", "  ")
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
		if format == "" && exec.CheckTTYSupport() {
			// Create a styled table for TTY
			t := table.New().
				Border(lipgloss.ThickBorder()).
				BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorBorder))).
				StyleFunc(func(row, col int) lipgloss.Style {
					style := lipgloss.NewStyle().PaddingLeft(1).PaddingRight(1)
					if row == 0 {
						return style.Inherit(theme.Styles.CommandName).Align(lipgloss.Center)
					}
					if row%2 == 0 {
						return style.Inherit(theme.Styles.GrayText)
					}
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
