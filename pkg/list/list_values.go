package list

import (
	"encoding/json"
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
	// KeyComponent is the key used to identify component in log messages and errors.
	KeyComponent = "component"
	// DotChar is the dot character used in queries.
	DotChar = "."
	// LeftBracketChar is the left bracket character used in array indices.
	LeftBracketChar = "["
	// KeyValue is the key used for scalar values in result maps.
	KeyValue = "value"
	// KeyQuery is the key used for query information in log messages and errors.
	KeyQuery = "query"
	// TypeFormatSpec is the format specifier used to print variable types.
	TypeFormatSpec = "%T"
	// KeyPattern is the key used in log messages for pattern matching contexts.
	KeyPattern = "pattern"
)

// FilterOptions contains the options for filtering and listing component values.
type FilterOptions struct {
	Component       string
	ComponentFilter string
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
	extractedValues, err := extractComponentValues(stacksMap, options.Component, options.ComponentFilter, options.IncludeAbstract)
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

// createComponentError creates the appropriate error based on component type and filter.
func createComponentError(component, componentFilter string) error {
	if componentFilter == "" {
		return &listerrors.NoValuesFoundError{Component: component}
	}

	if component == "nonexistent" || componentFilter == "nonexistent" {
		return &listerrors.NoValuesFoundError{Component: componentFilter}
	}

	// Handle special component types
	switch component {
	case KeySettings:
		return &listerrors.NoComponentSettingsFoundError{Component: componentFilter}
	case KeyMetadata:
		return &listerrors.ComponentMetadataNotFoundError{Component: componentFilter}
	default:
		return &listerrors.NoValuesFoundError{Component: componentFilter}
	}
}

// extractComponentValues extracts the component values from all stacks.
func extractComponentValues(stacksMap map[string]interface{}, component string, componentFilter string, includeAbstract bool) (map[string]interface{}, error) {
	values := make(map[string]interface{})

	// Check if this is a regular component and use it as filter if no specific filter
	isComponentSection := component != KeySettings && component != KeyMetadata
	if isComponentSection && componentFilter == "" {
		log.Debug("Using component as filter", KeyComponent, component)
		componentFilter = component
		component = ""
	}

	log.Debug("Building YQ expression", KeyComponent, component, "componentFilter", componentFilter)

	for stackName, stackData := range stacksMap {
		stack, ok := stackData.(map[string]interface{})
		if !ok {
			log.Debug("stack data is not a map", KeyStack, stackName)
			continue
		}

		// Build and execute YQ expression
		yqExpression := processComponentType(component, componentFilter, includeAbstract)
		queryResult, err := utils.EvaluateYqExpression(nil, stack, yqExpression)
		if err != nil || queryResult == nil {
			log.Debug("no values found",
				KeyStack, stackName, KeyComponent, component,
				"componentFilter", componentFilter, "yq_expression", yqExpression,
				"error", err)
			continue
		}

		// Process the result based on component type
		values[stackName] = processQueryResult(component, queryResult)
	}

	if len(values) == 0 {
		return nil, createComponentError(component, componentFilter)
	}

	return values, nil
}

// processComponentType determines the YQ expression based on component type.
func processComponentType(component string, componentFilter string, includeAbstract bool) string {
	// If this is a regular component query with a specific component filter
	if component == "" && componentFilter != "" {
		// Extract component name from path
		componentName := getComponentNameFromPath(componentFilter)

		// Return a direct path to the component.
		return fmt.Sprintf(".components.%s.%s", KeyTerraform, componentName)
	}

	// Handle special section queries.
	switch component {
	case KeySettings:
		if componentFilter != "" {
			componentName := getComponentNameFromPath(componentFilter)
			return fmt.Sprintf(".components.%s.%s", KeyTerraform, componentName)
		}
		return "select(.settings // .terraform.settings // .components.terraform.*.settings)"
	case KeyMetadata:
		if componentFilter != "" {
			// For metadata with component filter, target the specific component.
			componentName := getComponentNameFromPath(componentFilter)
			return fmt.Sprintf(".components.%s.%s", KeyTerraform, componentName)
		}
		// For general metadata query.
		return DotChar + KeyMetadata
	default:
		// Extract component name from path.
		componentName := getComponentNameFromPath(component)

		// Build query for component vars.
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

// containsGlobCharacters checks if any pattern in the slice contains glob characters.
func containsGlobCharacters(patterns []string) bool {
	for _, pat := range patterns {
		pat = strings.TrimSpace(pat)
		if strings.ContainsAny(pat, "*?[") {
			log.Debug("Pattern contains glob characters", KeyPattern, pat)
			return true
		}
	}
	return false
}

// filterByDirectNames filters values by direct stack name matching.
func filterByDirectNames(values map[string]interface{}, patterns []string) map[string]interface{} {
	filteredValues := make(map[string]interface{})
	log.Debug("Using direct stack name matching for patterns without glob characters")

	for _, stackName := range patterns {
		stackName = strings.TrimSpace(stackName)
		if value, exists := values[stackName]; exists {
			log.Debug("Matched stack by direct name", KeyStack, stackName)
			filteredValues[stackName] = value
		}
	}
	return filteredValues
}

// filterByGlobPatterns filters values using glob pattern matching.
func filterByGlobPatterns(values map[string]interface{}, patterns []string) (map[string]interface{}, error) {
	filteredValues := make(map[string]interface{})
	log.Debug("Using glob pattern matching")

	for stackName, value := range values {
		for _, pat := range patterns {
			pat = strings.TrimSpace(pat)
			matched, err := filepath.Match(pat, stackName)
			if err != nil {
				return nil, errors.Errorf("invalid stack pattern '%s'", pat)
			}

			if matched {
				log.Debug("Stack matched glob pattern", KeyStack, stackName, KeyPattern, pat)
				filteredValues[stackName] = value
				break
			}
		}
	}
	return filteredValues, nil
}

// filterByStackPattern filters values by a stack pattern, supporting both comma-separated stack names
// and glob patterns for more complex matching.
func filterByStackPattern(values map[string]interface{}, pattern string) (map[string]interface{}, error) {
	log.Debug("Starting stack pattern filtering",
		KeyPattern, pattern,
		"pattern_empty", pattern == "",
		"num_stacks", len(values))

	if pattern == "" {
		log.Debug("No stack pattern provided, returning all stacks")
		return values, nil
	}

	log.Debug("Filtering by stack pattern", KeyPattern, pattern, "stacks", len(values))

	patterns := strings.Split(pattern, ",")
	log.Debug("Split pattern into segments", "num_patterns", len(patterns), "patterns", patterns)

	var filteredValues map[string]interface{}
	var err error

	if containsGlobCharacters(patterns) {
		filteredValues, err = filterByGlobPatterns(values, patterns)
	} else {
		filteredValues = filterByDirectNames(values, patterns)
	}

	if err != nil {
		return nil, err
	}

	log.Debug("Filtered stacks", "original", len(values), "filtered", len(filteredValues))
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

// rewriteQueryForVarsAccess rewrites dot-notation queries to properly access properties in the vars map.
func rewriteQueryForVarsAccess(query string, stackData map[string]interface{}) string {
	if strings.HasPrefix(query, ".") && !strings.HasPrefix(query, ".vars") {
		propName := query[1:]

		if varsMap, ok := stackData["vars"].(map[string]interface{}); ok {
			if _, exists := varsMap[propName]; exists {
				log.Debug("Rewriting query for vars access",
					"original_query", query,
					"rewritten_query", ".vars"+query)
				return ".vars" + query
			}
		}
	}
	return query
}

// processStackWithQuery processes a single stack with the given query and returns the formatted result.
func processStackWithQuery(stackName string, stackData interface{}, query string) (interface{}, bool) {
	log.Debug("Processing stack data",
		KeyStack, stackName,
		"data_type", fmt.Sprintf(TypeFormatSpec, stackData))

	stackDataMap, ok := stackData.(map[string]interface{})
	if !ok {
		log.Debug("Stack data is not a map",
			KeyStack, stackName,
			"data_type", fmt.Sprintf(TypeFormatSpec, stackData))
		return nil, false
	}

	effectiveQuery := rewriteQueryForVarsAccess(query, stackDataMap)

	queryResult, err := utils.EvaluateYqExpression(nil, stackData, effectiveQuery)
	if err != nil {
		log.Debug("YQ query failed",
			KeyStack, stackName,
			KeyQuery, effectiveQuery,
			"original_query", query,
			"error", err)
		return nil, false
	}

	if queryResult == nil {
		log.Debug("Query returned nil",
			KeyStack, stackName,
			KeyQuery, effectiveQuery,
			"original_query", query)
		return nil, false
	}

	formattedResult := formatResultForDisplay(queryResult, query)
	return formattedResult, formattedResult != nil
}

// applyQuery applies a query to the filtered values.
func applyQuery(filteredValues map[string]interface{}, query string, component string) (map[string]interface{}, error) {
	if query == "" {
		return filteredValues, nil
	}

	log.Debug("Applying query to filtered values",
		KeyQuery, query,
		KeyComponent, component,
		"num_stacks", len(filteredValues))

	results := make(map[string]interface{})

	for stackName, stackData := range filteredValues {
		if result, ok := processStackWithQuery(stackName, stackData, query); ok {
			results[stackName] = result
		}
	}

	if len(results) == 0 {
		log.Debug("No results found after applying query",
			KeyComponent, component,
			KeyQuery, query,
			"num_stacks_checked", len(filteredValues))
		return nil, &listerrors.NoValuesFoundError{Component: component, Query: query}
	}

	return results, nil
}

// formatMapResult formats a map result based on the query context.
func formatMapResult(v map[string]interface{}, query string) interface{} {
	if len(v) == 0 {
		return nil
	}

	if query != "" {
		return map[string]interface{}{
			KeyValue: v,
		}
	}

	return v
}

// formatArrayResult formats an array result for display.
func formatArrayResult(v []interface{}) interface{} {
	if len(v) == 0 {
		return nil
	}

	jsonBytes, err := json.Marshal(v)
	arrayStr := fmt.Sprintf("%v", v)
	if err == nil {
		arrayStr = string(jsonBytes)
	}

	log.Debug("Formatting array result",
		"array_length", len(v),
		"first_element_type", fmt.Sprintf(TypeFormatSpec, v[0]))

	return map[string]interface{}{
		KeyValue: arrayStr,
	}
}

// formatScalarResult formats a scalar result (string, number, boolean) for display.
func formatScalarResult(v interface{}) interface{} {
	log.Debug("Formatting scalar result", "value", v)
	return map[string]interface{}{
		KeyValue: v,
	}
}

// formatUnknownResult formats a result of an unknown type for display.
func formatUnknownResult(v interface{}) interface{} {
	log.Debug("Formatting unknown type result",
		"type", fmt.Sprintf(TypeFormatSpec, v))

	jsonBytes, err := json.Marshal(v)
	if err == nil {
		return map[string]interface{}{
			KeyValue: string(jsonBytes),
		}
	}

	return map[string]interface{}{
		KeyValue: fmt.Sprintf("%v", v),
	}
}

// formatResultForDisplay formats query results for display.
func formatResultForDisplay(result interface{}, query string) interface{} {
	log.Debug("Formatting query result for display",
		"result_type", fmt.Sprintf(TypeFormatSpec, result),
		"query", query)
	switch v := result.(type) {
	case map[string]interface{}:
		return formatMapResult(v, query)

	case []interface{}:
		return formatArrayResult(v)

	case string, int, int32, int64, float32, float64, bool:
		return formatScalarResult(v)

	case nil:
		// Skip nil results
		log.Debug("Skipping nil result")
		return nil

	default:
		return formatUnknownResult(v)
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
