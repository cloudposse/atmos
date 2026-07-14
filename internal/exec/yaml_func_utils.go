package exec

import (
	"fmt"
	"slices"
	"strings"

	"github.com/cloudposse/atmos/pkg/emulator"
	atmosGit "github.com/cloudposse/atmos/pkg/git"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/secrets"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/cloudposse/atmos/pkg/version/manager"
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

	// Reuse the goroutine-local ResolutionContext so that cycle detection survives
	// across nested ProcessCustomYamlTags entries triggered by !terraform.state /
	// !terraform.output (where resolving the function recurses into a fresh
	// ExecuteDescribeComponent → ProcessStacks → ProcessCustomYamlTags pass on the
	// referenced component). Installing a fresh, scoped context here — as a prior
	// implementation did — wiped the Visited map of the outer walk and made A↔B
	// component cycles unrecoverable; see #2457.
	//
	// Cleanup is owned by the Push/Pop discipline in processTagTerraformState* /
	// processTagTerraformOutput*: every successful Push has a matching deferred Pop,
	// so the context is empty when the top-level walk returns. Callers that need a
	// hard reset (e.g., test isolation) can call ClearResolutionContext explicitly.
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
				log.Debug(
					"Error processing YAML function",
					"value", v,
					"stack", currentStack,
					"error", err.Error(),
				)
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
			// Pre-allocate a non-nil slice so an empty input list (or a list whose items are
			// all removed by !unset) stays an empty list rather than collapsing to nil. A nil
			// slice marshals to JSON `null` in generated tfvars, which breaks consumers such as
			// Terraform's concat() that reject null where a list is expected.
			newSlice := make([]any, 0, len(v))
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

// matchesTag reports whether input starts with prefix as a complete YAML tag.
func matchesTag(input, prefix string) bool {
	if !strings.HasPrefix(input, prefix) {
		return false
	}
	rest := strings.TrimPrefix(input, prefix)
	return rest == "" || rest[0] == ' ' || rest[0] == '\t' || rest[0] == '\n'
}

// matchesPrefix checks if input has the given tag prefix and the function is not skipped.
func matchesPrefix(input, prefix string, skip []string) bool {
	return matchesTag(input, prefix) && !skipFunc(skip, prefix)
}

func exactTagSkipped(input, tag string, skip []string) bool {
	return input == tag && skipFunc(skip, tag)
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
	if matchesPrefix(input, u.AtmosYamlFuncSecret, skip) {
		res, err := secrets.Resolve(atmosConfig, input, currentStack, stackInfo)
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
	if matchesPrefix(input, u.AtmosYamlFuncGitRoot, skip) || matchesPrefix(input, u.AtmosYamlFuncGitRootAlias, skip) {
		res, err := atmosGit.ProcessTagRoot(input)
		if err != nil {
			return nil, true, err
		}
		return res, true, nil
	}
	if matchesPrefix(input, u.AtmosYamlFuncGitSha, skip) || matchesPrefix(input, u.AtmosYamlFuncGitRef, skip) {
		res, err := atmosGit.ProcessTagSHA(input)
		if err != nil {
			return nil, true, err
		}
		return res, true, nil
	}
	if matchesPrefix(input, u.AtmosYamlFuncGitBranch, skip) {
		res, err := atmosGit.ProcessTagBranch(input)
		if err != nil {
			return nil, true, err
		}
		return res, true, nil
	}
	if matchesPrefix(input, u.AtmosYamlFuncGitRepository, skip) {
		res, err := atmosGit.ProcessTagRepository(input)
		if err != nil {
			return nil, true, err
		}
		return res, true, nil
	}
	if matchesPrefix(input, u.AtmosYamlFuncGitOwner, skip) {
		res, err := atmosGit.ProcessTagOwner(input)
		if err != nil {
			return nil, true, err
		}
		return res, true, nil
	}
	if matchesPrefix(input, u.AtmosYamlFuncGitName, skip) {
		res, err := atmosGit.ProcessTagName(input)
		if err != nil {
			return nil, true, err
		}
		return res, true, nil
	}
	if matchesPrefix(input, u.AtmosYamlFuncGitHost, skip) {
		res, err := atmosGit.ProcessTagHost(input)
		if err != nil {
			return nil, true, err
		}
		return res, true, nil
	}
	if matchesPrefix(input, u.AtmosYamlFuncGitUrl, skip) {
		res, err := atmosGit.ProcessTagURL(input)
		if err != nil {
			return nil, true, err
		}
		return res, true, nil
	}
	// AWS YAML functions - note these check for exact match since they take no arguments.
	if exactTagSkipped(input, u.AtmosYamlFuncAwsAccountID, skip) {
		return input, true, nil
	}
	if input == u.AtmosYamlFuncAwsAccountID && !skipFunc(skip, u.AtmosYamlFuncAwsAccountID) {
		return processTagAwsAccountID(atmosConfig, input, stackInfo), true, nil
	}
	if exactTagSkipped(input, u.AtmosYamlFuncAwsCallerIdentityArn, skip) {
		return input, true, nil
	}
	if input == u.AtmosYamlFuncAwsCallerIdentityArn && !skipFunc(skip, u.AtmosYamlFuncAwsCallerIdentityArn) {
		return processTagAwsCallerIdentityArn(atmosConfig, input, stackInfo), true, nil
	}
	if exactTagSkipped(input, u.AtmosYamlFuncAwsCallerIdentityUserID, skip) {
		return input, true, nil
	}
	if input == u.AtmosYamlFuncAwsCallerIdentityUserID && !skipFunc(skip, u.AtmosYamlFuncAwsCallerIdentityUserID) {
		return processTagAwsCallerIdentityUserID(atmosConfig, input, stackInfo), true, nil
	}
	if exactTagSkipped(input, u.AtmosYamlFuncAwsRegion, skip) {
		return input, true, nil
	}
	if input == u.AtmosYamlFuncAwsRegion && !skipFunc(skip, u.AtmosYamlFuncAwsRegion) {
		return processTagAwsRegion(atmosConfig, input, stackInfo), true, nil
	}
	if input == u.AtmosYamlFuncAwsOrganizationID && !skipFunc(skip, u.AtmosYamlFuncAwsOrganizationID) {
		return processTagAwsOrganizationID(atmosConfig, input, stackInfo), true, nil
	}
	if matchesPrefix(input, u.AtmosYamlFuncEmulator, skip) {
		args, err := getStringAfterTag(input, u.AtmosYamlFuncEmulator)
		if err != nil {
			return nil, true, err
		}
		res, err := emulator.ResolveYAMLFunc(atmosConfig, args, currentStack, stackInfo)
		if err != nil {
			return nil, true, err
		}
		return res, true, nil
	}
	if matchesPrefix(input, u.AtmosYamlFuncVersion, skip) {
		name, err := getStringAfterTag(input, u.AtmosYamlFuncVersion)
		if err != nil {
			return nil, true, err
		}
		res, err := manager.ResolveYAMLFunc(atmosConfig, name, stackInfo)
		if err != nil {
			return nil, true, err
		}
		return res, true, nil
	}
	// !tags/!labels family - no arguments; check the longer .keys/.values
	// suffixes before the bare !labels match.
	if exactTagSkipped(input, u.AtmosYamlFuncTags, skip) {
		return input, true, nil
	}
	if input == u.AtmosYamlFuncTags && !skipFunc(skip, u.AtmosYamlFuncTags) {
		return processTagTags(atmosConfig, input, stackInfo), true, nil
	}
	if exactTagSkipped(input, u.AtmosYamlFuncLabelsKeys, skip) {
		return input, true, nil
	}
	if input == u.AtmosYamlFuncLabelsKeys && !skipFunc(skip, u.AtmosYamlFuncLabelsKeys) {
		return processTagLabelsKeys(atmosConfig, input, stackInfo), true, nil
	}
	if exactTagSkipped(input, u.AtmosYamlFuncLabelsValues, skip) {
		return input, true, nil
	}
	if input == u.AtmosYamlFuncLabelsValues && !skipFunc(skip, u.AtmosYamlFuncLabelsValues) {
		return processTagLabelsValues(atmosConfig, input, stackInfo), true, nil
	}
	if exactTagSkipped(input, u.AtmosYamlFuncLabels, skip) {
		return input, true, nil
	}
	if input == u.AtmosYamlFuncLabels && !skipFunc(skip, u.AtmosYamlFuncLabels) {
		return processTagLabels(atmosConfig, input, stackInfo), true, nil
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
	c := slices.Contains(skip, t)
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
