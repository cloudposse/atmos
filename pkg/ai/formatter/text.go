package formatter

import (
	"fmt"
	"io"
)

// TextFormatter formats responses as plain text.
type TextFormatter struct{}

// Format writes the execution result as plain text (just the AI response).
func (f *TextFormatter) Format(w io.Writer, result *ExecutionResult) error {
	if result.Error != nil {
		_, err := fmt.Fprintf(w, "Error: %s\n", result.Error.Message)
		return err
	}

	_, err := fmt.Fprintln(w, result.Response)
	return err
}
