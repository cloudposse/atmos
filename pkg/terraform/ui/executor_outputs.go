package ui

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strconv"

	"github.com/charmbracelet/lipgloss"

	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

// Numeric constants.
const (
	floatBitSize = 64
)

// displayOutputs renders terraform outputs after apply using a styled table.
func displayOutputs(tracker *ResourceTracker) {
	outputs := tracker.GetOutputs()
	if outputs == nil || len(outputs.Outputs) == 0 {
		return
	}

	rows := outputRows(outputs.Outputs)
	renderOutputsTable(rows)
}

// fetchAndDisplayOutputs fetches outputs using terraform output -json and displays them.
// This is used when there are no changes to apply but we still want to show current outputs.
// The env parameter is the component's effective environment (credentials, TF_VAR_*,
// backend config) assembled by the caller; without it the subprocess falls back to the
// Atmos process's own ambient environment, which can lack the credentials the component needs.
func fetchAndDisplayOutputs(command, workingDir string, env []string) {
	// Run terraform output -json to get current outputs.
	cmd := exec.Command(command, "output", "-json")
	cmd.Dir = workingDir
	cmd.Env = env

	outputBytes, err := cmd.Output()
	if err != nil {
		// Silently ignore errors - outputs might not exist yet.
		return
	}

	// Parse the JSON output.
	var outputs map[string]OutputValue
	if err := json.Unmarshal(outputBytes, &outputs); err != nil {
		return
	}

	if len(outputs) == 0 {
		return
	}

	renderOutputsTable(outputRows(outputs))
}

// outputRows builds sorted table rows from a map of output values, masking sensitive values.
func outputRows(outputs map[string]OutputValue) [][]string {
	var rows [][]string
	for name, output := range outputs {
		var value string
		if output.Sensitive {
			value = "<sensitive>"
		} else {
			value = formatOutputValue(output.Value)
		}
		rows = append(rows, []string{name, value})
	}

	// Sort rows by output name for consistent display.
	sort.Slice(rows, func(i, j int) bool {
		return rows[i][0] < rows[j][0]
	})

	return rows
}

// renderOutputsTable writes the rendered outputs table to the UI.
func renderOutputsTable(rows [][]string) {
	headers := []string{"Output", "Value"}
	tableStr := createOutputsTable(headers, rows)
	ui.Writef("\n%s\n", tableStr)
}

// formatOutputValue formats an output value for display in a table.
func formatOutputValue(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case float64:
		// JSON numbers are float64.
		if v == float64(int64(v)) {
			return fmt.Sprintf("%d", int64(v))
		}
		return fmt.Sprintf("%g", v)
	case bool:
		return fmt.Sprintf("%t", v)
	case nil:
		return "null"
	default:
		// For complex types (maps, arrays), use JSON.
		data, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(data)
	}
}

// createOutputsTable creates a table with semantic cell styling for terraform outputs.
func createOutputsTable(headers []string, rows [][]string) string {
	styles := theme.GetCurrentStyles()

	config := theme.TableConfig{
		Style:       theme.TableStyleMinimal,
		ShowBorders: false,
		ShowHeader:  true,
		Styles:      styles,
		StyleFunc:   createOutputsStyleFunc(rows, styles),
	}

	return theme.CreateTable(&config, headers, rows)
}

// createOutputsStyleFunc returns a styling function for the outputs table.
func createOutputsStyleFunc(rows [][]string, styles *theme.StyleSet) func(int, int) lipgloss.Style {
	return func(row, col int) lipgloss.Style {
		baseStyle := lipgloss.NewStyle().PaddingLeft(1).PaddingRight(1)

		if styles == nil {
			return baseStyle
		}

		// Header row styling.
		if row == -1 {
			return baseStyle.Inherit(styles.TableHeader)
		}

		// First column (output name) uses standard row styling.
		if col == 0 {
			return baseStyle.Inherit(styles.TableRow)
		}

		// Value column (col 1) - apply semantic styling based on content.
		if row >= 0 && row < len(rows) && col < len(rows[row]) {
			value := rows[row][col]
			return getOutputCellStyle(value, baseStyle, styles)
		}

		return baseStyle.Inherit(styles.TableRow)
	}
}

// getOutputCellStyle returns the appropriate style for an output value cell.
//
//nolint:gocritic // lipgloss.Style is designed to be passed by value (immutable)
func getOutputCellStyle(value string, baseStyle lipgloss.Style, styles *theme.StyleSet) lipgloss.Style {
	contentType := detectOutputContentType(value)

	switch contentType {
	case outputContentTypeBoolean:
		if value == "true" {
			return baseStyle.Foreground(styles.Success.GetForeground())
		}
		return baseStyle.Foreground(styles.Error.GetForeground())

	case outputContentTypeNumber:
		return baseStyle.Foreground(styles.Info.GetForeground())

	case outputContentTypeSensitive:
		return baseStyle.Foreground(styles.Muted.GetForeground())

	case outputContentTypeNull:
		return baseStyle.Foreground(styles.Muted.GetForeground())

	default:
		return baseStyle.Inherit(styles.TableRow)
	}
}

// outputContentType represents the type of content in an output value cell.
type outputContentType int

const (
	outputContentTypeDefault outputContentType = iota
	outputContentTypeBoolean
	outputContentTypeNumber
	outputContentTypeSensitive
	outputContentTypeNull
)

// detectOutputContentType determines the content type of an output value.
func detectOutputContentType(value string) outputContentType {
	if value == "" {
		return outputContentTypeDefault
	}

	// Check for sensitive marker.
	if value == "<sensitive>" {
		return outputContentTypeSensitive
	}

	// Check for null.
	if value == "null" {
		return outputContentTypeNull
	}

	// Check for booleans.
	if value == "true" || value == "false" {
		return outputContentTypeBoolean
	}

	// Check for numbers (integers or floats).
	if isNumericString(value) {
		return outputContentTypeNumber
	}

	return outputContentTypeDefault
}

// isNumericString checks if a string represents a number.
func isNumericString(s string) bool {
	// Try to parse as float (covers both integers and floats).
	_, err := strconv.ParseFloat(s, floatBitSize)
	return err == nil
}
