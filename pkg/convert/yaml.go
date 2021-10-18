package convert

import (
	"gopkg.in/yaml.v2"
)

// YAMLToMapOfInterfaces takes a YAML string as input and returns a map[interface{}]interface{}
func YAMLToMapOfInterfaces(input string) (map[interface{}]interface{}, error) {
	var data map[interface{}]interface{}
	byt := []byte(input)

	if err := yaml.Unmarshal(byt, &data); err != nil {
		return nil, err
	}
	return data, nil
}

// YAMLSliceOfInterfaceToSliceOfMaps takes a slice of interfaces as input and returns a slice of map[interface{}]interface{}
func YAMLSliceOfInterfaceToSliceOfMaps(input []interface{}) ([]map[interface{}]interface{}, error) {
	output := make([]map[interface{}]interface{}, 0)
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
