package convert

// MapsOfStringsToMapsOfInterfaces takes map[string]any and returns map[any]any
func MapsOfStringsToMapsOfInterfaces(input map[string]any) map[any]any {
	output := map[any]any{}
	for k, v := range input {
		output[k] = v
	}
	return output
}

// MapsOfInterfacesToMapsOfStrings takes map[any]any and returns map[string]any
func MapsOfInterfacesToMapsOfStrings(input map[any]any) map[string]any {
	output := map[string]any{}
	for k, v := range input {
		output[k.(string)] = v
	}
	return output
}
