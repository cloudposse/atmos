package step

import (
	"context"
	"fmt"
	"strings"

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
		return fmt.Errorf("step '%s' (table): either data or content is required", step.Name)
	}
	return nil
}

// Execute renders the table.
func (h *TableHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	// If content is provided, use it as raw table data.
	if step.Content != "" {
		content, err := h.ResolveContent(ctx, step, vars)
		if err != nil {
			return nil, err
		}
		if err := data.Writeln(content); err != nil {
			return nil, err
		}
		return NewStepResult(content), nil
	}

	// Build table from data array.
	styles := theme.GetCurrentStyles()

	// Determine columns.
	columns := step.Columns
	if len(columns) == 0 && len(step.Data) > 0 {
		// Infer columns from first row.
		for k := range step.Data[0] {
			columns = append(columns, k)
		}
	}

	// Build header.
	var header []string
	for _, col := range columns {
		if styles != nil {
			header = append(header, styles.TableHeader.Render(col))
		} else {
			header = append(header, col)
		}
	}

	// Build rows.
	var rows []string
	for _, rowData := range step.Data {
		var cells []string
		for _, col := range columns {
			val := ""
			if v, ok := rowData[col]; ok {
				val = fmt.Sprintf("%v", v)
			}
			cells = append(cells, val)
		}
		rows = append(rows, strings.Join(cells, "\t"))
	}

	// Simple tab-separated output (for basic table rendering).
	// TODO: Integrate with pkg/list/format/table.go for lipgloss styling.
	output := strings.Join(header, "\t") + "\n" + strings.Join(rows, "\n")

	// Show title if present.
	if step.Title != "" {
		title, err := vars.Resolve(step.Title)
		if err != nil {
			return nil, fmt.Errorf("step '%s': failed to resolve title: %w", step.Name, err)
		}
		if styles != nil {
			title = styles.Title.Render(title)
		}
		output = title + "\n\n" + output
	}

	if err := data.Writeln(output); err != nil {
		return nil, err
	}

	return NewStepResult(output), nil
}
