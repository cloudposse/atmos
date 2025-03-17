package values

import (
	"fmt"
	"strings"

	log "github.com/charmbracelet/log"

	"github.com/cloudposse/atmos/pkg/list/errors"
	"github.com/cloudposse/atmos/pkg/utils"
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
)

// ExtractStackValues implements the ValueExtractor interface for DefaultExtractor.
// Uses YQ expressions to extract component values from stacks.
func (e *DefaultExtractor) ExtractStackValues(stacksMap map[string]interface{}, component string, includeAbstract bool) (map[string]interface{}, error) {
	values := make(map[string]interface{})

	for stackName, stackData := range stacksMap {
		stack, ok := stackData.(map[string]interface{})
		if !ok {
			log.Debug("stack data is not a map", "stack", stackName)
			continue
		}

		var yqExpression string
		var queryResult interface{}
		var err error

		switch component {
		case KeySettings:
			yqExpression = "select(.settings // .terraform.settings // .components.terraform.*.settings)"
			queryResult, err = utils.EvaluateYqExpression(nil, stack, yqExpression)
			if err == nil && queryResult != nil {
				values[stackName] = queryResult
			} else {
				log.Debug("no settings found", "stack", stackName, "error", err)
			}
			continue

		case KeyMetadata:
			yqExpression = ".metadata"
			queryResult, err = utils.EvaluateYqExpression(nil, stack, yqExpression)
			if err == nil && queryResult != nil {
				values[stackName] = queryResult
			} else {
				log.Debug("no metadata found", "stack", stackName, "error", err)
			}
			continue

		default:
			// Extract the component name from the full component path
			componentName := component
			parts := strings.Split(component, "/")
			if len(parts) > 1 {
				componentName = parts[len(parts)-1]
			}

			// Build query for component vars
			yqExpression = fmt.Sprintf(".components.%s.%s", KeyTerraform, componentName)

			// If not including abstract components, filter them out
			if !includeAbstract {
				// Only get component that either doesn't have abstract flag or has it set to false
				yqExpression += " | select(has(\"abstract\") == false or .abstract == false)"
			}

			// Get the vars
			yqExpression += " | .vars"

			queryResult, err = utils.EvaluateYqExpression(nil, stack, yqExpression)
			if err == nil && queryResult != nil {
				values[stackName] = queryResult
			} else {
				log.Debug("no component values found",
					"stack", stackName,
					"component", component,
					"yq_expression", yqExpression,
					"error", err)
			}
		}
	}

	if len(values) == 0 {
		return nil, &errors.NoValuesFoundError{Component: component}
	}

	return values, nil
}

// ApplyValueQuery implements the ValueExtractor interface for DefaultExtractor.
// It uses YQ expressions to query the extracted values.
func (e *DefaultExtractor) ApplyValueQuery(values map[string]interface{}, query string) (map[string]interface{}, error) {
	if query == "" {
		return values, nil
	}

	results := make(map[string]interface{})

	for stackName, stackData := range values {
		// Apply YQ expression directly to the data
		queryResult, err := utils.EvaluateYqExpression(nil, stackData, query)
		if err != nil {
			log.Debug("YQ query failed",
				"stack", stackName,
				"query", query,
				"error", err)
			continue // Skip this stack if query fails
		}

		// Handle the query result based on its type
		switch result := queryResult.(type) {
		case map[string]interface{}:
			// Maps can be added directly
			results[stackName] = result
		case []interface{}:
			// Arrays should be added directly
			results[stackName] = result
		case string, int, int32, int64, float32, float64, bool:
			// Scalar values need to be wrapped in a map to display correctly in tables
			// Use the last part of the query as the key (e.g., .location -> location)
			key := strings.TrimPrefix(query, ".")
			if strings.Contains(key, ".") {
				parts := strings.Split(key, ".")
				key = parts[len(parts)-1]
			}
			// Create a map with the value using the key from query
			results[stackName] = map[string]interface{}{
				key: result,
			}
		case nil:
			// Skip nil results
			log.Debug("query returned nil", "stack", stackName, "query", query)
		default:
			// For any other types, wrap them like scalar values
			key := strings.TrimPrefix(query, ".")
			if strings.Contains(key, ".") {
				parts := strings.Split(key, ".")
				key = parts[len(parts)-1]
			}
			results[stackName] = map[string]interface{}{
				key: result,
			}
		}
	}

	if len(results) == 0 {
		return nil, &errors.NoValuesFoundError{Query: query}
	}

	return results, nil
}
