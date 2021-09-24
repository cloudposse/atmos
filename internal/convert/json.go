package convert

import "encoding/json"

// JSONToMapOfInterfaces takes a JSON string as input and returns a map[string]interface{}
func JSONToMapOfInterfaces(input string) (map[string]interface{}, error) {
	var data map[string]interface{}
	byt := []byte(input)

	if err := json.Unmarshal(byt, &data); err != nil {
		return nil, err
	}
	return data, nil
}

// JSONSliceOfInterfaceToSliceOfMaps takes a slice of JSON strings as input and returns a slice of map[interface{}]interface{}
func JSONSliceOfInterfaceToSliceOfMaps(input []interface{}) ([]map[interface{}]interface{}, error) {
	outputMap := make([]map[interface{}]interface{}, 0)
	for _, current := range input {
		data, err := JSONToMapOfInterfaces(current.(string))
		if err != nil {
			return nil, err
		}

		map2 := map[interface{}]interface{}{}

		for k, v := range data {
			map2[k] = v
		}

		outputMap = append(outputMap, map2)
	}
	return outputMap, nil
}
