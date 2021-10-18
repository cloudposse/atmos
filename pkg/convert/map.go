package convert

// MapsOfStringsToMapsOfInterfaces takes map[string]interface{} and returns map[interface{}]interface{}
func MapsOfStringsToMapsOfInterfaces(input map[string]interface{}) map[interface{}]interface{} {
	output := map[interface{}]interface{}{}
	for k, v := range input {
		output[k] = v
	}
	return output
}

// MapsOfInterfacesToMapsOfStrings takes map[interface{}]interface{} and returns map[string]interface{}
func MapsOfInterfacesToMapsOfStrings(input map[interface{}]interface{}) map[string]interface{} {
	output := map[string]interface{}{}
	for k, v := range input {
		output[k.(string)] = v
	}
	return output
}
