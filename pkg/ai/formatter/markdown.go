package formatter

import (
	"fmt"
	"io"
)

// MarkdownFormatter formats responses as markdown.
type MarkdownFormatter struct{}

// Format writes the execution result as formatted markdown.
func (f *MarkdownFormatter) Format(w io.Writer, result *ExecutionResult) error {
	if result.Error != nil {
		_, err := fmt.Fprintf(w, "# Error\n\n%s\n", result.Error.Message)
		return err
	}

	// Write the AI response.
	if _, err := fmt.Fprintln(w, result.Response); err != nil {
		return err
	}

	// Optionally add metadata section.
	if len(result.ToolCalls) > 0 {
		if _, err := fmt.Fprint(w, "\n---\n\n"); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "## Tool Executions (%d)\n\n", len(result.ToolCalls)); err != nil {
			return err
		}
		for i, tc := range result.ToolCalls {
			status := "✅"
			if !tc.Success {
				status = "❌"
			}
			if _, err := fmt.Fprintf(w, "%d. %s **%s** (%dms)\n", i+1, status, tc.Tool, tc.DurationMs); err != nil {
				return err
			}
		}
	}

	return nil
}
