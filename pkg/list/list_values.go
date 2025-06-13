package list

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	log "github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/internal/tui/templates/term"
	listerrors "github.com/cloudposse/atmos/pkg/list/errors"
	listformat "github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/utils"
	"github.com/pkg/errors"
)

// Error variables for list_values package.
var (
	ErrInvalidStackPattern         = errors.New("invalid stack pattern")
	ErrEmptyTargetComponentName    = errors.New("target component name cannot be empty")
	ErrComponentsSectionNotFound   = errors.New("components section not found in stack")
	ErrComponentNotFoundInSections = errors.New("component not found in terraform or helmfile sections")
	ErrQueryFailed                 = errors.New("query execution failed")
)

// Component and section name constants.
const (
	// KeyTerraform is the key for terraform components.
	KeyTerraform = "terraform"
	// KeyHelmfile is the key for helmfile components.
	KeyHelmfile = "helmfile"
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
		options.FormatStr = string(listformat.FormatTable)
	}

	if err := listformat.ValidateFormat(options.FormatStr); err != nil {
		return "", err
	}

	// Extract stack values
	extractedValues, err := extractComponentValuesFromAllStacks(stacksMap, options.Component, options.ComponentFilter, options.IncludeAbstract)
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

func extractComponentValuesFromAllStacks(stacks map[string]interface{}, component, filter string, includeAbstract bool) (map[string]interface{}, error) {
	stackComponentValues := make(map[string]interface{})

	component, filter = normalizeComponentAndFilterInputs(component, filter)

	for stackName, data := range stacks {
		stackMap, ok := data.(map[string]interface{})
		if !ok {
			continue
		}

		componentValue := extractComponentValueFromSingleStack(stackMap, stackName, component, filter, includeAbstract)
		if componentValue != nil {
			stackComponentValues[stackName] = componentValue
		}
	}

	if len(stackComponentValues) == 0 {
		return nil, createComponentError(component, filter)
	}

	return stackComponentValues, nil
}

func normalizeComponentAndFilterInputs(component, filter string) (string, string) {
	isRegularComponent := component != KeySettings && component != KeyMetadata
	if isRegularComponent && filter == "" {
		log.Debug("Using component name as filter", KeyComponent, component)
		return "", component
	}
	return component, filter
}

func extractComponentValueFromSingleStack(stackMap map[string]interface{}, stackName, component, filter string, includeAbstract bool) interface{} {
	targetComponentName := determineTargetComponentName(component, filter)

	componentType := detectComponentTypeInStack(stackMap, targetComponentName, stackName)
	if componentType == "" {
		return nil
	}

	params := &QueryParams{
		StackName:           stackName,
		StackMap:            stackMap,
		Component:           component,
		ComponentFilter:     filter,
		TargetComponentName: targetComponentName,
		ComponentType:       componentType,
		IncludeAbstract:     includeAbstract,
	}

	value, err := executeQueryForStack(params)
	if err != nil {
		log.Warn("Query failed", KeyStack, stackName, "error", err)
		return nil
	}

	return value
}

func detectComponentTypeInStack(stackMap map[string]interface{}, targetComponent, stackName string) string {
	if targetComponent == "" {
		return KeyTerraform
	}

	detectedType, err := determineComponentType(stackMap, targetComponent)
	if err != nil {
		log.Debug("Component not found", KeyStack, stackName, KeyComponent, targetComponent)
		return ""
	}

	return detectedType
}

// QueryParams holds all parameters needed for executing a query on a stack.
type QueryParams struct {
	StackName           string
	StackMap            map[string]interface{}
	Component           string
	ComponentFilter     string
	TargetComponentName string
	ComponentType       string
	IncludeAbstract     bool
}

func executeQueryForStack(params *QueryParams) (interface{}, error) {
	yqExpression := buildYqExpressionForComponent(
		params.Component,
		params.ComponentFilter,
		params.IncludeAbstract,
		params.ComponentType,
	)

	queryResult, err := utils.EvaluateYqExpression(nil, params.StackMap, yqExpression)
	if err != nil {
		var logKey string
		var logValue string
		if params.TargetComponentName != "" {
			logKey = KeyComponent
			logValue = params.TargetComponentName
		} else {
			logKey = "section"
			logValue = params.Component
		}

		log.Warn("YQ evaluation failed",
			KeyStack, params.StackName,
			"yqExpression", yqExpression,
			logKey, logValue,
			"error", err)
		return nil, fmt.Errorf("%w: %s", ErrQueryFailed, err.Error())
	}

	if queryResult == nil {
		return nil, nil
	}

	return extractRelevantDataFromQueryResult(params.Component, queryResult), nil
}

func determineTargetComponentName(component, componentFilter string) string {
	if componentFilter != "" {
		return componentFilter
	}

	isRegularComponent := component != KeySettings && component != KeyMetadata
	if isRegularComponent {
		return component
	}

	return ""
}

