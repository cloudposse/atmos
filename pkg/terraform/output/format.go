package output

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// singleQuoteEscape is the escape sequence for single quotes in shell literals.
// Transforms ' to '\” which ends the string, adds escaped quote, reopens string.
const singleQuoteEscape = `'\''`

// escapeSingleQuotes escapes single quotes for safe shell literal strings.
func escapeSingleQuotes(s string) string {
	return strings.ReplaceAll(s, "'", singleQuoteEscape)
}

// FormatOutputs converts terraform outputs map to the specified format.
func FormatOutputs(outputs map[string]any, format Format) (string, error) {
	defer perf.Track(nil, "output.FormatOutputs")()

	return FormatOutputsWithOptions(outputs, format, FormatOptions{})
}

// FormatOutputsWithOptions converts terraform outputs map to the specified format with options.
func FormatOutputsWithOptions(outputs map[string]any, format Format, opts FormatOptions) (string, error) {
	defer perf.Track(nil, "output.FormatOutputsWithOptions")()

	// Apply transformations in order: flatten first, then uppercase.
	transformed := outputs
	if opts.Flatten {
		transformed = flattenMap(transformed, "", opts.GetFlattenSeparator())
	}
	transformed = transformKeys(transformed, opts)

	return formatWithOptions(transformed, format)
}

// formatWithOptions applies the specified format to transformed outputs.
func formatWithOptions(transformed map[string]any, format Format) (string, error) {
	switch format {
	case FormatJSON:
		return formatJSON(transformed)
	case FormatYAML:
		return formatYAML(transformed)
	case FormatHCL:
		return formatHCL(transformed)
	case FormatEnv:
		return formatEnv(transformed)
	case FormatDotenv:
		return formatDotenv(transformed)
	case FormatBash:
		return formatBash(transformed)
	case FormatCSV:
		return formatCSV(transformed)
	case FormatTSV:
		return formatTSV(transformed)
	default:
		return "", errUtils.Build(errUtils.ErrInvalidArgumentError).
			WithExplanationf("Unsupported format %q.", format).
			WithHintf("Supported formats: %s.", strings.Join(SupportedFormats, ", ")).
			Err()
	}
}

// transformKeys applies key transformations based on options.
func transformKeys(outputs map[string]any, opts FormatOptions) map[string]any {
	if !opts.Uppercase {
		return outputs
	}
	transformed := make(map[string]any, len(outputs))
	for k, v := range outputs {
		transformed[strings.ToUpper(k)] = v
	}
	return transformed
}

// flattenMap recursively flattens nested maps and arrays into a single-level map with compound keys.
// For example: {"config": {"host": "localhost", "port": 3000}} becomes
// {"config_host": "localhost", "config_port": 3000} with separator "_".
// Arrays are flattened with numeric indices: {"hosts": ["a", "b"]} becomes
// {"hosts_0": "a", "hosts_1": "b"}.
func flattenMap(m map[string]any, prefix, separator string) map[string]any {
	result := make(map[string]any)
	flattenMapRecursive(m, prefix, separator, result)
	return result
}

// flattenMapRecursive is the recursive helper for flattenMap.
func flattenMapRecursive(m map[string]any, prefix, separator string, result map[string]any) {
	for k, v := range m {
		key := k
		if prefix != "" {
			key = prefix + separator + k
		}
		flattenValue(key, v, separator, result)
	}
}

// flattenValue flattens a single value into the result map.
func flattenValue(key string, v any, separator string, result map[string]any) {
	switch val := v.(type) {
	case map[string]any:
		// Recursively flatten nested maps.
		flattenMapRecursive(val, key, separator, result)
	case []any:
		// Flatten arrays with numeric indices.
		flattenSlice(key, val, separator, result)
	default:
		// Scalar values are stored as-is.
		result[key] = v
	}
}

// flattenSlice flattens an array into the result map with numeric indices.
// For example: {"hosts": ["a", "b"]} becomes {"hosts_0": "a", "hosts_1": "b"}.
func flattenSlice(prefix string, slice []any, separator string, result map[string]any) {
	for i, v := range slice {
		key := fmt.Sprintf("%s%s%d", prefix, separator, i)
		flattenValue(key, v, separator, result)
	}
}

// FormatSingleValue formats a single terraform output value.
// For scalar formats (env, dotenv, bash, csv, tsv), complex values (maps/lists) are not supported.
// For structured formats (json, yaml, hcl), any value type is supported.
func FormatSingleValue(key string, value any, format Format) (string, error) {
	defer perf.Track(nil, "output.FormatSingleValue")()

	return FormatSingleValueWithOptions(key, value, format, FormatOptions{})
}

// FormatSingleValueWithOptions formats a single terraform output value with options.
func FormatSingleValueWithOptions(key string, value any, format Format, opts FormatOptions) (string, error) {
	defer perf.Track(nil, "output.FormatSingleValueWithOptions")()

	// Validate that scalar-only formats don't receive complex values.
	if err := ValidateSingleValueFormat(value, format); err != nil {
		return "", err
	}

	// Transform key if uppercase option is set.
	transformedKey := key
	if opts.Uppercase {
		transformedKey = strings.ToUpper(key)
	}

	return dispatchSingleValueFormat(transformedKey, value, format)
}

