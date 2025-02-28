package validator

import (
	"encoding/json"
	"errors"

	"github.com/cloudposse/atmos/pkg/datafetcher"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/xeipuuv/gojsonschema"
	"gopkg.in/yaml.v3"
)

var ErrSchemaNotFound = errors.New("failed to fetch schema")

//go:generate mockgen -source=$GOFILE -destination=mock_$GOFILE -package=$GOPACKAGE
type Validator interface {
	ValidateYAMLSchema(schema, sourceFile, sourceKey string) ([]gojsonschema.ResultError, error)
}

type yamlSchemaValidator struct {
	atmosConfig *schema.AtmosConfiguration
	dataFetcher datafetcher.DataFetcher
}

func NewYAMLSchemaValidator(atmosConfig *schema.AtmosConfiguration) Validator {
	return &yamlSchemaValidator{
		atmosConfig: atmosConfig,
		dataFetcher: datafetcher.NewDataFetcher(),
	}
}

// yamlToJSON converts YAML data to JSON format in an optimized way.
func yamlToJSON(yamlData []byte, yamlKey string) ([]byte, error) {
	var rawData any
	err := yaml.Unmarshal(yamlData, &rawData)
	if err != nil {
		return nil, err
	}
	if yamlKey != "" {
		rawData = rawData.(map[string]any)[yamlKey]
	}
	// Marshal the processed structure into JSON
	return json.Marshal(rawData)
}

func (y yamlSchemaValidator) ValidateYAMLSchema(schemaSource, yamlSource, yamlKey string) ([]gojsonschema.ResultError, error) {
	yamlData, err := y.dataFetcher.GetData(y.atmosConfig, yamlSource)
	if err != nil {
		return nil, err
	}
	data, err := yamlToJSON(yamlData, yamlKey)
	if err != nil {
		return nil, err
	}
	if schemaSource == "" {
		schemaSource, err = y.getSchemaSourceFromYAML(data)
		if err != nil {
			return nil, err
		}
	}
	schemaData, err := y.dataFetcher.GetData(y.atmosConfig, schemaSource)
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

func (v yamlSchemaValidator) getSchemaSourceFromYAML(data []byte) (string, error) {
	if data == nil {
		return "", ErrSchemaNotFound
	}
	var yamlData any
	err := json.Unmarshal(data, &yamlData)
	if err != nil {
		return "", ErrSchemaNotFound
	}
	yamlGenericData := yamlData.(map[string]any)
	if val, ok := yamlGenericData["schema"]; ok && val != "" {
		if schema, ok := val.(string); ok {
			return schema, nil
		}
	}
	return "", ErrSchemaNotFound
}