func determineComponentType(stack map[string]interface{}, targetComponentName string) (string, error) {
	if targetComponentName == "" {
		return "", ErrEmptyTargetComponentName
	}

	components, ok := stack[KeyComponents].(map[string]interface{})
	if !ok {
		return "", ErrComponentsSectionNotFound
	}

	if isComponentInSection(components, KeyTerraform, targetComponentName) {
		log.Debug("Component found under terraform", KeyComponent, targetComponentName)
		return KeyTerraform, nil
	}

	if isComponentInSection(components, KeyHelmfile, targetComponentName) {
		log.Debug("Component found under helmfile", KeyComponent, targetComponentName)
		return KeyHelmfile, nil
	}

	return "", fmt.Errorf("%w: %s", ErrComponentNotFoundInSections, targetComponentName)
}

func isComponentInSection(components map[string]interface{}, sectionKey, componentName string) bool {
	section, ok := components[sectionKey].(map[string]interface{})
	if !ok {
		return false
	}
	_, exists := section[componentName]
	return exists
}

func buildYqExpressionForComponent(component string, componentFilter string, includeAbstract bool, componentType string) string {
	if component == "" && componentFilter != "" {
		return fmt.Sprintf(".components.%s.\"%s\"", componentType, componentFilter)
	}

	switch component {
	case KeySettings:
		return buildSettingsExpression(componentFilter, componentType)
	case KeyMetadata:
		return buildMetadataExpression(componentFilter, componentType)
	default:
		return buildComponentYqExpression(component, includeAbstract, componentType)
	}
}

func buildSettingsExpression(componentFilter, componentType string) string {
	if componentFilter != "" {
		return fmt.Sprintf(".components.%s.\"%s\"", componentType, componentFilter)
	}
	return "select(.settings // " +
		".components." + KeyTerraform + ".*.settings // " +
		".components." + KeyHelmfile + ".*.settings)"
}

func buildMetadataExpression(componentFilter, componentType string) string {
	if componentFilter != "" {
		// Use full component path and wrap in quotes for nested support
		return fmt.Sprintf(".components.%s.\"%s\"", componentType, componentFilter)
	}
	return DotChar + KeyMetadata
}

func buildComponentYqExpression(component string, includeAbstract bool, componentType string) string {
	path := fmt.Sprintf("%scomponents%s%s%s\"%s\"", DotChar, DotChar, componentType, DotChar, component)

	if !includeAbstract {
		path += fmt.Sprintf(" | select(has(\"%s\") == false or %s%s == false)",
			KeyAbstract, DotChar, KeyAbstract)
	}

	return path + fmt.Sprintf(" | %s%s", DotChar, KeyVars)
}

