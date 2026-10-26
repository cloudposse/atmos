package formatter

import (
	"encoding/json"
	"io"
)

// JSONFormatter formats responses as JSON.
type JSONFormatter struct{}

// Format writes the execution result as pretty-printed JSON.
func (f *JSONFormatter) Format(w io.Writer, result *ExecutionResult) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}
