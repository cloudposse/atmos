package validator

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/xeipuuv/gojsonschema"
	"go.yaml.in/yaml/v3"

	"github.com/cloudposse/atmos/pkg/datafetcher"
	"github.com/cloudposse/atmos/pkg/schema"
)

var ErrSchemaNotFound = errors.New("failed to fetch schema")

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_$GOFILE -package=$GOPACKAGE
type Validator interface {
	ValidateYAMLSchema(schema, sourceFile string) ([]gojsonschema.ResultError, error)
	// ValidateYAMLContent validates in-memory YAML content (e.g. an unsaved
	// editor buffer) instead of a fetched source.
	ValidateYAMLContent(schema string, yamlContent []byte) ([]gojsonschema.ResultError, error)
}

type yamlSchemaValidator struct {
	atmosConfig *schema.AtmosConfiguration
	dataFetcher datafetcher.DataFetcher
}

func NewYAMLSchemaValidator(atmosConfig *schema.AtmosConfiguration) Validator {
	return &yamlSchemaValidator{
		atmosConfig: atmosConfig,
		dataFetcher: datafetcher.NewDataFetcher(atmosConfig),
	}
}

// yamlToJSON converts YAML data to JSON format in an optimized way.
func yamlToJSON(yamlData []byte) ([]byte, error) {
	var node yaml.Node
	err := yaml.Unmarshal(yamlData, &node)
	if err != nil {
		return nil, err
	}
	rawData := yamlNodeToInterface(&node)
	// Marshal the processed structure into JSON
	return json.Marshal(rawData)
}

func yamlNodeToInterface(node *yaml.Node) any {
	if node == nil {
		return nil
	}

	switch node.Kind {
	case yaml.DocumentNode:
		if len(node.Content) == 0 {
			return nil
		}
		return yamlNodeToInterface(node.Content[0])
	case yaml.MappingNode:
		return yamlMappingNodeToInterface(node)
	case yaml.SequenceNode:
		return yamlSequenceNodeToInterface(node)
	case yaml.AliasNode:
		return yamlNodeToInterface(node.Alias)
	case yaml.ScalarNode:
		return yamlScalarNodeToInterface(node)
	default:
		return nil
	}
}

func yamlMappingNodeToInterface(node *yaml.Node) map[string]any {
	result := make(map[string]any, len(node.Content)/2)
	for i := 0; i < len(node.Content); i += 2 {
		key := yamlNodeToInterface(node.Content[i])
		result[fmt.Sprint(key)] = yamlNodeToInterface(node.Content[i+1])
	}
	return result
}

func yamlSequenceNodeToInterface(node *yaml.Node) []any {
	result := make([]any, 0, len(node.Content))
	for _, child := range node.Content {
		result = append(result, yamlNodeToInterface(child))
	}
	return result
}

func yamlScalarNodeToInterface(node *yaml.Node) any {
	if isCustomYAMLTag(node.Tag) {
		return strings.TrimSpace(node.Tag + " " + node.Value)
	}
	var value any
	if err := node.Decode(&value); err == nil {
		return value
	}
	return node.Value
}

func isCustomYAMLTag(tag string) bool {
	return tag != "" && !strings.HasPrefix(tag, "!!")
}

func (y yamlSchemaValidator) ValidateYAMLSchema(schemaSource, yamlSource string) ([]gojsonschema.ResultError, error) {
	yamlData, err := y.dataFetcher.GetData(yamlSource)
	if err != nil {
		return nil, err
	}
	return y.ValidateYAMLContent(schemaSource, yamlData)
}

// ValidateYAMLContent validates raw YAML content against the schema fetched
// from schemaSource. Custom YAML function tags (e.g. `!include`) are
// stringified before validation, matching ValidateYAMLSchema.
func (y yamlSchemaValidator) ValidateYAMLContent(schemaSource string, yamlContent []byte) ([]gojsonschema.ResultError, error) {
	data, err := yamlToJSON(yamlContent)
	if err != nil {
		return nil, err
	}
	if schemaSource == "" {
		schemaSource, err = y.getSchemaSourceFromYAML(data)
		if err != nil {
			return nil, err
		}
	}
	schemaData, err := y.dataFetcher.GetData(schemaSource)
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
