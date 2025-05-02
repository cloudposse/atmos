package format

import (
	"encoding/json"
	"fmt"
)

// Format implements the Formatter interface for JSONFormatter.
func (f *JSONFormatter) Format(data map[string]interface{}, options FormatOptions) (string, error) {
	jsonBytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", fmt.Errorf("error formatting JSON output: %w", err)
	}
	return string(jsonBytes), nil
}
