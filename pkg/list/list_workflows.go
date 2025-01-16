package list

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	"github.com/samber/lo"

	"github.com/cloudposse/atmos/pkg/schema"
)

// getWorkflowsFromManifest extracts workflows from a workflow manifest
func getWorkflowsFromManifest(manifest schema.WorkflowManifest) ([][]string, error) {
	var rows [][]string
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
func FilterAndListWorkflows(fileFlag string, listConfig schema.ListConfig) (string, error) {
	// Parse columns configuration
	header := []string{"File", "Workflow", "Description"}

	// Get all workflows from manifests
	var rows [][]string

	// TODO: Implement actual workflow manifest loading logic
	// For now using example data
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

	// Remove duplicates and sort
	rows = lo.UniqBy(rows, func(row []string) string {
		return strings.Join(row, "\t")
	})
	sort.Slice(rows, func(i, j int) bool {
		return strings.Join(rows[i], "\t") < strings.Join(rows[j], "\t")
	})

	if len(rows) == 0 {
		return "No workflows found", nil
	}

	// Calculate column widths
	colWidths := make([]int, len(header))
	for i, h := range header {
		colWidths[i] = len(h)
	}
	for _, row := range rows {
		for i, field := range row {
			if len(field) > colWidths[i] {
				colWidths[i] = len(field)
			}
		}
	}

	// Check if TTY is attached
	if !exec.CheckTTYSupport() {
		// Degrade to simple tabular format
		var output strings.Builder
		output.WriteString(strings.Join(header, "\t") + "\n")
		for _, row := range rows {
			output.WriteString(strings.Join(row, "\t") + "\n")
		}
		return output.String(), nil
	}

	// Create a styled table
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

	return t.String() + "\n", nil
}
