package merge

import (
	"fmt"

	"dario.cat/mergo"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const (
	ListMergeStrategyReplace = "replace"
	ListMergeStrategyAppend  = "append"
	ListMergeStrategyMerge   = "merge"
)

// MergeWithOptions takes a list of maps and options as input, deep-merges the items in the order they are defined in the list,
// and returns a single map with the merged contents.
func MergeWithOptions(
	inputs []map[string]any,
	appendSlice bool,
	sliceDeepCopy bool,
) (map[string]any, error) {
	merged := map[string]any{}

	for index := range inputs {
		current := inputs[index]

		if len(current) == 0 {
			continue
		}

		// Process !append tagged lists before merging
		current = processAppendTags(current, merged)

		// Due to a bug in `mergo.Merge`
		// (Note: in the `for` loop, it DOES modify the source of the previous loop iteration if it's a complex map and `mergo` gets a pointer to it,
		// not only the destination of the current loop iteration),
		// we don't give it our maps directly; we convert them to YAML strings and then back to `Go` maps,
		// so `mergo` does not have access to the original pointers
		yamlCurrent, err := u.ConvertToYAML(current)
		if err != nil {
			return nil, fmt.Errorf("%w: failed to convert to YAML: %v", errUtils.ErrMerge, err)
		}

		dataCurrent, err := u.UnmarshalYAML[any](yamlCurrent)
		if err != nil {
			return nil, fmt.Errorf("%w: failed to unmarshal YAML: %v", errUtils.ErrMerge, err)
		}

		var opts []func(*mergo.Config)
		opts = append(opts, mergo.WithOverride, mergo.WithTypeCheck)

		// This was fixed/broken in https://github.com/imdario/mergo/pull/231/files
		// It was released in https://github.com/imdario/mergo/releases/tag/v0.3.14
		// It was not working before in `github.com/imdario/mergo` so we need to disable it in our code
		// opts = append(opts, mergo.WithOverwriteWithEmptyValue)

		if sliceDeepCopy {
			opts = append(opts, mergo.WithSliceDeepCopy)
		} else if appendSlice {
			opts = append(opts, mergo.WithAppendSlice)
		}

		if err = mergo.Merge(&merged, dataCurrent, opts...); err != nil {
			// Return the error without debug logging
			return nil, fmt.Errorf("%w: mergo merge failed: %v", errUtils.ErrMerge, err)
		}
	}

	return merged, nil
}

// processAppendTags handles special !append tagged lists during merging.
// It processes any values wrapped with __atmos_append__ metadata and appends them to existing lists.
func processAppendTags(current map[string]any, merged map[string]any) map[string]any {
	result := make(map[string]any)

	for key, value := range current {
		result[key] = processValue(key, value, merged)
	}

	return result
}

// processValue processes a single value for append tags.
func processValue(key string, value any, merged map[string]any) any {
	// Check if this is an append-tagged list
	if list, isAppend := u.ExtractAppendListValue(value); isAppend {
		return processAppendList(key, list, merged)
	}

	// Check if this is a nested map
	if nestedMap, ok := value.(map[string]any); ok {
		return processNestedMap(key, nestedMap, merged)
	}

	// Regular value, pass through
	return value
}

// processAppendList handles appending a list to existing values.
func processAppendList(key string, list []any, merged map[string]any) []any {
	var existingList []any
	if existingValue, exists := merged[key]; exists {
		if el, ok := existingValue.([]any); ok {
			existingList = el
		}
	}

	// Create a new slice to avoid modifying the original
	result := make([]any, len(existingList), len(existingList)+len(list))
	copy(result, existingList)
	result = append(result, list...)
	return result
}

// processNestedMap recursively processes nested maps for append tags.
func processNestedMap(key string, nestedMap map[string]any, merged map[string]any) map[string]any {
	var mergedNested map[string]any
	if existingNested, exists := merged[key]; exists {
		if mn, ok := existingNested.(map[string]any); ok {
			mergedNested = mn
		}
	}
	if mergedNested == nil {
		mergedNested = make(map[string]any)
	}
	return processAppendTags(nestedMap, mergedNested)
}

// Merge takes a list of maps as input, deep-merges the items in the order they are defined in the list, and returns a single map with the merged contents.
func Merge(
	atmosConfig *schema.AtmosConfiguration,
	inputs []map[string]any,
) (map[string]any, error) {
	// Check for nil config to prevent panic
	if atmosConfig == nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrMerge, errUtils.ErrAtmosConfigIsNil)
	}

	// Default to replace strategy if strategy is empty
	strategy := ListMergeStrategyReplace
	if atmosConfig.Settings.ListMergeStrategy != "" {
		strategy = atmosConfig.Settings.ListMergeStrategy
	}

	if strategy != ListMergeStrategyReplace &&
		strategy != ListMergeStrategyAppend &&
		strategy != ListMergeStrategyMerge {
		return nil, fmt.Errorf("%w: %w: '%s'. Supported list merge strategies are: %s",
			errUtils.ErrMerge,
			errUtils.ErrInvalidListMergeStrategy,
			strategy,
			fmt.Sprintf("%s, %s, %s", ListMergeStrategyReplace, ListMergeStrategyAppend, ListMergeStrategyMerge))
	}

	sliceDeepCopy := false
	appendSlice := false

	switch strategy {
	case ListMergeStrategyMerge:
		sliceDeepCopy = true
	case ListMergeStrategyAppend:
		appendSlice = true
	}

	return MergeWithOptions(inputs, appendSlice, sliceDeepCopy)
}

