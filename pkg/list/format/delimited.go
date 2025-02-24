package format

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/cloudposse/atmos/pkg/utils"
)

const (
	DefaultCSVDelimiter = ","
	DefaultTSVDelimiter = "\t"
)

// Format implements the Formatter interface for DelimitedFormatter
func (f *DelimitedFormatter) Format(data map[string]interface{}, options FormatOptions) (string, error) {
	// Set default delimiter based on format
	if options.Delimiter == "" {
		if f.format == FormatCSV {
			options.Delimiter = DefaultCSVDelimiter
		} else {
			options.Delimiter = DefaultTSVDelimiter
		}
	}

	// Extract and sort keys
	var keys []string
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Get all possible value keys from the first stack
	var valueKeys []string
	for _, stackName := range keys {
		if stackData, ok := data[stackName].(map[string]interface{}); ok {
			if _, hasValue := stackData["value"]; hasValue {
				valueKeys = []string{"value"}
				break
			}
			// collect all keys from the map
			for k := range stackData {
				valueKeys = append(valueKeys, k)
			}
			break
		}
	}
	sort.Strings(valueKeys)

	// Create header and rows we may need to change this in the future to be more flexible
	header := []string{"Key"}
	for _, k := range keys {
		header = append(header, k)
	}

	var rows [][]string
	for _, valueKey := range valueKeys {
		row := []string{valueKey}
		for _, stackName := range keys {
			value := ""
			if stackData, ok := data[stackName].(map[string]interface{}); ok {
				if val, ok := stackData[valueKey]; ok {
					value = formatValue(val)
				}
			}
			row = append(row, value)
		}
		rows = append(rows, row)
	}

	// Build output
	var output strings.Builder
	output.WriteString(strings.Join(header, options.Delimiter) + utils.GetLineEnding())
	for _, row := range rows {
		output.WriteString(strings.Join(row, options.Delimiter) + utils.GetLineEnding())
	}
	return output.String(), nil
}

// FormatValue converts a value to its string representation
func formatValue(value interface{}) string {
	switch v := value.(type) {
	case string:
		return v
	case []interface{}:
		var values []string
		for _, item := range v {
			values = append(values, fmt.Sprintf("%v", item))
		}
		return strings.Join(values, ",")
	case map[string]interface{}:
		jsonBytes, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(jsonBytes)
	default:
		return fmt.Sprintf("%v", v)
	}
}