// dispatchSingleValueFormat routes to the appropriate format handler.
func dispatchSingleValueFormat(key string, value any, format Format) (string, error) {
	switch format {
	case FormatJSON:
		return formatSingleJSON(value)
	case FormatYAML:
		return formatSingleYAML(value)
	case FormatHCL:
		return formatSingleHCL(key, value)
	case FormatEnv:
		return formatSingleEnv(key, value)
	case FormatDotenv:
		return formatSingleDotenv(key, value)
	case FormatBash:
		return formatSingleBash(key, value)
	case FormatCSV:
		return formatSingleDelimited(key, value, ",")
	case FormatTSV:
		return formatSingleDelimited(key, value, "\t")
	default:
		return "", errUtils.Build(errUtils.ErrInvalidArgumentError).
			WithExplanationf("Unsupported format %q.", format).
			WithHintf("Supported formats: %s.", strings.Join(SupportedFormats, ", ")).
			Err()
	}
}

// formatSingleJSON outputs a single value as JSON.
func formatSingleJSON(value any) (string, error) {
	jsonBytes, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal value to JSON: %w", err)
	}
	return string(jsonBytes) + "\n", nil
}

// formatSingleYAML outputs a single value as YAML.
func formatSingleYAML(value any) (string, error) {
	yamlBytes, err := yaml.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("failed to marshal value to YAML: %w", err)
	}
	return string(yamlBytes), nil
}

// formatSingleHCL outputs a single value as HCL assignment.
func formatSingleHCL(key string, value any) (string, error) {
	hclValue, err := valueToHCL(value)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s = %s\n", key, hclValue), nil
}

// formatSingleEnv outputs a single value as key=value.
func formatSingleEnv(key string, value any) (string, error) {
	strVal, err := valueToString(value)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s=%s\n", key, strVal), nil
}

// formatSingleDotenv outputs a single value as key='value'.
func formatSingleDotenv(key string, value any) (string, error) {
	strVal, err := valueToString(value)
	if err != nil {
		return "", err
	}
	safe := escapeSingleQuotes(strVal)
	return fmt.Sprintf("%s='%s'\n", key, safe), nil
}

// formatSingleBash outputs a single value as export key='value'.
func formatSingleBash(key string, value any) (string, error) {
	strVal, err := valueToString(value)
	if err != nil {
		return "", err
	}
	safe := escapeSingleQuotes(strVal)
	return fmt.Sprintf("export %s='%s'\n", key, safe), nil
}

// formatSingleDelimited outputs a single value as key<delimiter>value (no header).
func formatSingleDelimited(key string, value any, delimiter string) (string, error) {
	strVal, err := valueToString(value)
	if err != nil {
		return "", err
	}
	escapedVal := escapeDelimitedValue(strVal, delimiter)
	return key + delimiter + escapedVal + "\n", nil
}

// formatJSON outputs as a JSON object.
func formatJSON(outputs map[string]any) (string, error) {
	jsonBytes, err := json.MarshalIndent(outputs, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal outputs to JSON: %w", err)
	}
	return string(jsonBytes) + "\n", nil
}

// formatYAML outputs as YAML.
func formatYAML(outputs map[string]any) (string, error) {
	yamlBytes, err := yaml.Marshal(outputs)
	if err != nil {
		return "", fmt.Errorf("failed to marshal outputs to YAML: %w", err)
	}
	return string(yamlBytes), nil
}

// formatHCL outputs as HCL format: key = "value".
func formatHCL(outputs map[string]any) (string, error) {
	keys := sortedKeys(outputs)
	var sb strings.Builder

	for _, key := range keys {
		value := outputs[key]
		if value == nil {
			continue // Skip null values.
		}

		hclValue, err := valueToHCL(value)
		if err != nil {
			return "", err
		}

		sb.WriteString(fmt.Sprintf("%s = %s\n", key, hclValue))
	}

	return sb.String(), nil
}

// formatEnv outputs key=value (no quotes, no export) - ideal for $GITHUB_OUTPUT.
func formatEnv(outputs map[string]any) (string, error) {
	keys := sortedKeys(outputs)
	var sb strings.Builder

	for _, key := range keys {
		value := outputs[key]
		if value == nil {
			continue // Skip null values.
		}

		strVal, err := valueToString(value)
		if err != nil {
			return "", err
		}

		sb.WriteString(fmt.Sprintf("%s=%s\n", key, strVal))
	}

	return sb.String(), nil
}

// formatDotenv outputs key='value' for .env files.
func formatDotenv(outputs map[string]any) (string, error) {
	keys := sortedKeys(outputs)
	var sb strings.Builder

	for _, key := range keys {
		value := outputs[key]
		if value == nil {
			continue // Skip null values.
		}

		strVal, err := valueToString(value)
		if err != nil {
			return "", err
		}

		// Escape single quotes for safe single-quoted shell literals: ' -> '\''.
		safe := escapeSingleQuotes(strVal)
		sb.WriteString(fmt.Sprintf("%s='%s'\n", key, safe))
	}

	return sb.String(), nil
}

