package convert

import (
	"gopkg.in/yaml.v2"
)

// YAMLToMapOfInterfaces takes a YAML string as input and returns a map[any]any
func YAMLToMapOfInterfaces(input string) (map[any]any, error) {
	var data map[any]any
	byt := []byte(input)

	if err := yaml.Unmarshal(byt, &data); err != nil {
		return nil, err
	}
	return data, nil
}

// YAMLSliceOfInterfaceToSliceOfMaps takes a slice of interfaces as input and returns a slice of map[any]any
func YAMLSliceOfInterfaceToSliceOfMaps(input []any) ([]map[any]any, error) {
	output := make([]map[any]any, 0)
	for _, current := range input {
		// Apply YAMLToMap only if string is passed
		if currentYaml, ok := current.(string); ok {
			data, err := YAMLToMapOfInterfaces(currentYaml)
			if err != nil {
				return nil, err
			}
			output = append(output, data)
		}
	}
	return output, nil
}
