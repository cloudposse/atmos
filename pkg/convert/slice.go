package convert

import "github.com/pkg/errors"

// SliceOfInterfacesToSliceOfStrings takes a slice of interfaces and converts it to a slice of strings
func SliceOfInterfacesToSliceOfStrings(input []any) ([]string, error) {
	if input == nil {
		return nil, errors.New("input must not be nil")
	}

	output := make([]string, 0)
	for _, current := range input {
		output = append(output, current.(string))
	}

	return output, nil
}

// SliceOfMapsOfStringsToSliceOfMapsOfInterfaces takes a slice of map[string]any and returns a slice of map[any]any
func SliceOfMapsOfStringsToSliceOfMapsOfInterfaces(input []map[string]any) []map[any]any {
	output := make([]map[any]any, 0)
	for k, v := range input {
		output = append(output, map[any]any{k: v})
	}
	return output
}
