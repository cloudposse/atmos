package list

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	log "github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/internal/tui/templates/term"
	listerrors "github.com/cloudposse/atmos/pkg/list/errors"
	"github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/utils"
	"github.com/pkg/errors"
)

// Error variables for list_values package.
var (
	ErrInvalidStackPattern = errors.New("invalid stack pattern")
)

// Component and section name constants.
const (
	// KeyTerraform is the key for terraform components.
	KeyTerraform = "terraform"
	// KeySettings is the key for settings section.
	KeySettings = "settings"
	// KeyMetadata is the key for metadata section.
	KeyMetadata = "metadata"
	// KeyComponents is the key for components section.
	KeyComponents = "components"
	// KeyVars is the key for vars section in components.
	KeyVars = "vars"
	// KeyAbstract is the key for abstract flag in components.
	KeyAbstract = "abstract"
	// KeyStack is the key used in log messages to identify stack contexts.
	KeyStack = "stack"
	// DotChar is the dot character used in queries.
	DotChar = "."
	// LeftBracketChar is the left bracket character used in array indices.
	LeftBracketChar = "["
	// KeyValue is the key used for scalar values in result maps.
	KeyValue = "value"
)

// FilterOptions contains the options for filtering and listing component values.
type FilterOptions struct {
	Component       string
	Query           string
	IncludeAbstract bool
	MaxColumns      int
	FormatStr       string
	Delimiter       string
	StackPattern    string
}

// FilterAndListValues filters and lists component values across stacks.
func FilterAndListValues(stacksMap map[string]interface{}, options *FilterOptions) (string, error) {
	// Set default format if not specified
	if options.FormatStr == "" {
		options.FormatStr = string(format.FormatTable)
	}

	if err := format.ValidateFormat(options.FormatStr); err != nil {
		return "", err
	}

	// Extract stack values
	extractedValues, err := extractComponentValues(stacksMap, options.Component, options.IncludeAbstract)
	if err != nil {
		return "", err
	}

	// Apply filters
	filteredValues, err := applyFilters(extractedValues, options.StackPattern, options.MaxColumns)
	if err != nil {
		return "", err
	}

	// Apply query to values
	queriedValues, err := applyQuery(filteredValues, options.Query, options.Component)
	if err != nil {
		return "", err
	}

	// Format the output
	return formatOutput(queriedValues, options.FormatStr, options.Delimiter, options.MaxColumns)
}

// extractComponentValues extracts the component values from all stacks.
func extractComponentValues(stacksMap map[string]interface{}, component string, includeAbstract bool) (map[string]interface{}, error) {
	values := make(map[string]interface{})

	for stackName, stackData := range stacksMap {
		stack, ok := stackData.(map[string]interface{})
		if !ok {
			log.Debug("stack data is not a map", KeyStack, stackName)
			continue
		}

		// Build YQ expression based on component type
		yqExpression := processComponentType(component, includeAbstract)

		// Execute YQ query
		queryResult, err := utils.EvaluateYqExpression(nil, stack, yqExpression)
		if err != nil || queryResult == nil {
			log.Debug("no values found",
				KeyStack, stackName,
				"component", component,
				"yq_expression", yqExpression,
				"error", err)
			continue
		}

		// Process the result based on component type
		values[stackName] = processQueryResult(component, queryResult)
	}

	if len(values) == 0 {
		return nil, &listerrors.NoValuesFoundError{Component: component}
	}

	return values, nil
}

// processComponentType determines the YQ expression based on component type.
func processComponentType(component string, includeAbstract bool) string {
	switch component {
	case KeySettings:
		return "select(.settings // .terraform.settings // .components.terraform.*.settings)"
	case KeyMetadata:
		return DotChar + KeyMetadata
	default:
		// Extract component name from path
		componentName := getComponentNameFromPath(component)

		// Build query for component vars
		return buildComponentYqExpression(componentName, includeAbstract)
	}
}

// getComponentNameFromPath extracts the component name from a potentially nested path.
func getComponentNameFromPath(component string) string {
	parts := strings.Split(component, "/")
	if len(parts) > 1 {
		return parts[len(parts)-1]
	}
	return component
}

// buildComponentYqExpression creates the YQ expression for extracting component vars.
func buildComponentYqExpression(componentName string, includeAbstract bool) string {
	// Base expression to target the component
	yqExpression := fmt.Sprintf("%scomponents%s%s%s%s", DotChar, DotChar, KeyTerraform, DotChar, componentName)

	// If not including abstract components, filter them out
	if !includeAbstract {
		// Only get component that either doesn't have abstract flag or has it set to false
		yqExpression += fmt.Sprintf(" | select(has(\"%s\") == false or %s%s == false)",
			KeyAbstract, DotChar, KeyAbstract)
	}

	// Get the vars
	yqExpression += fmt.Sprintf(" | %s%s", DotChar, KeyVars)

	return yqExpression
}