func extractRelevantDataFromQueryResult(component string, queryResult interface{}) interface{} {
	if component != KeySettings {
		return queryResult
	}

	settings, ok := queryResult.(map[string]interface{})
	if !ok {
		return queryResult
	}

	settingsContent, ok := settings[KeySettings].(map[string]interface{})
	if !ok {
		return queryResult
	}

	return settingsContent
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

	formattedResult := formatResultForDisplay(queryResult)
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

// formatResultForDisplay formats query results for display.
func formatResultForDisplay(result interface{}) interface{} {
	if result == nil {
		return nil
	}
	return result
}

// formatOutput formats the output based on the specified format.
func formatOutput(values map[string]interface{}, formatStr, delimiter string, maxColumns int) (string, error) {
	formatter, err := listformat.NewFormatter(listformat.Format(formatStr))
	if err != nil {
		return "", err
	}

	options := listformat.FormatOptions{
		MaxColumns: maxColumns,
		Delimiter:  delimiter,
		TTY:        term.IsTTYSupportForStdout(),
		Format:     listformat.Format(formatStr),
	}

	return formatter.Format(values, options)
}

// ValidateFormat validates the output format.
func ValidateValuesFormat(formatStr string) error {
	return listformat.ValidateFormat(formatStr)
}

// FilterAndListValuesWithColumns filters and lists component values with custom column support
func FilterAndListValuesWithColumns(stacksMap map[string]interface{}, options *FilterOptions, listConfig schema.ListConfig) (string, error) {
	if options == nil {
		return "", errors.New("options cannot be nil")
	}

	if options.FormatStr == "" {
		if listConfig.Format != "" {
			options.FormatStr = listConfig.Format
		} else {
			options.FormatStr = string(listformat.FormatTable)
		}
	}

	if err := listformat.ValidateFormat(options.FormatStr); err != nil {
		return "", err
	}

	extractedValues, err := extractComponentValuesFromAllStacks(stacksMap, options.Component, options.ComponentFilter, options.IncludeAbstract)
	if err != nil {
		return "", err
	}

	filteredValues, err := applyFilters(extractedValues, options.StackPattern, options.MaxColumns)
	if err != nil {
		return "", err
	}

	queriedValues, err := applyQuery(filteredValues, options.Query, options.Component)
	if err != nil {
		return "", err
	}

	columns := GetColumnsWithDefaults(listConfig.Columns, "values")

	return formatOutputWithColumns(queriedValues, columns, options.FormatStr, options.Delimiter, options.MaxColumns)
}

// formatOutputWithColumns formats the output with custom column support
func formatOutputWithColumns(values map[string]interface{}, columns []schema.ListColumnConfig, formatStr, delimiter string, maxColumns int) (string, error) {
	switch formatStr {
	case "json":
		return formatJSONOutputWithColumns(values, columns)
	case "csv", "tsv":
		if formatStr == "tsv" {
			delimiter = "\t"
		}
		return formatDelimitedOutputWithColumns(values, columns, delimiter)
	case "table", "":
		return formatTableOutputWithColumns(values, columns)
	default:
		return formatOutput(values, formatStr, delimiter, maxColumns)
	}
}

// formatJSONOutputWithColumns formats values as JSON with custom columns
func formatJSONOutputWithColumns(values map[string]interface{}, columns []schema.ListColumnConfig) (string, error) {
	var jsonData []map[string]interface{}

	var stackNames []string
	for stackName := range values {
		stackNames = append(stackNames, stackName)
	}
	sort.Strings(stackNames)

	for _, stackName := range stackNames {
		stackData := values[stackName]

		switch v := stackData.(type) {
		case map[string]interface{}:
			var keys []string
			for key := range v {
				keys = append(keys, key)
			}
			sort.Strings(keys)

			for _, key := range keys {
				value := v[key]
				templateData := map[string]interface{}{
					"stack_name": stackName,
					"key":        key,
					"value":      value,
				}
				row, err := ProcessCustomColumns(columns, templateData)
				if err == nil {
					jsonData = append(jsonData, row)
				}
			}
		default:
			templateData := map[string]interface{}{
				"stack_name": stackName,
				"value":      stackData,
			}
			row, err := ProcessCustomColumns(columns, templateData)
			if err == nil {
				jsonData = append(jsonData, row)
			}
		}
	}

	jsonBytes, err := json.MarshalIndent(jsonData, "", "  ")
	if err != nil {
		return "", fmt.Errorf("error formatting JSON output: %w", err)
	}
	return string(jsonBytes), nil
}

// formatDelimitedOutputWithColumns formats values as CSV/TSV with custom columns
func formatDelimitedOutputWithColumns(values map[string]interface{}, columns []schema.ListColumnConfig, delimiter string) (string, error) {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	if len(delimiter) > 0 {
		writer.Comma = rune(delimiter[0])
	}

	header := ExtractHeaders(columns)
	if err := writer.Write(header); err != nil {
		return "", fmt.Errorf("error writing header: %w", err)
	}

	var stackNames []string
	for stackName := range values {
		stackNames = append(stackNames, stackName)
	}
	sort.Strings(stackNames)

	for _, stackName := range stackNames {
		stackData := values[stackName]

		switch v := stackData.(type) {
		case map[string]interface{}:
			var keys []string
			for key := range v {
				keys = append(keys, key)
			}
			sort.Strings(keys)

			for _, key := range keys {
				value := v[key]
				templateData := map[string]interface{}{
					"stack_name": stackName,
					"key":        key,
					"value":      value,
				}

				var row []string
				for _, col := range columns {
					val, err := ProcessColumnTemplate(col.Value, templateData)
					if err != nil {
						val = ""
					}
					row = append(row, val)
				}
				if err := writer.Write(row); err != nil {
					return "", fmt.Errorf("error writing row: %w", err)
				}
			}
		default:
			templateData := map[string]interface{}{
				"stack_name": stackName,
				"value":      stackData,
			}

			var row []string
			for _, col := range columns {
				val, err := ProcessColumnTemplate(col.Value, templateData)
				if err != nil {
					val = ""
				}
				row = append(row, val)
			}
			if err := writer.Write(row); err != nil {
				return "", fmt.Errorf("error writing row: %w", err)
			}
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return "", fmt.Errorf("error flushing CSV writer: %w", err)
	}

	return buf.String(), nil
}

// formatTableOutputWithColumns formats values as a table with custom columns
func formatTableOutputWithColumns(values map[string]interface{}, columns []schema.ListColumnConfig) (string, error) {
	header := ExtractHeaders(columns)
	var rows [][]string

	var stackNames []string
	for stackName := range values {
		stackNames = append(stackNames, stackName)
	}
	sort.Strings(stackNames)

	for _, stackName := range stackNames {
		stackData := values[stackName]

		switch v := stackData.(type) {
		case map[string]interface{}:
			var keys []string
			for key := range v {
				keys = append(keys, key)
			}
			sort.Strings(keys)

			for _, key := range keys {
				value := v[key]
				templateData := map[string]interface{}{
					"stack_name": stackName,
					"key":        key,
					"value":      fmt.Sprintf("%v", value),
				}

				var row []string
				for _, col := range columns {
					val, err := ProcessColumnTemplate(col.Value, templateData)
					if err != nil {
						val = ""
					}
					row = append(row, val)
				}
				rows = append(rows, row)
			}
		default:
			templateData := map[string]interface{}{
				"stack_name": stackName,
				"value":      fmt.Sprintf("%v", stackData),
			}

			var row []string
			for _, col := range columns {
				val, err := ProcessColumnTemplate(col.Value, templateData)
				if err != nil {
					val = ""
				}
				row = append(row, val)
			}
			rows = append(rows, row)
		}
	}

	return listformat.CreateStyledTable(header, rows), nil
}