// MergeWithContext performs a merge operation with file context tracking for better error messages.
func MergeWithContext(
	atmosConfig *schema.AtmosConfiguration,
	inputs []map[string]any,
	context *MergeContext,
) (map[string]any, error) {
	// Check for nil config to prevent panic
	if atmosConfig == nil {
		err := fmt.Errorf("%w: %w", errUtils.ErrMerge, errUtils.ErrAtmosConfigIsNil)
		if context != nil {
			return nil, context.FormatError(err)
		}
		return nil, err
	}

	// Default to replace strategy if strategy is empty
	strategy := ListMergeStrategyReplace
	if atmosConfig.Settings.ListMergeStrategy != "" {
		strategy = atmosConfig.Settings.ListMergeStrategy
	}

	if strategy != ListMergeStrategyReplace &&
		strategy != ListMergeStrategyAppend &&
		strategy != ListMergeStrategyMerge {
		err := fmt.Errorf("%w: %w: '%s'. Supported list merge strategies are: %s",
			errUtils.ErrMerge,
			errUtils.ErrInvalidListMergeStrategy,
			strategy,
			fmt.Sprintf("%s, %s, %s", ListMergeStrategyReplace, ListMergeStrategyAppend, ListMergeStrategyMerge))
		if context != nil {
			return nil, context.FormatError(err)
		}
		return nil, err
	}

	sliceDeepCopy := false
	appendSlice := false

	switch strategy {
	case ListMergeStrategyMerge:
		sliceDeepCopy = true
	case ListMergeStrategyAppend:
		appendSlice = true
	}

	return MergeWithOptionsAndContext(inputs, appendSlice, sliceDeepCopy, context)
}

// MergeWithOptionsAndContext performs merge with options and context tracking.
func MergeWithOptionsAndContext(
	inputs []map[string]any,
	appendSlice bool,
	sliceDeepCopy bool,
	context *MergeContext,
) (map[string]any, error) {
	// Remove verbose merge operation logging - it creates too much noise
	// Users can use ATMOS_LOGS_LEVEL=Trace if they need detailed merge debugging

	result, err := MergeWithOptions(inputs, appendSlice, sliceDeepCopy)
	if err != nil {
		// Remove verbose merge failure logging
		// The error context will be shown in the formatted error message

		// Add context information to the error
		if context != nil {
			return nil, context.FormatError(err)
		}
		return nil, err
	}

	return result, nil
}
