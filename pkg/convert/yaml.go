package convert

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/schema"
)

// YAMLToMapOfStrings takes a YAML string as input and unmarshals it into a map[string]any
func YAMLToMapOfStrings(input string) (schema.AtmosSectionMapType, error) {
	var data map[string]any
	byt := []byte(input)

	if err := yaml.Unmarshal(byt, &data); err != nil {
		return nil, err
	}
	return data, nil
}

type CustomType struct {
	Value string
}

func (c CustomType) MarshalYAML() (any, error) {
	// Return a YAML node or a plain value.
	node := yaml.Node{
		Kind:  yaml.ScalarNode,
		Tag:   "!!str",
		Value: "custom:" + c.Value,
	}
	return &node, nil
}

// UnmarshalYAML implements the yaml.Unmarshaler interface
func (c *CustomType) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode && value.Tag == "!!str" && len(value.Value) > 7 && value.Value[:7] == "custom:" {
		c.Value = value.Value[7:]
		return nil
	}
	return fmt.Errorf("unexpected YAML value: %v", value.Value)
}
