package convert

import (
	"gopkg.in/yaml.v3"
)

// YAMLToMapOfStrings takes a YAML string as input and returns a map[string]any
func YAMLToMapOfStrings(input string) (map[string]any, error) {
	var data map[string]any
	byt := []byte(input)

	if err := yaml.Unmarshal(byt, &data); err != nil {
		return nil, err
	}
	return data, nil
}
