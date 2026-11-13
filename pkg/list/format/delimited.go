package format

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/cloudposse/atmos/pkg/utils"
)

const (
	DefaultCSVDelimiter = ","
	DefaultTSVDelimiter = "\t"
	ValueKey            = "value"
)

// Format implements the Formatter interface for DelimitedFormatter.
func (f *DelimitedFormatter) Format(data map[string]interface{}, options FormatOptions) (string, error) {
	f.setDefaultDelimiter(&options)

	keys := extractSortedKeys(data)
	valueKeys := getValueKeysFromStacks(data, keys)
	header, rows := f.generateHeaderAndRows(keys, valueKeys, data)

	return f.buildOutput(header, rows, options.Delimiter), nil
}

// setDefaultDelimiter sets the default delimiter if not specified.
func (f *DelimitedFormatter) setDefaultDelimiter(options *FormatOptions) {
	if options.Delimiter == "" {
		if f.format == FormatCSV {
			options.Delimiter = DefaultCSVDelimiter
		} else {
			options.Delimiter = DefaultTSVDelimiter
		}
	}
}

// extractSortedKeys extracts and sorts the keys from data.
func extractSortedKeys(data map[string]interface{}) []string {
	var keys []string
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// getValueKeysFromStacks extracts all possible value keys from the first stack.
func getValueKeysFromStacks(data map[string]interface{}, keys []string) []string {
	var valueKeys []string

	for _, stackName := range keys {
		stackData := data[stackName]

		switch typedData := stackData.(type) {
		case map[string]interface{}:
			if varsData, ok := typedData["vars"].(map[string]interface{}); ok {
				for k := range varsData {
					valueKeys = append(valueKeys, k)
				}
				break
			}

			for k := range typedData {
				valueKeys = append(valueKeys, k)
			}

		case []interface{}:
			valueKeys = []string{ValueKey}

		default:
			valueKeys = []string{ValueKey}
		}

		if len(valueKeys) > 0 {
			break
		}
	}
	sort.Strings(valueKeys)
	return valueKeys
}

// generateHeaderAndRows creates the header and rows for the delimited output.
func (f *DelimitedFormatter) generateHeaderAndRows(keys []string, valueKeys []string, data map[string]interface{}) ([]string, [][]string) {
	// Create header
	header := []string{"Key"}
	header = append(header, keys...)

	var rows [][]string

	if len(valueKeys) == 1 && valueKeys[0] == ValueKey {
		rows = f.generateValueKeyRows(keys, data)
	} else {
		rows = f.generatePropertyKeyRows(keys, valueKeys, data)
	}

	return header, rows
}

// generateValueKeyRows creates rows for the special case with a "value" key.
func (f *DelimitedFormatter) generateValueKeyRows(keys []string, data map[string]interface{}) [][]string {
	var rows [][]string
	row := []string{ValueKey}

	for _, stackName := range keys {
		value := ""

		stackData := data[stackName]

		switch typedData := stackData.(type) {
		case map[string]interface{}:
			if val, ok := typedData[ValueKey]; ok {
				value = formatValue(val)
			} else {
				value = formatValue(typedData)
			}
		default:
			value = formatValue(stackData)
		}

		row = append(row, value)
	}

	rows = append(rows, row)
	return rows
}

// extractPropertyValue gets a property value from a stack's data structure,
// checking both top-level keys and the vars map.
func extractPropertyValue(stackData map[string]interface{}, propertyKey string) string {
	// Check if this is a top-level key
	if val, ok := stackData[propertyKey]; ok {
		return formatValue(val)
	}

	// Check if this is a nested key in the vars map
	varsData, ok := stackData["vars"].(map[string]interface{})
	if !ok {
		return ""
	}

	if val, ok := varsData[propertyKey]; ok {
		return formatValue(val)
	}

	return ""
}

// generatePropertyKeyRows creates rows where each row represents a property key with values
// from different stacks as columns. This is different from generateValueKeyRows which handles
// the special case where stacks have a single "value" key.
func (f *DelimitedFormatter) generatePropertyKeyRows(keys []string, valueKeys []string, data map[string]interface{}) [][]string {
	var rows [][]string

	// Property key case: for each value key, create a row
	for _, valueKey := range valueKeys {
		row := []string{valueKey}

		for _, stackName := range keys {
			value := ""
			stackData, ok := data[stackName].(map[string]interface{})
			if ok {
				value = extractPropertyValue(stackData, valueKey)
			}
			row = append(row, value)
		}
		rows = append(rows, row)
	}
	return rows
}

// buildOutput builds the final delimited output string.
// When the delimiter is a single character that encoding/csv accepts, fields are written with
// encoding/csv so values that contain the delimiter, quotes or newlines are quoted.
// Any other delimiter (multi-character or reserved by encoding/csv) falls back to joining the fields as is.
func (f *DelimitedFormatter) buildOutput(header []string, rows [][]string, delimiter string) string {
	if comma, ok := csvDelimiter(delimiter); ok {
		var output bytes.Buffer
		writer := csv.NewWriter(&output)
		writer.Comma = comma
		writer.UseCRLF = utils.GetLineEnding() == "\r\n"
		records := append([][]string{header}, rows...)
		if err := writer.WriteAll(records); err == nil {
			return output.String()
		}
	}

	var output strings.Builder
	output.WriteString(strings.Join(header, delimiter) + utils.GetLineEnding())
	for _, row := range rows {
		output.WriteString(strings.Join(row, delimiter) + utils.GetLineEnding())
	}
	return output.String()
}

// csvDelimiter returns the delimiter as a rune when encoding/csv can use it.
func csvDelimiter(delimiter string) (rune, bool) {
	r, size := utf8.DecodeRuneInString(delimiter)
	if size == 0 || size != len(delimiter) || r == utf8.RuneError || r == '"' || r == '\r' || r == '\n' {
		return 0, false
	}
	return r, true
}

// FormatValue converts a value to its string representation.
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