// processQueryResult handles the query result based on component type.
func processQueryResult(component string, queryResult interface{}) interface{} {
	// Process settings specially to handle nested settings key
	if component == KeySettings {
		if settings, ok := queryResult.(map[string]interface{}); ok {
			if settingsContent, ok := settings[KeySettings].(map[string]interface{}); ok {
				return settingsContent
			}
		}
	}

	// Return the result as is for other components
	return queryResult
}

// applyFilters applies stack pattern and column limits to the values.
func applyFilters(extractedValues map[string]interface{}, stackPattern string, maxColumns int) (map[string]interface{}, error) {
	// Apply stack pattern filter
	filteredByPattern, err := filterByStackPattern(extractedValues, stackPattern)
	if err != nil {
		return nil, err
	}

	// Apply column limit
	return limitColumns(filteredByPattern, maxColumns), nil
}

// filterByStackPattern filters values by a glob pattern.
func filterByStackPattern(values map[string]interface{}, pattern string) (map[string]interface{}, error) {
	if pattern == "" {
		return values, nil
	}

	filteredValues := make(map[string]interface{})
	for stackName, value := range values {
		matched, err := filepath.Match(pattern, stackName)
		if err != nil {
			return nil, errors.Errorf("invalid stack pattern '%s'", pattern)
		}
		if matched {
			filteredValues[stackName] = value
		}
	}
	return filteredValues, nil
}

// limitColumns limits the number of columns in the output.
func limitColumns(values map[string]interface{}, maxColumns int) map[string]interface{} {
	if maxColumns <= 0 {
		return values
	}

	limitedValues := make(map[string]interface{})
	var sortedKeys []string
	for stackName := range values {
		sortedKeys = append(sortedKeys, stackName)
	}
	sort.Strings(sortedKeys)

	count := len(sortedKeys)
	if count > maxColumns {
		count = maxColumns
	}

	for i := 0; i < count; i++ {
		limitedValues[sortedKeys[i]] = values[sortedKeys[i]]
	}
	return limitedValues
}

// applyQuery applies a query to the filtered values.
func applyQuery(filteredValues map[string]interface{}, query string, component string) (map[string]interface{}, error) {
	if query == "" {
		return filteredValues, nil
	}

	results := make(map[string]interface{})

	for stackName, stackData := range filteredValues {
		// Apply YQ expression directly to the data
		queryResult, err := utils.EvaluateYqExpression(nil, stackData, query)
		if err != nil {
			log.Debug("YQ query failed",
				KeyStack, stackName,
				"query", query,
				"error", err)
			continue // Skip this stack if query fails
		}

		if queryResult == nil {
			log.Debug("query returned nil", KeyStack, stackName, "query", query)
			continue
		}

		// Format the result for display
		formattedResult := formatResultForDisplay(queryResult, query)
		if formattedResult != nil {
			results[stackName] = formattedResult
		}
	}

	if len(results) == 0 {
		return nil, &listerrors.NoValuesFoundError{Component: component, Query: query}
	}

	return results, nil
}

// formatResultForDisplay formats query results for display.
func formatResultForDisplay(result interface{}, query string) interface{} {
	// Handle different result types
	switch v := result.(type) {
	case map[string]interface{}:
		// Maps can be added directly if not empty
		if len(v) > 0 {
			// For nested maps from a query, wrap them in a value key
			// This is needed for test compatibility
			if query != "" && !strings.HasPrefix(query, ".vars") {
				return map[string]interface{}{
					KeyValue: v,
				}
			}
			return v
		}
		return nil

	case []interface{}:
		// For arrays, convert to string representation for display
		if len(v) > 0 {
			// If it's a simple array of primitives, format as a string
			arrayStr := fmt.Sprintf("%v", v)
			return map[string]interface{}{
				KeyValue: arrayStr,
			}
		}
		return nil

	case string, int, int32, int64, float32, float64, bool:
		// For scalar values, wrap them in a map with "value" key
		return map[string]interface{}{
			KeyValue: v,
		}

	case nil:
		// Skip nil results
		return nil

	default:
		// For any other types, wrap them like scalar values
		return map[string]interface{}{
			KeyValue: v,
		}
	}
}

// formatOutput formats the output based on the specified format.
func formatOutput(values map[string]interface{}, formatStr, delimiter string, maxColumns int) (string, error) {
	formatter, err := format.NewFormatter(format.Format(formatStr))
	if err != nil {
		return "", err
	}

	options := format.FormatOptions{
		MaxColumns: maxColumns,
		Delimiter:  delimiter,
		TTY:        term.IsTTYSupportForStdout(),
		Format:     format.Format(formatStr),
	}

	return formatter.Format(values, options)
}

// IsNoValuesFoundError checks if an error is a NoValuesFoundError.
func IsNoValuesFoundError(err error) bool {
	_, ok := err.(*listerrors.NoValuesFoundError)
	return ok
}

// ValidateFormat validates the output format.
func ValidateValuesFormat(formatStr string) error {
	return format.ValidateFormat(formatStr)
}
