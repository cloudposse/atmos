package step

import (
	"context"
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

// TableHandler renders data as a formatted table.
type TableHandler struct {
	BaseHandler
}

func init() {
	Register(&TableHandler{
		BaseHandler: NewBaseHandler("table", CategoryOutput, false),
	})
}

// Validate checks that the step has required fields.
func (h *TableHandler) Validate(step *schema.WorkflowStep) error {
	if len(step.Data) == 0 && step.Content == "" {
		return errUtils.Build(errUtils.ErrStepDataOrContentRequired).
			WithContext("step", step.Name).
			WithContext("type", "table").
			Err()
	}
	return nil
}

// Execute renders the table.
func (h *TableHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	if step.Content != "" {
		return h.executeContentTable(ctx, step, vars)
	}
	return h.executeDataTable(step, vars)
}

// executeContentTable renders content as raw table data.
func (h *TableHandler) executeContentTable(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	content, err := h.ResolveContent(ctx, step, vars)
	if err != nil {
		return nil, err
	}
	if err := data.Writeln(content); err != nil {
		return nil, err
	}
	return NewStepResult(content), nil
}

// executeDataTable builds and renders a table from structured data.
func (h *TableHandler) executeDataTable(step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	styles := theme.GetCurrentStyles()
	columns := h.determineColumns(step)
	header := h.buildHeader(columns, styles)
	rows := h.buildRows(step.Data, columns)

	output := strings.Join(header, "\t") + "\n" + strings.Join(rows, "\n")

	output, err := h.addTitle(output, step, vars, styles)
	if err != nil {
		return nil, err
	}

	if err := data.Writeln(output); err != nil {
		return nil, err
	}

	return NewStepResult(output), nil
}

// determineColumns returns the columns to use for the table.
func (h *TableHandler) determineColumns(step *schema.WorkflowStep) []string {
	if len(step.Columns) > 0 {
		return step.Columns
	}
	if len(step.Data) == 0 {
		return nil
	}
	var columns []string
	for k := range step.Data[0] {
		columns = append(columns, k)
	}
	return columns
}

// buildHeader builds the styled table header.
func (h *TableHandler) buildHeader(columns []string, styles *theme.StyleSet) []string {
	header := make([]string, len(columns))
	for i, col := range columns {
		if styles != nil {
			header[i] = styles.TableHeader.Render(col)
		} else {
			header[i] = col
		}
	}
	return header
}

// buildRows builds the table rows from data.
func (h *TableHandler) buildRows(data []map[string]any, columns []string) []string {
	rows := make([]string, len(data))
	for i, rowData := range data {
		cells := make([]string, len(columns))
		for j, col := range columns {
			if v, ok := rowData[col]; ok {
				cells[j] = fmt.Sprintf("%v", v)
			}
		}
		rows[i] = strings.Join(cells, "\t")
	}
	return rows
}

// addTitle adds a styled title to the output if present.
func (h *TableHandler) addTitle(output string, step *schema.WorkflowStep, vars *Variables, styles *theme.StyleSet) (string, error) {
	if step.Title == "" {
		return output, nil
	}
	title, err := vars.Resolve(step.Title)
	if err != nil {
		return "", fmt.Errorf("step '%s': failed to resolve title: %w", step.Name, err)
	}
	if styles != nil {
		title = styles.Title.Render(title)
	}
	return title + "\n\n" + output, nil
}
