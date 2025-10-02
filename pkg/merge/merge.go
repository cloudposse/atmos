package merge

import (
	"errors"
	"fmt"

	"dario.cat/mergo"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
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
	atmosConfig *schema.AtmosConfiguration,
	inputs []map[string]any,
	appendSlice bool,
	sliceDeepCopy bool,
) (map[string]any, error) {
	defer perf.Track(atmosConfig, "merge.MergeWithOptions")()

	merged := map[string]any{}

	for index := range inputs {
		current := inputs[index]

		if len(current) == 0 {
			continue
		}

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

// Merge takes a list of maps as input, deep-merges the items in the order they are defined in the list, and returns a single map with the merged contents.
func Merge(
	atmosConfig *schema.AtmosConfiguration,
	inputs []map[string]any,
) (map[string]any, error) {
	defer perf.Track(atmosConfig, "merge.Merge")()

	// Check for nil config to prevent panic.
	if atmosConfig == nil {
		return nil, errors.Join(errUtils.ErrMerge, errUtils.ErrAtmosConfigIsNil)
	}

	// Default to replace strategy if strategy is empty
	strategy := ListMergeStrategyReplace
	if atmosConfig.Settings.ListMergeStrategy != "" {
		strategy = atmosConfig.Settings.ListMergeStrategy
	}

	if strategy != ListMergeStrategyReplace &&
		strategy != ListMergeStrategyAppend &&
		strategy != ListMergeStrategyMerge {
		err := fmt.Errorf("%w: '%s'. Supported list merge strategies are: %s",
			errUtils.ErrInvalidListMergeStrategy,
			strategy,
			fmt.Sprintf("%s, %s, %s", ListMergeStrategyReplace, ListMergeStrategyAppend, ListMergeStrategyMerge))
		return nil, errors.Join(errUtils.ErrMerge, err)
	}

	sliceDeepCopy := false
	appendSlice := false

	switch strategy {
	case ListMergeStrategyMerge:
		sliceDeepCopy = true
	case ListMergeStrategyAppend:
		appendSlice = true
	}

	return MergeWithOptions(atmosConfig, inputs, appendSlice, sliceDeepCopy)
}

// MergeWithContext performs a merge operation with file context tracking for better error messages.
func MergeWithContext(
	atmosConfig *schema.AtmosConfiguration,
	inputs []map[string]any,
	context *MergeContext,
) (map[string]any, error) {
	defer perf.Track(atmosConfig, "merge.MergeWithContext")()

	// Check for nil config to prevent panic
	if atmosConfig == nil {
		err := fmt.Errorf("%w: %s", errUtils.ErrMerge, errUtils.ErrAtmosConfigIsNil)
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
		err := fmt.Errorf("%w: %s: '%s'. Supported list merge strategies are: %s",
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

	return MergeWithOptionsAndContext(atmosConfig, inputs, appendSlice, sliceDeepCopy, context)
}

// MergeWithOptionsAndContext performs merge with options and context tracking.
func MergeWithOptionsAndContext(
	atmosConfig *schema.AtmosConfiguration,
	inputs []map[string]any,
	appendSlice bool,
	sliceDeepCopy bool,
	context *MergeContext,
) (map[string]any, error) {
	defer perf.Track(atmosConfig, "merge.MergeWithOptionsAndContext")()

	// Remove verbose merge operation logging - it creates too much noise
	// Users can use ATMOS_LOGS_LEVEL=Trace if they need detailed merge debugging

	result, err := MergeWithOptions(atmosConfig, inputs, appendSlice, sliceDeepCopy)
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