// formatBash outputs export key='value' for shell sourcing.
func formatBash(outputs map[string]any) (string, error) {
	keys := sortedKeys(outputs)
	var sb strings.Builder

	for _, key := range keys {
		value := outputs[key]
		if value == nil {
			continue // Skip null values.
		}

		strVal, err := valueToString(value)
		if err != nil {
			return "", err
		}

		// Escape single quotes for safe single-quoted shell literals: ' -> '\''.
		safe := escapeSingleQuotes(strVal)
		sb.WriteString(fmt.Sprintf("export %s='%s'\n", key, safe))
	}

	return sb.String(), nil
}

// formatCSV outputs key,value with proper CSV escaping.
func formatCSV(outputs map[string]any) (string, error) {
	return formatDelimited(outputs, ",")
}

// formatTSV outputs key<tab>value with proper TSV escaping.
func formatTSV(outputs map[string]any) (string, error) {
	return formatDelimited(outputs, "\t")
}

// formatDelimited outputs key<delimiter>value with proper escaping.
func formatDelimited(outputs map[string]any, delimiter string) (string, error) {
	keys := sortedKeys(outputs)
	var sb strings.Builder

	// Write header.
	sb.WriteString("key" + delimiter + "value\n")

	for _, key := range keys {
		value := outputs[key]
		if value == nil {
			continue // Skip null values.
		}

		strVal, err := valueToString(value)
		if err != nil {
			return "", err
		}

		// Escape values that contain delimiter, quotes, or newlines.
		escapedVal := escapeDelimitedValue(strVal, delimiter)
		sb.WriteString(key + delimiter + escapedVal + "\n")
	}

	return sb.String(), nil
}

// escapeDelimitedValue escapes a value for CSV/TSV format.
func escapeDelimitedValue(value, delimiter string) string {
	needsQuoting := strings.ContainsAny(value, delimiter+"\"\n\r")
	if !needsQuoting {
		return value
	}
	// Escape double quotes by doubling them.
	escaped := strings.ReplaceAll(value, "\"", "\"\"")
	return "\"" + escaped + "\""
}

// valueToString converts a terraform output value to a string.
// Scalars are returned as-is, complex types are JSON-encoded.
func valueToString(value any) (string, error) {
	switch v := value.(type) {
	case string:
		return v, nil
	case float64:
		// Check if it's an integer value.
		if v == float64(int64(v)) {
			return fmt.Sprintf("%d", int64(v)), nil
		}
		return fmt.Sprintf("%v", v), nil
	case bool:
		return fmt.Sprintf("%t", v), nil
	case nil:
		return "", nil
	default:
		// Complex types (maps, slices) - JSON encode.
		jsonBytes, err := json.Marshal(v)
		if err != nil {
			return "", fmt.Errorf("failed to marshal value: %w", err)
		}
		return string(jsonBytes), nil
	}
}

// valueToHCL converts a terraform output value to HCL format.
func valueToHCL(value any) (string, error) {
	switch v := value.(type) {
	case string:
		return fmt.Sprintf("%q", v), nil // %q escapes backslashes, quotes, and special characters.
	case float64:
		return formatHCLNumber(v), nil
	case bool:
		return fmt.Sprintf("%t", v), nil
	case nil:
		return "null", nil
	case []any:
		return formatHCLList(v)
	case map[string]any:
		return formatHCLObject(v)
	default:
		return formatHCLFallback(v)
	}
}

// formatHCLNumber formats a float64 as integer if whole, otherwise as float.
func formatHCLNumber(v float64) string {
	if v == float64(int64(v)) {
		return fmt.Sprintf("%d", int64(v))
	}
	return fmt.Sprintf("%v", v)
}

// formatHCLList formats a slice as an HCL list.
func formatHCLList(v []any) (string, error) {
	items := make([]string, 0, len(v))
	for _, item := range v {
		hclItem, err := valueToHCL(item)
		if err != nil {
			return "", err
		}
		items = append(items, hclItem)
	}
	return "[" + strings.Join(items, ", ") + "]", nil
}

// formatHCLObject formats a map as an HCL object.
func formatHCLObject(v map[string]any) (string, error) {
	keys := sortedKeys(v)
	items := make([]string, 0, len(keys))
	for _, key := range keys {
		hclValue, err := valueToHCL(v[key])
		if err != nil {
			return "", err
		}
		items = append(items, fmt.Sprintf("%s = %s", key, hclValue))
	}
	return "{\n    " + strings.Join(items, "\n    ") + "\n  }", nil
}

// formatHCLFallback uses JSON encoding for unknown types.
func formatHCLFallback(v any) (string, error) {
	jsonBytes, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("failed to marshal value to HCL: %w", err)
	}
	return string(jsonBytes), nil
}

// sortedKeys returns the keys of a map sorted alphabetically.
func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
