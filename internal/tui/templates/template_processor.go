package templates

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/itchyny/gojq"
)

// ProcessJSONWithTemplate takes a JSON-serializable input and applies a gojq template to it
func ProcessJSONWithTemplate(input interface{}, template string) (string, error) {
	// If no template is provided, just return JSON marshaled string
	if template == "" {
		jsonBytes, err := json.MarshalIndent(input, "", "  ")
		if err != nil {
			return "", fmt.Errorf("error marshaling JSON: %w", err)
		}
		return string(jsonBytes), nil
	}

	// Parse the template query
	query, err := gojq.Parse(template)
	if err != nil {
		return "", fmt.Errorf("error parsing template: %w", err)
	}

	// Create a buffer to store the output
	var buf bytes.Buffer

	// Run the query
	iter := query.Run(input)
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if err, ok := v.(error); ok {
			return "", fmt.Errorf("error processing template: %w", err)
		}

		// Marshal each result to JSON
		jsonBytes, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return "", fmt.Errorf("error marshaling result: %w", err)
		}

		// Add newline between multiple results
		if buf.Len() > 0 {
			buf.WriteString("\n")
		}
		buf.Write(jsonBytes)
	}

	return buf.String(), nil
}
