package validator

import (
	"encoding/json"

	"github.com/xeipuuv/gojsonschema"
	"gopkg.in/yaml.v3"
)

// yamlToJSON converts YAML data to JSON format in an optimized way.
func yamlToJSON(yamlData []byte) ([]byte, error) {
	var rawData interface{}
	err := yaml.Unmarshal(yamlData, &rawData)
	if err != nil {
		return nil, err
	}
	// Marshal the processed structure into JSON
	return json.Marshal(rawData)
}

func ValidateYAMLSchema(schemaSource, yamlSource string) ([]gojsonschema.ResultError, error) {
	schemaData, err := GetData(schemaSource)
	if err != nil {
		return nil, err
	}
	yamlData, err := GetData(yamlSource)
	if err != nil {
		return nil, err
	}
	data, err := yamlToJSON(yamlData)
	if err != nil {
		return nil, err
	}
	schemaLoader := gojsonschema.NewStringLoader(string(schemaData))
	dataLoader := gojsonschema.NewStringLoader(string(data))

	result, err := gojsonschema.Validate(schemaLoader, dataLoader)
	if err != nil {
		return nil, err
	}
	return result.Errors(), nil
}
