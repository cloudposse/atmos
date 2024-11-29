package exec

import (
	"strings"

	"github.com/cloudposse/atmos/pkg/schema"
)

func ProcessCustomYamlTags(input schema.AtmosSectionMapType) (schema.AtmosSectionMapType, error) {
	return processNodes(input), nil
}

func processNodes(data map[string]any) map[string]any {
	newMap := make(map[string]any)
	var recurse func(any) any

	recurse = func(node any) any {
		switch v := node.(type) {
		case string:
			return processCustomTags(v)

		case map[string]any:
			newNestedMap := make(map[string]any)
			for k, val := range v {
				newNestedMap[k] = recurse(val)
			}
			return newNestedMap

		case []any:
			newSlice := make([]any, len(v))
			for i, val := range v {
				newSlice[i] = recurse(val)
			}
			return newSlice

		default:
			return v
		}
	}

	for k, v := range data {
		newMap[k] = recurse(v)
	}

	return newMap
}

func processCustomTags(input string) any {
	if strings.HasPrefix(input, "!template") {
		return processTemplateTag(input)
	} else if strings.HasPrefix(input, "!exec") {
		return processExecTag(input)
	} else if strings.HasPrefix(input, "!terraform.output") {
		return processTerraformOutputTag(input)
	}
	return input
}
