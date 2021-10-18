package convert

import "github.com/pkg/errors"

// SliceOfInterfacesToSliceOfStrings takes a slice of interfaces and converts it to a slice of strings
func SliceOfInterfacesToSliceOfStrings(input []interface{}) ([]string, error) {
	if input == nil {
		return nil, errors.New("input must not be nil")
	}

	output := make([]string, 0)
	for _, current := range input {
		output = append(output, current.(string))
	}

	return output, nil
}

// SliceOfMapsOfStringsToSliceOfMapsOfInterfaces takes a slice of map[string]interface{} and returns a slice of map[interface{}]interface{}
func SliceOfMapsOfStringsToSliceOfMapsOfInterfaces(input []map[string]interface{}) []map[interface{}]interface{} {
	output := make([]map[interface{}]interface{}, 0)
	for k, v := range input {
		output = append(output, map[interface{}]interface{}{k: v})
	}
	return output
}
