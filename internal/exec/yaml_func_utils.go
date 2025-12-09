package exec

import (
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// UnsetMarker is a special type to mark values that should be deleted from the configuration.
type UnsetMarker struct {
	IsUnset bool
}

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
				// Check if the value is a string with !unset tag and it's not skipped.
				if strVal, ok := val.(string); ok && strings.HasPrefix(strVal, u.AtmosYamlFuncUnset) && !skipFunc(skip, u.AtmosYamlFuncUnset) {
					// Skip adding this key to the map - effectively deleting it.
					continue
				}
				processed := recurse(val)
				// Check if the processed value is the unset marker.
				if marker, ok := processed.(UnsetMarker); ok && marker.IsUnset {
					// Skip adding this key to the map - effectively deleting it.
					continue
				}
				newNestedMap[k] = processed
			}
			return newNestedMap

		case []any:
			var newSlice []any
			for _, val := range v {
				// Check if the value is a string with !unset tag and it's not skipped.
				if strVal, ok := val.(string); ok && strings.HasPrefix(strVal, u.AtmosYamlFuncUnset) && !skipFunc(skip, u.AtmosYamlFuncUnset) {
					// Skip adding this item to the slice - effectively deleting it.
					continue
				}
				processed := recurse(val)
				// Check if the processed value is the unset marker.
				if marker, ok := processed.(UnsetMarker); ok && marker.IsUnset {
					// Skip adding this item to the slice - effectively deleting it.
					continue
				}
				newSlice = append(newSlice, processed)
			}
			return newSlice

		default:
			return v
		}
	}

	for k, v := range data {
		// Check if the value is a string with !unset tag and it's not skipped.
		if strVal, ok := v.(string); ok && strings.HasPrefix(strVal, u.AtmosYamlFuncUnset) && !skipFunc(skip, u.AtmosYamlFuncUnset) {
			// Skip adding this key to the map - effectively deleting it.
			continue
		}
		processed := recurse(v)
		// Check if the processed value is the unset marker.
		if marker, ok := processed.(UnsetMarker); ok && marker.IsUnset {
			// Skip adding this key to the map - effectively deleting it.
			continue
		}
		newMap[k] = processed
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
	stackInfo *schema.ConfigAndStacksInfo,
) (any, bool) {
	// Handle !unset tag - return marker to indicate value should be deleted.
	if matchesPrefix(input, u.AtmosYamlFuncUnset, skip) {
		return UnsetMarker{IsUnset: true}, true
	}
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
		res, err := u.ProcessTagEnv(input, stackInfo)
		errUtils.CheckErrorPrintAndExit(err, "", "")
		return res, true
	}
	// AWS YAML functions - note these check for exact match since they take no arguments.
	if input == u.AtmosYamlFuncAwsAccountID && !skipFunc(skip, u.AtmosYamlFuncAwsAccountID) {
		return processTagAwsAccountID(atmosConfig, input, stackInfo), true
	}
	if input == u.AtmosYamlFuncAwsCallerIdentityArn && !skipFunc(skip, u.AtmosYamlFuncAwsCallerIdentityArn) {
		return processTagAwsCallerIdentityArn(atmosConfig, input, stackInfo), true
	}
	if input == u.AtmosYamlFuncAwsCallerIdentityUserID && !skipFunc(skip, u.AtmosYamlFuncAwsCallerIdentityUserID) {
		return processTagAwsCallerIdentityUserID(atmosConfig, input, stackInfo), true
	}
	if input == u.AtmosYamlFuncAwsRegion && !skipFunc(skip, u.AtmosYamlFuncAwsRegion) {
		return processTagAwsRegion(atmosConfig, input, stackInfo), true
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
	if result, handled := processSimpleTags(atmosConfig, input, currentStack, skip, stackInfo); handled {
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
