package merge

import (
	"fmt"

	"dario.cat/mergo"
	"github.com/fatih/color"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const (
	ListMergeStrategyReplace = "replace"
	ListMergeStrategyAppend  = "append"
	ListMergeStrategyMerge   = "merge"
)

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

		// Due to a bug in `mergo.Merge`
		// (Note: in the `for` loop, it DOES modify the source of the previous loop iteration if it's a complex map and `mergo` gets a pointer to it,
		// not only the destination of the current loop iteration),
		// we don't give it our maps directly; we convert them to YAML strings and then back to `Go` maps,
		// so `mergo` does not have access to the original pointers
		yamlCurrent, err := u.ConvertToYAML(current)
		if err != nil {
			_, _ = theme.Colors.Error.Fprintln(color.Error, err.Error()+"\n")
			return nil, err
		}

		dataCurrent, err := u.UnmarshalYAML[any](yamlCurrent)
		if err != nil {
			_, _ = theme.Colors.Error.Fprintln(color.Error, err.Error()+"\n")
			return nil, err
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
			_, _ = theme.Colors.Error.Fprintln(color.Error, err.Error()+"\n")
			return nil, err
		}
	}

	return merged, nil
}

// Merge takes a list of maps as input, deep-merges the items in the order they are defined in the list, and returns a single map with the merged contents.
func Merge(
	atmosConfig schema.AtmosConfiguration,
	inputs []map[string]any,
) (map[string]any, error) {
	if atmosConfig.Settings.ListMergeStrategy == "" {
		atmosConfig.Settings.ListMergeStrategy = ListMergeStrategyReplace
	}

	if atmosConfig.Settings.ListMergeStrategy != ListMergeStrategyReplace &&
		atmosConfig.Settings.ListMergeStrategy != ListMergeStrategyAppend &&
		atmosConfig.Settings.ListMergeStrategy != ListMergeStrategyMerge {
		return nil, fmt.Errorf("invalid Atmos manifests list merge strategy '%s'.\n"+
			"Supported list merge strategies are: %s.",
			atmosConfig.Settings.ListMergeStrategy,
			fmt.Sprintf("%s, %s, %s", ListMergeStrategyReplace, ListMergeStrategyAppend, ListMergeStrategyMerge))
	}

	sliceDeepCopy := false
	appendSlice := false

	if atmosConfig.Settings.ListMergeStrategy == ListMergeStrategyMerge {
		sliceDeepCopy = true
	} else if atmosConfig.Settings.ListMergeStrategy == ListMergeStrategyAppend {
		appendSlice = true
	}

	return MergeWithOptions(inputs, appendSlice, sliceDeepCopy)
}
