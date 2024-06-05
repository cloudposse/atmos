package merge

import (
	"fmt"

	"dario.cat/mergo"
	"github.com/fatih/color"
	"gopkg.in/yaml.v2"

	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	ListMergeStrategyReplace = "replace"
	ListMergeStrategyAppend  = "append"
	ListMergeStrategyMerge   = "merge"
)

// MergeWithOptions takes a list of maps and options as input, deep-merges the items in the order they are defined in the list,
// and returns a single map with the merged contents
func MergeWithOptions(inputs []map[any]any, appendSlice, sliceDeepCopy bool) (map[any]any, error) {
	merged := map[any]any{}

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
		yamlCurrent, err := yaml.Marshal(current)
		if err != nil {
			c := color.New(color.FgRed)
			_, _ = c.Fprintln(color.Error, err.Error()+"\n")
			return nil, err
		}

		var dataCurrent map[any]any
		if err = yaml.Unmarshal(yamlCurrent, &dataCurrent); err != nil {
			c := color.New(color.FgRed)
			_, _ = c.Fprintln(color.Error, err.Error()+"\n")
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
			c := color.New(color.FgRed)
			_, _ = c.Fprintln(color.Error, err.Error()+"\n")
			return nil, err
		}
	}

	return merged, nil
}

// Merge takes a list of maps as input, deep-merges the items in the order they are defined in the list, and returns a single map with the merged contents
func Merge(
	cliConfig schema.CliConfiguration,
	inputs []map[any]any,
) (map[any]any, error) {
	if cliConfig.Settings.ListMergeStrategy == "" {
		cliConfig.Settings.ListMergeStrategy = ListMergeStrategyReplace
	}

	if cliConfig.Settings.ListMergeStrategy != ListMergeStrategyReplace &&
		cliConfig.Settings.ListMergeStrategy != ListMergeStrategyAppend &&
		cliConfig.Settings.ListMergeStrategy != ListMergeStrategyMerge {
		return nil, fmt.Errorf("invalid Atmos manifests list merge strategy '%s'.\n"+
			"Supported list merge strategies are: %s.",
			cliConfig.Settings.ListMergeStrategy,
			fmt.Sprintf("%s, %s, %s", ListMergeStrategyReplace, ListMergeStrategyAppend, ListMergeStrategyMerge))
	}

	sliceDeepCopy := false
	appendSlice := false

	if cliConfig.Settings.ListMergeStrategy == ListMergeStrategyMerge {
		sliceDeepCopy = true
	} else if cliConfig.Settings.ListMergeStrategy == ListMergeStrategyAppend {
		appendSlice = true
	}

	return MergeWithOptions(inputs, appendSlice, sliceDeepCopy)
}
