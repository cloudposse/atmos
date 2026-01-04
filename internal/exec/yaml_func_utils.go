package exec

import (
	"fmt"
	"strings"

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
	return processNodesWithContext(atmosConfig, input, currentStack, skip, resolutionCtx, stackInfo)
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

	return processNodesWithContext(atmosConfig, input, currentStack, skip, resolutionCtx, stackInfo)
}

func processNodes(
	atmosConfig *schema.AtmosConfiguration,
	data map[string]any,
	currentStack string,
	skip []string,
	stackInfo *schema.ConfigAndStacksInfo,
) (map[string]any, error) {
	return processNodesWithContext(atmosConfig, data, currentStack, skip, nil, stackInfo)
}

func processNodesWithContext(
	atmosConfig *schema.AtmosConfiguration,
	data map[string]any,
	currentStack string,
	skip []string,
	resolutionCtx *ResolutionContext,
	stackInfo *schema.ConfigAndStacksInfo,
) (map[string]any, error) {
	newMap := make(map[string]any)
	var firstErr error

	var recurse func(any) any
	recurse = func(node any) any {
		// If we already have an error, skip processing.
		if firstErr != nil {
			return node
		}

		switch v := node.(type) {
		case string:
			result, err := processCustomTagsWithContext(atmosConfig, v, currentStack, skip, resolutionCtx, stackInfo)
			if err != nil {
				firstErr = err
				return v
			}
			return result

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

	if firstErr != nil {
		return nil, firstErr
	}

	return newMap, nil
}

func processCustomTags(
	atmosConfig *schema.AtmosConfiguration,
	input string,
	currentStack string,
	skip []string,
	stackInfo *schema.ConfigAndStacksInfo,
) (any, error) {
	return processCustomTagsWithContext(atmosConfig, input, currentStack, skip, nil, stackInfo)
}

// matchesPrefix checks if input has the given prefix and the function is not skipped.
func matchesPrefix(input, prefix string, skip []string) bool {
	return strings.HasPrefix(input, prefix) && !skipFunc(skip, prefix)
}

// processContextAwareTags processes tags that support cycle detection.
// Returns (result, handled, error) where handled indicates if a matching tag was found.
func processContextAwareTags(
	atmosConfig *schema.AtmosConfiguration,
	input string,
	currentStack string,
	skip []string,
	resolutionCtx *ResolutionContext,
	stackInfo *schema.ConfigAndStacksInfo,
) (any, bool, error) {
	if matchesPrefix(input, u.AtmosYamlFuncTerraformOutput, skip) {
		result, err := processTagTerraformOutputWithContext(atmosConfig, input, currentStack, resolutionCtx, stackInfo)
		return result, true, err
	}
	if matchesPrefix(input, u.AtmosYamlFuncTerraformState, skip) {
		result, err := processTagTerraformStateWithContext(atmosConfig, input, currentStack, resolutionCtx, stackInfo)
		return result, true, err
	}
	return nil, false, nil
}

// processSimpleTags processes tags that don't need cycle detection.
// Returns (result, handled, error) where handled indicates if a matching tag was found.
func processSimpleTags(
	atmosConfig *schema.AtmosConfiguration,
	input string,
	currentStack string,
	skip []string,
	stackInfo *schema.ConfigAndStacksInfo,
) (any, bool, error) {
	// Handle !unset tag - return marker to indicate value should be deleted.
	if matchesPrefix(input, u.AtmosYamlFuncUnset, skip) {
		return UnsetMarker{IsUnset: true}, true, nil
	}
	if matchesPrefix(input, u.AtmosYamlFuncTemplate, skip) {
		return processTagTemplate(input), true, nil
	}
	if matchesPrefix(input, u.AtmosYamlFuncExec, skip) {
		res, err := u.ProcessTagExec(input)
		if err != nil {
			return nil, true, err
		}
		return res, true, nil
	}
	if matchesPrefix(input, u.AtmosYamlFuncStoreGet, skip) {
		return processTagStoreGet(atmosConfig, input, currentStack), true, nil
	}
	if matchesPrefix(input, u.AtmosYamlFuncStore, skip) {
		return processTagStore(atmosConfig, input, currentStack), true, nil
	}
	if matchesPrefix(input, u.AtmosYamlFuncEnv, skip) {
		res, err := u.ProcessTagEnv(input, stackInfo)
		if err != nil {
			return nil, true, err
		}
		return res, true, nil
	}
	// AWS YAML functions - note these check for exact match since they take no arguments.
	if input == u.AtmosYamlFuncAwsAccountID && !skipFunc(skip, u.AtmosYamlFuncAwsAccountID) {
		return processTagAwsAccountID(atmosConfig, input, stackInfo), true, nil
	}
	if input == u.AtmosYamlFuncAwsCallerIdentityArn && !skipFunc(skip, u.AtmosYamlFuncAwsCallerIdentityArn) {
		return processTagAwsCallerIdentityArn(atmosConfig, input, stackInfo), true, nil
	}
	if input == u.AtmosYamlFuncAwsCallerIdentityUserID && !skipFunc(skip, u.AtmosYamlFuncAwsCallerIdentityUserID) {
		return processTagAwsCallerIdentityUserID(atmosConfig, input, stackInfo), true, nil
	}
	if input == u.AtmosYamlFuncAwsRegion && !skipFunc(skip, u.AtmosYamlFuncAwsRegion) {
		return processTagAwsRegion(atmosConfig, input, stackInfo), true, nil
	}
	return nil, false, nil
}

func processCustomTagsWithContext(
	atmosConfig *schema.AtmosConfiguration,
	input string,
	currentStack string,
	skip []string,
	resolutionCtx *ResolutionContext,
	stackInfo *schema.ConfigAndStacksInfo,
) (any, error) {
	// Try context-aware tags first.
	if result, handled, err := processContextAwareTags(atmosConfig, input, currentStack, skip, resolutionCtx, stackInfo); handled {
		return result, err
	}

	// Try simple tags.
	if result, handled, err := processSimpleTags(atmosConfig, input, currentStack, skip, stackInfo); handled {
		return result, err
	}

	// If any other YAML explicit tag (not currently supported by Atmos) is used, return it w/o processing.
	return input, nil
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
