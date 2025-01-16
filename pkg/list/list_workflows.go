package list

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	"github.com/samber/lo"

	"github.com/cloudposse/atmos/pkg/schema"
	"gopkg.in/yaml.v3"
)

// Extracts workflows from a workflow manifest
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

// Filters and lists workflows based on the given file
func FilterAndListWorkflows(fileFlag string, listConfig schema.ListConfig) (string, error) {
	header := []string{"File", "Workflow", "Description"}

	var rows [][]string

	// If a specific file is provided, validate and load it
	if fileFlag != "" {
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
		return strings.Join(row, "\t")
	})
	sort.Slice(rows, func(i, j int) bool {
		return strings.Join(rows[i], "\t") < strings.Join(rows[j], "\t")
	})

	if len(rows) == 0 {
		return "No workflows found", nil
	}

	// See if TTY is attached
	if !exec.CheckTTYSupport() {
		// Degrade it to tabular output
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
