package validator

import (
	"encoding/json"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/xeipuuv/gojsonschema"
	"gopkg.in/yaml.v3"
)

//go:generate mockgen -source=$GOFILE -destination=mock_$GOFILE -package=$GOPACKAGE
type Validator interface {
	ValidateYAMLSchema(schema, source string) ([]gojsonschema.ResultError, error)
}

type yamlSchemaValidator struct {
	atmosConfig *schema.AtmosConfiguration
	dataFetcher DataFetcher
}

func NewYAMLSchemaValidator(atmosConfig *schema.AtmosConfiguration) Validator {
	return &yamlSchemaValidator{
		atmosConfig: atmosConfig,
		dataFetcher: NewDataFetcher(),
	}
}

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

func (y yamlSchemaValidator) ValidateYAMLSchema(schemaSource, yamlSource string) ([]gojsonschema.ResultError, error) {
	schemaData, err := y.dataFetcher.GetData(y.atmosConfig, schemaSource)
	if err != nil {
		return nil, err
	}
	yamlData, err := y.dataFetcher.GetData(y.atmosConfig, yamlSource)
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
