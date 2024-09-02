package convert

import (
	"encoding/json"

	"github.com/cloudposse/atmos/pkg/schema"
)

// JSONToMapOfInterfaces takes a JSON string as input and returns a map[string]any
func JSONToMapOfInterfaces(input string) (schema.AtmosSectionMapType, error) {
	var data schema.AtmosSectionMapType
	byt := []byte(input)

	if err := json.Unmarshal(byt, &data); err != nil {
		return nil, err
	}
	return data, nil
}
