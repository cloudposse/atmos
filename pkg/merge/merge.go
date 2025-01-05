package merge

import (
	"dario.cat/mergo"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/fatih/color"
)

// MergeInterfaces merges multiple interfaces into a single interface
func MergeInterfaces(atmosConfig schema.AtmosConfiguration, interfaces ...interface{}) (interface{}, error) {
	var merged interface{}

	opts := []func(*mergo.Config){
		mergo.WithOverride,
		mergo.WithAppendSlice,
	}

	for _, current := range interfaces {
		if current == nil {
			continue
		}

		yamlCurrent, err := u.ConvertToYAML(current)
		if err != nil {
			c := theme.Colors.Error
			_, _ = c.Fprintln(color.Error, err.Error()+"\n")
			return nil, err
		}

		dataCurrent, err := u.UnmarshalYAML[any](yamlCurrent)
		if err != nil {
			c := theme.Colors.Error
			_, _ = c.Fprintln(color.Error, err.Error()+"\n")
			return nil, err
		}

		if merged == nil {
			merged = dataCurrent
			continue
		}

		if err = mergo.Merge(&merged, dataCurrent, opts...); err != nil {
			c := theme.Colors.Error
			_, _ = c.Fprintln(color.Error, err.Error()+"\n")
			return nil, err
		}
	}

	return merged, nil
}
