package convert

import (
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/schema"
)

// YAMLToMapOfStrings takes a YAML string as input and unmarshals it into a map[string]any
func YAMLToMapOfStrings(input string) (schema.AtmosSectionMapType, error) {
	var data schema.AtmosSectionMapType
	b := []byte(input)

	if err := yaml.Unmarshal(b, &data); err != nil {
		return nil, err
	}
	return data, nil
}
