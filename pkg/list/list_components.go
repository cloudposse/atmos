package list

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/utils"
	"github.com/samber/lo"
)

// ComponentData represents component information for template processing
type ComponentData struct {
	Name string
	Type string
	Path string
}

// Error definitions for component listing.
var (
	// ErrParseStacks is returned when stack data cannot be parsed.
	ErrParseStacks = errors.New("could not parse stacks")
	// ErrParseComponents is returned when component data cannot be parsed.
	ErrParseComponents = errors.New("could not parse components")
	// ErrParseTerraformComponents is returned when terraform component data cannot be parsed.
	ErrParseTerraformComponents = errors.New("could not parse Terraform components")
	// ErrStackNotFound is returned when a requested stack is not found.
	ErrStackNotFound = errors.New("stack not found")
	// ErrProcessStack is returned when there's an error processing a stack.
	ErrProcessStack = errors.New("error processing stack")
)

// getStackComponents extracts Terraform components from the final map of stacks.
func getStackComponents(stackData any) ([]string, error) {
	stackMap, ok := stackData.(map[string]any)
	if !ok {
		return nil, ErrParseStacks
	}

	componentsMap, ok := stackMap["components"].(map[string]any)
	if !ok {
		return nil, ErrParseComponents
	}

	terraformComponents, ok := componentsMap["terraform"].(map[string]any)
	if !ok {
		return nil, ErrParseTerraformComponents
	}

	return lo.Keys(terraformComponents), nil
}

// getComponentsForSpecificStack extracts components from a specific stack.
func getComponentsForSpecificStack(stackName string, stacksMap map[string]any) ([]string, error) {
	// Verify stack exists.
	stackData, ok := stacksMap[stackName]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrStackNotFound, stackName)
	}

	// Get components for the specific stack.
	stackComponents, err := getStackComponents(stackData)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrProcessStack, stackName, err)
	}

	return stackComponents, nil
}

// processAllStacks collects components from all valid stacks.
func processAllStacks(stacksMap map[string]any) []string {
	var components []string
	for _, stackData := range stacksMap {
		stackComponents, err := getStackComponents(stackData)
		if err != nil {
			continue // Skip invalid stacks.
		}
		components = append(components, stackComponents...)
	}
	return components
}

// FilterAndListComponents filters and lists components based on the given stack.
func FilterAndListComponents(stackFlag string, stacksMap map[string]any) ([]string, error) {
	var components []string
	if stacksMap == nil {
		return nil, fmt.Errorf("%w: %s", ErrStackNotFound, stackFlag)
	}

	// Handle specific stack case.
	if stackFlag != "" {
		stackComponents, err := getComponentsForSpecificStack(stackFlag, stacksMap)
		if err != nil {
			return nil, err
		}
		components = stackComponents
	} else {
		// Process all stacks.
		components = processAllStacks(stacksMap)
	}

	// Remove duplicates and sort components.
	components = lo.Uniq(components)
	sort.Strings(components)

	if len(components) == 0 {
		return []string{}, nil
	}
	return components, nil
}

// FilterAndListComponentsWithColumns filters and lists components with custom column support
func FilterAndListComponentsWithColumns(stackFlag string, stacksMap map[string]any, listConfig schema.ListConfig, format string, delimiter string, atmosConfig schema.AtmosConfiguration) (string, error) {
	if err := ValidateFormat(format); err != nil {
		return "", err
	}

	if format == "" && listConfig.Format != "" {
		if err := ValidateFormat(listConfig.Format); err != nil {
			return "", err
		}
		format = listConfig.Format
	}

	componentNames, err := FilterAndListComponents(stackFlag, stacksMap)
	if err != nil {
		return "", err
	}

	if len(componentNames) == 0 {
		return "No components found", nil
	}

	var componentDatas []ComponentData
	for _, name := range componentNames {
		componentData := ComponentData{
			Name: name,
			Type: "terraform",
			Path: filepath.Join(atmosConfig.Components.Terraform.BasePath, name),
		}
		componentDatas = append(componentDatas, componentData)
	}

	columns := GetColumnsWithDefaults(listConfig.Columns, "components")

	var rows [][]string
	for _, component := range componentDatas {
		templateData := map[string]interface{}{
			"component_name": component.Name,
			"component_type": component.Type,
			"component_path": component.Path,
		}
		
		var row []string
		for _, col := range columns {
			value, err := ProcessColumnTemplate(col.Value, templateData)
			if err != nil {
				value = ""
			}
			row = append(row, value)
		}
		rows = append(rows, row)
	}

	switch format {
	case "json":
		var jsonData []map[string]interface{}
		for _, component := range componentDatas {
			templateData := map[string]interface{}{
				"component_name": component.Name,
				"component_type": component.Type,
				"component_path": component.Path,
			}
			
			row, err := ProcessCustomColumns(columns, templateData)
			if err == nil {
				jsonData = append(jsonData, row)
			}
		}
		jsonBytes, err := json.MarshalIndent(jsonData, "", "  ")
		if err != nil {
			return "", fmt.Errorf("error formatting JSON output: %w", err)
		}
		return string(jsonBytes), nil

	case "csv", "tsv":
		if format == "tsv" {
			delimiter = "\t"
		}
		return formatDelimitedOutputWithColumns(map[string]interface{}{"components": rows}, columns, delimiter)

	case "table":
		return formatTableOutputWithColumns(map[string]interface{}{"components": rows}, columns)
	
	case "":
		if len(listConfig.Columns) == 0 {
			var output strings.Builder
			for i, name := range componentNames {
				output.WriteString(name)
				if i < len(componentNames)-1 {
					output.WriteString(utils.GetLineEnding())
				}
			}
			return output.String(), nil
		}
		return formatTableOutputWithColumns(map[string]interface{}{"components": rows}, columns)

	default:
		var output strings.Builder
		for i, name := range componentNames {
			output.WriteString(name)
			if i < len(componentNames)-1 {
				output.WriteString(utils.GetLineEnding())
			}
		}
		return output.String(), nil
	}
}
