package exec

import (
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func ProcessCustomYamlTags(
	atmosConfig *schema.AtmosConfiguration,
	input schema.AtmosSectionMapType,
	currentStack string,
	skip []string,
) (schema.AtmosSectionMapType, error) {
	defer perf.Track(atmosConfig, "exec.ProcessCustomYamlTags")()

	return processNodes(atmosConfig, input, currentStack, skip), nil
}

func processNodes(
	atmosConfig *schema.AtmosConfiguration,
	data map[string]any,
	currentStack string,
	skip []string,
) map[string]any {
	newMap := make(map[string]any)
	var recurse func(any) any

	recurse = func(node any) any {
		switch v := node.(type) {
		case string:
			return processCustomTags(atmosConfig, v, currentStack, skip)

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
	case strings.HasPrefix(input, u.AtmosYamlFuncStoreGet) && !skipFunc(skip, u.AtmosYamlFuncStoreGet):
		return processTagStoreGet(atmosConfig, input, currentStack)
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
		// Check if the input looks like a YAML tag (starts with !)
		if strings.HasPrefix(input, "!") {
			// Extract just the tag part (before any space)
			tagParts := strings.SplitN(input, " ", 2)
			tag := tagParts[0]

			// Check if this is an unsupported tag
			supportedTags := []string{
				u.AtmosYamlFuncTemplate,
				u.AtmosYamlFuncExec,
				u.AtmosYamlFuncStore,
				u.AtmosYamlFuncStoreGet,
				u.AtmosYamlFuncTerraformOutput,
				u.AtmosYamlFuncTerraformState,
				u.AtmosYamlFuncEnv,
			}

			for _, supported := range supportedTags {
				if strings.HasPrefix(input, supported) {
					// It's a supported tag but not handled in switch above (might be skipped)
					return input
				}
			}

			// It's an unsupported tag - log error and exit
			errUtils.CheckErrorPrintAndExit(
				fmt.Errorf("%w: '%s' in stack '%s'. Supported tags are: %s",
					errUtils.ErrUnsupportedYamlTag,
					tag,
					currentStack,
					strings.Join(supportedTags, ", ")),
				"", "")
		}
		// Not a tag, return as-is
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
