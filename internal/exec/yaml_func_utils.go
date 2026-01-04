package exec

import (
	"context"
	"fmt"
	"strings"

	fn "github.com/cloudposse/atmos/pkg/function"
	fntag "github.com/cloudposse/atmos/pkg/function/tag"
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

// matchesPrefixOrSkipped checks if input matches a tag prefix.
// Returns (shouldProcess, isHandled) - if isHandled is true, no further processing needed.
// This prevents skipped tags like !store.get from falling through to match !store.
func matchesPrefixOrSkipped(input, prefix string, skip []string) (shouldProcess, isHandled bool) {
	if !strings.HasPrefix(input, prefix) {
		return false, false // Not this tag, continue checking other tags.
	}
	// Input matches this prefix. Check if it should be skipped.
	if skipFunc(skip, prefix) {
		return false, true // Tag is skipped, return input unchanged.
	}
	return true, true // Tag should be processed.
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
	// Check !terraform.output.
	if shouldProcess, handled := matchesPrefixOrSkipped(input, u.AtmosYamlFuncTerraformOutput, skip); handled {
		if shouldProcess {
			result, err := processTagTerraformOutputWithContext(atmosConfig, input, currentStack, resolutionCtx, stackInfo)
			return result, true, err
		}
		return input, true, nil
	}
	// Check !terraform.state.
	if shouldProcess, handled := matchesPrefixOrSkipped(input, u.AtmosYamlFuncTerraformState, skip); handled {
		if shouldProcess {
			result, err := processTagTerraformStateWithContext(atmosConfig, input, currentStack, resolutionCtx, stackInfo)
			return result, true, err
		}
		return input, true, nil
	}
	return nil, false, nil
}

// processSimpleTags processes tags that don't need cycle detection.
// Returns (result, handled, error) where handled indicates if a matching tag was found.
// This function uses the function registry for most tags.
func processSimpleTags(
	atmosConfig *schema.AtmosConfiguration,
	input string,
	currentStack string,
	skip []string,
	stackInfo *schema.ConfigAndStacksInfo,
) (any, bool, error) {
	// Use the function registry to process tags.
	// The registry handles: env, exec, template, store, store.get, aws.*, random, literal, etc.
	return executeRegistryFunction(atmosConfig, input, currentStack, skip, stackInfo)
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

// extractTagAndArgs extracts the tag name and arguments from a YAML function string.
// For example, "!env HOME" returns ("env", "HOME").
// For "!aws.account_id" returns ("aws.account_id", "").
func extractTagAndArgs(input string) (tagName, args string, ok bool) {
	if !strings.HasPrefix(input, "!") {
		return "", "", false
	}

	// Find the first whitespace (space, tab, or newline) to separate tag from args.
	whitespaceIdx := strings.IndexAny(input, " \t\n")
	if whitespaceIdx == -1 {
		// No whitespace means the entire string is the tag (e.g., "!aws.account_id").
		return fntag.FromYAML(input), "", true
	}

	tagPart := input[:whitespaceIdx]
	argsPart := strings.TrimSpace(input[whitespaceIdx+1:])
	return fntag.FromYAML(tagPart), argsPart, true
}

// createExecutionContext creates a fn.ExecutionContext from the current parameters.
func createExecutionContext(
	atmosConfig *schema.AtmosConfiguration,
	currentStack string,
	stackInfo *schema.ConfigAndStacksInfo,
) *fn.ExecutionContext {
	return &fn.ExecutionContext{
		AtmosConfig: atmosConfig,
		Stack:       currentStack,
		StackInfo:   stackInfo,
	}
}

// executeRegistryFunction looks up a function in the registry and executes it.
// Returns (result, handled, error) where handled indicates if a matching function was found.
func executeRegistryFunction(
	atmosConfig *schema.AtmosConfiguration,
	input string,
	currentStack string,
	skip []string,
	stackInfo *schema.ConfigAndStacksInfo,
) (any, bool, error) {
	tagName, args, ok := extractTagAndArgs(input)
	if !ok {
		return nil, false, nil
	}

	// Check if this tag should be skipped.
	if skipFunc(skip, "!"+tagName) {
		return input, true, nil
	}

	// Look up the function in the registry.
	registry := fn.DefaultRegistry()
	if !registry.Has(tagName) {
		// Function not found in registry - not handled.
		return nil, false, nil
	}
	regFn, _ := registry.Get(tagName)

	// Create execution context and execute the function.
	execCtx := createExecutionContext(atmosConfig, currentStack, stackInfo)
	result, err := regFn.Execute(context.Background(), args, execCtx)
	if err != nil {
		return nil, true, err
	}

	return result, true, nil
}
