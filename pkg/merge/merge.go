package merge

import (
	"github.com/imdario/mergo"
	"gopkg.in/yaml.v2"
)

// Merge takes a list of maps of interface as input and returns a single map with the merged contents
func Merge(inputs []map[interface{}]interface{}) (map[interface{}]interface{}, error) {
	merged := map[interface{}]interface{}{}

	for index := range inputs {
		current := inputs[index]

		// Due to a bug in `mergo.Merge`
		// (in the `for` loop, it DOES modify the source of the previous loop iteration if it's a complex map and `mergo` get a pointer to it,
		// not only the destination of the current loop iteration),
		// we don't give it our maps directly; we convert them to YAML strings and then back to `Go` maps,
		// so `mergo` does not have access to the original pointers
		yamlCurrent, err := yaml.Marshal(current)
		if err != nil {
			return nil, err
		}

		var dataCurrent map[interface{}]interface{}
		if err = yaml.Unmarshal(yamlCurrent, &dataCurrent); err != nil {
			return nil, err
		}

		if err = mergo.Merge(&merged, dataCurrent, mergo.WithOverride, mergo.WithOverwriteWithEmptyValue, mergo.WithTypeCheck); err != nil {
			return nil, err
		}
	}

	return merged, nil
}
