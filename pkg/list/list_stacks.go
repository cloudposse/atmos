package list

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/utils"
	"github.com/samber/lo"
)

type StackData struct {
	Name string
	Path string
}

func FilterAndListStacks(stacksMap map[string]any, component string) ([]string, error) {
	if component != "" {
		// Filter stacks by component
		filteredStacks := []string{}
		for stackName, stackData := range stacksMap {
			v2, ok := stackData.(map[string]any)
			if !ok {
				continue
			}
			components, ok := v2["components"].(map[string]any)
			if !ok {
				continue
			}
			terraform, ok := components["terraform"].(map[string]any)
			if !ok {
				continue
			}
			if _, exists := terraform[component]; exists {
				filteredStacks = append(filteredStacks, stackName)
			}
		}

		if len(filteredStacks) == 0 {
			return nil, nil
		}
		sort.Strings(filteredStacks)
		return filteredStacks, nil
	}

	// List all stacks
	stacks := lo.Keys(stacksMap)
	sort.Strings(stacks)
	return stacks, nil
}

func FilterAndListStacksWithColumns(stacksMap map[string]any, component string, listConfig schema.ListConfig, format string, delimiter string, atmosConfig schema.AtmosConfiguration) (string, error) {
	if err := ValidateFormat(format); err != nil {
		return "", err
	}

	if format == "" && listConfig.Format != "" {
		if err := ValidateFormat(listConfig.Format); err != nil {
			return "", err
		}
		format = listConfig.Format
	}

	stackNames, err := FilterAndListStacks(stacksMap, component)
	if err != nil {
		return "", err
	}

	if len(stackNames) == 0 {
		return "No stacks found", nil
	}

	var stackDatas []StackData
	for _, name := range stackNames {
		stackPath := filepath.Join(atmosConfig.Stacks.BasePath, name+".yaml")
		stackData := StackData{
			Name: name,
			Path: stackPath,
		}
		stackDatas = append(stackDatas, stackData)
	}

	columns := GetColumnsWithDefaults(listConfig.Columns, "stacks")

	var rows [][]string
	for _, stack := range stackDatas {
		templateData := map[string]interface{}{
			"stack_name": stack.Name,
			"stack_path": stack.Path,
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
		for _, stack := range stackDatas {
			templateData := map[string]interface{}{
				"stack_name": stack.Name,
				"stack_path": stack.Path,
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
		return formatDelimitedOutputWithColumns(map[string]interface{}{"stacks": rows}, columns, delimiter)

	case "table":
		return formatTableOutputWithColumns(map[string]interface{}{"stacks": rows}, columns)

	default:
		var output strings.Builder
		for i, name := range stackNames {
			output.WriteString(name)
			if i < len(stackNames)-1 {
				output.WriteString(utils.GetLineEnding())
			}
		}
		return output.String(), nil
	}
}
