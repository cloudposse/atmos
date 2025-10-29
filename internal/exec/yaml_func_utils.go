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
	stackInfo *schema.ConfigAndStacksInfo,
) (schema.AtmosSectionMapType, error) {
	defer perf.Track(atmosConfig, "exec.ProcessCustomYamlTags")()

	// Create a scoped resolution context to prevent memory leaks and cross-call contamination.
	// Save any existing context, install a fresh one, and restore on exit.
	restoreCtx := scopedResolutionContext()
	defer restoreCtx()

	// Get the fresh context we just installed.
	resolutionCtx := GetOrCreateResolutionContext()
	return processNodesWithContext(atmosConfig, input, currentStack, skip, resolutionCtx, stackInfo), nil
}

func ProcessCustomYamlTagsWithContext(
	atmosConfig *schema.AtmosConfiguration,
	input schema.AtmosSectionMapType,
	currentStack string,
	skip []string,
	resolutionCtx *ResolutionContext,
	stackInfo *schema.ConfigAndStacksInfo,
) (schema.AtmosSectionMapType, error) {
	defer perf.Track(atmosConfig, "exec.ProcessCustomYamlTagsWithContext")()

	return processNodesWithContext(atmosConfig, input, currentStack, skip, resolutionCtx, stackInfo), nil
}

func processNodes(
	atmosConfig *schema.AtmosConfiguration,
	data map[string]any,
	currentStack string,
	skip []string,
	stackInfo *schema.ConfigAndStacksInfo,
) map[string]any {
	return processNodesWithContext(atmosConfig, data, currentStack, skip, nil, stackInfo)
}

func processNodesWithContext(
	atmosConfig *schema.AtmosConfiguration,
	data map[string]any,
	currentStack string,
	skip []string,
	resolutionCtx *ResolutionContext,
	stackInfo *schema.ConfigAndStacksInfo,
) map[string]any {
	newMap := make(map[string]any)
	var recurse func(any) any

	recurse = func(node any) any {
		switch v := node.(type) {
		case string:
			return processCustomTagsWithContext(atmosConfig, v, currentStack, skip, resolutionCtx, stackInfo)

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
	stackInfo *schema.ConfigAndStacksInfo,
) any {
	return processCustomTagsWithContext(atmosConfig, input, currentStack, skip, nil, stackInfo)
}

// matchesPrefix checks if input has the given prefix and the function is not skipped.
func matchesPrefix(input, prefix string, skip []string) bool {
	return strings.HasPrefix(input, prefix) && !skipFunc(skip, prefix)
}

// processContextAwareTags processes tags that support cycle detection.
func processContextAwareTags(
	atmosConfig *schema.AtmosConfiguration,
	input string,
	currentStack string,
	skip []string,
	resolutionCtx *ResolutionContext,
	stackInfo *schema.ConfigAndStacksInfo,
) (any, bool) {
	if matchesPrefix(input, u.AtmosYamlFuncTerraformOutput, skip) {
		return processTagTerraformOutputWithContext(atmosConfig, input, currentStack, resolutionCtx, stackInfo), true
	}
	if matchesPrefix(input, u.AtmosYamlFuncTerraformState, skip) {
		return processTagTerraformStateWithContext(atmosConfig, input, currentStack, resolutionCtx, stackInfo), true
	}
	return nil, false
}

// processSimpleTags processes tags that don't need cycle detection.
func processSimpleTags(
	atmosConfig *schema.AtmosConfiguration,
	input string,
	currentStack string,
	skip []string,
) (any, bool) {
	if matchesPrefix(input, u.AtmosYamlFuncTemplate, skip) {
		return processTagTemplate(input), true
	}
	if matchesPrefix(input, u.AtmosYamlFuncExec, skip) {
		res, err := u.ProcessTagExec(input)
		errUtils.CheckErrorPrintAndExit(err, "", "")
		return res, true
	}
	if matchesPrefix(input, u.AtmosYamlFuncStoreGet, skip) {
		return processTagStoreGet(atmosConfig, input, currentStack), true
	}
	if matchesPrefix(input, u.AtmosYamlFuncStore, skip) {
		return processTagStore(atmosConfig, input, currentStack), true
	}
	if matchesPrefix(input, u.AtmosYamlFuncEnv, skip) {
		res, err := u.ProcessTagEnv(input)
		errUtils.CheckErrorPrintAndExit(err, "", "")
		return res, true
	}
	return nil, false
}

func processCustomTagsWithContext(
	atmosConfig *schema.AtmosConfiguration,
	input string,
	currentStack string,
	skip []string,
	resolutionCtx *ResolutionContext,
	stackInfo *schema.ConfigAndStacksInfo,
) any {
	// Try context-aware tags first.
	if result, handled := processContextAwareTags(atmosConfig, input, currentStack, skip, resolutionCtx, stackInfo); handled {
		return result
	}

	// Try simple tags.
	if result, handled := processSimpleTags(atmosConfig, input, currentStack, skip); handled {
		return result
	}

	// If any other YAML explicit tag (not currently supported by Atmos) is used, return it w/o processing.
	return input
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
