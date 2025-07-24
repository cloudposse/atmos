package exec

import (
	"fmt"
	"strings"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func ProcessCustomYamlTags(
	atmosConfig *schema.AtmosConfiguration,
	input schema.AtmosSectionMapType,
	currentStack string,
	skip []string,
) (schema.AtmosSectionMapType, error) {
	return processNodes(atmosConfig, input, currentStack, skip), nil
}

func processNodes(
	atmosConfig *schema.AtmosConfiguration,
	data map[string]any,
	currentStack string,
	skip []string,
) map[string]any {
	type stateNode struct {
		parent any    // *map[string]any or *[]any
		key    any    // string (for maps) or int (for slices)
		value  string // original string
	}

	var stateNodes []stateNode
	var mu sync.Mutex

	var recurse func(node any, parent any, key any) any

	recurse = func(node any, parent any, key any) any {
		switch v := node.(type) {
		case string:
			if strings.HasPrefix(v, u.AtmosYamlFuncTerraformState) && !skipFunc(skip, u.AtmosYamlFuncTerraformState) {
				// Defer processing `!terraform.state` tags, just collect them.
				// All of them will be processed concurrently later in the flow, and the results will be inserted back into the structure.
				mu.Lock()
				stateNodes = append(stateNodes, stateNode{parent: parent, key: key, value: v})
				mu.Unlock()
				return v // Keep the original value for now
			}
			// Process all other strings immediately
			return processCustomTags(atmosConfig, v, currentStack, skip)

		case map[string]any:
			newNestedMap := make(map[string]any)
			for k, val := range v {
				newNestedMap[k] = recurse(val, newNestedMap, k)
			}
			return newNestedMap

		case []any:
			newSlice := make([]any, len(v))
			for i, val := range v {
				newSlice[i] = recurse(val, newSlice, i)
			}
			return newSlice

		default:
			return v
		}
	}

	// Start recursion with the original map.
	newMap := make(map[string]any)
	for k, v := range data {
		newMap[k] = recurse(v, newMap, k)
	}

	// Process `!terraform.state` tags concurrently and insert the results back into the structure.
	var wg sync.WaitGroup
	for _, node := range stateNodes {
		wg.Add(1)
		go func(n stateNode) {
			defer wg.Done()
			res := processTagTerraformState(atmosConfig, n.value, currentStack)

			switch parent := n.parent.(type) {
			case map[string]any:
				parent[n.key.(string)] = res
			case []any:
				parent[n.key.(int)] = res
			}
		}(node)
	}
	wg.Wait()

	return newMap
}

func processCustomTags(
	atmosConfig *schema.AtmosConfiguration,
	input string,
	currentStack string,
	skip []string,
) any {
	switch {
	case strings.HasPrefix(input, u.AtmosYamlFuncTemplate) && !skipFunc(skip, u.AtmosYamlFuncTemplate):
		return processTagTemplate(input)
	case strings.HasPrefix(input, u.AtmosYamlFuncExec) && !skipFunc(skip, u.AtmosYamlFuncExec):
		res, err := u.ProcessTagExec(input)
		errUtils.CheckErrorPrintAndExit(err, "", "")
		return res
	case strings.HasPrefix(input, u.AtmosYamlFuncStore) && !skipFunc(skip, u.AtmosYamlFuncStore):
		return processTagStore(atmosConfig, input, currentStack)
	case strings.HasPrefix(input, u.AtmosYamlFuncTerraformOutput) && !skipFunc(skip, u.AtmosYamlFuncTerraformOutput):
		return processTagTerraformOutput(atmosConfig, input, currentStack)
	case strings.HasPrefix(input, u.AtmosYamlFuncTerraformState) && !skipFunc(skip, u.AtmosYamlFuncTerraformState):
		return processTagTerraformState(atmosConfig, input, currentStack)
	case strings.HasPrefix(input, u.AtmosYamlFuncEnv) && !skipFunc(skip, u.AtmosYamlFuncEnv):
		res, err := u.ProcessTagEnv(input)
		errUtils.CheckErrorPrintAndExit(err, "", "")
		return res
	default:
		// If any other YAML explicit tag (not currently supported by Atmos) is used, return it w/o processing
		return input
	}
}

func skipFunc(skip []string, f string) bool {
	t := strings.TrimPrefix(f, "!")
	c := u.SliceContainsString(skip, t)
	return c
}

func getStringAfterTag(input string, tag string) (string, error) {
	str := strings.TrimPrefix(input, tag)
	str = strings.TrimSpace(str)

	if str == "" {
		err := fmt.Errorf("invalid Atmos YAML function: %s", input)
		return "", err
	}

	return str, nil
}
