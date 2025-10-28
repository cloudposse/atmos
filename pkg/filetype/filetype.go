package filetype

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/hcl"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/zclconf/go-cty/cty"
	"go.yaml.in/yaml/v3"
)

var ErrFailedToProcessHclFile = errors.New("failed to process HCL file")

// IsYAML checks if data is in YAML format.
func IsYAML(data string) bool {
	if strings.TrimSpace(data) == "" {
		return false
	}

	var yml any
	err := yaml.Unmarshal([]byte(data), &yml)
	if err != nil {
		return false
	}

	// Ensure that the parsed result is not nil and has some meaningful content.
	_, isMap := yml.(map[string]any)
	_, isSlice := yml.([]any)

	return isMap || isSlice
}

// IsHCL checks if data is in HCL format.
func IsHCL(data string) bool {
	if strings.TrimSpace(data) == "" {
		return false
	}

	var hclData any
	return hcl.Unmarshal([]byte(data), &hclData) == nil
}

// IsJSON checks if data is in JSON format.
func IsJSON(data string) bool {
	if strings.TrimSpace(data) == "" {
		return false
	}

	var js json.RawMessage
	return json.Unmarshal([]byte(data), &js) == nil
}

// DetectFormatAndParseFile detects the format of the file (JSON, YAML, HCL) and parses the file into a Go type.
// For all other formats, it just reads the file and returns the content as a string.
func DetectFormatAndParseFile(readFileFunc func(string) ([]byte, error), filename string) (any, error) {
	d, err := readFileFunc(filename)
	if err != nil {
		return nil, err
	}

	data := string(d)
	switch {
	case IsJSON(data):
		return parseJSON(d)
	case IsHCL(data):
		return parseHCL(d, filename)
	case IsYAML(data):
		return parseYAML(d)
	default:
		return data, nil
	}
}

func parseJSON(data []byte) (any, error) {
	var v any
	err := json.Unmarshal(data, &v)
	if err != nil {
		return nil, err
	}
	return v, nil
}

func parseYAML(data []byte) (any, error) {
	// First, unmarshal into a yaml.Node to preserve the original structure.
	var node yaml.Node
	err := yaml.Unmarshal(data, &node)
	if err != nil {
		return nil, err
	}

	// Process the node to ensure strings starting with '#' are properly handled.
	processYAMLNode(&node)

	// Decode the processed node into a Go value.
	var v any
	err = node.Decode(&v)
	if err != nil {
		return nil, err
	}
	return v, nil
}

func processYAMLNode(node *yaml.Node) {
	if node == nil {
		return
	}

	if node.Kind == yaml.ScalarNode && node.Tag == "!!str" && strings.HasPrefix(node.Value, "#") {
		node.Style = yaml.SingleQuotedStyle
	}

	for _, child := range node.Content {
		processYAMLNode(child)
	}
}

func parseHCL(data []byte, filename string) (any, error) {
	parser := hclparse.NewParser()
	file, diags := parser.ParseHCL(data, filename)
	if diags != nil && diags.HasErrors() {
		return nil, fmt.Errorf("%w, file: %s, error: %s", ErrFailedToProcessHclFile, filename, diags.Error())
	}
	if file == nil {
		return nil, fmt.Errorf("%w, file: %s, file parsing returned nil", ErrFailedToProcessHclFile, filename)
	}

	attributes, diags := file.Body.JustAttributes()
	if diags != nil && diags.HasErrors() {
		return nil, fmt.Errorf("%w, file: %s, error: %s", ErrFailedToProcessHclFile, filename, diags.Error())
	}

	result := make(map[string]any)
	for name, attr := range attributes {
		ctyValue, diags := attr.Expr.Value(nil)
		if diags != nil && diags.HasErrors() {
			return nil, fmt.Errorf("%w, file: %s, error: %s", ErrFailedToProcessHclFile, filename, diags.Error())
		}
		result[name] = ctyToGo(ctyValue)
	}
	return result, nil
}

// ctyToGo converts cty.Value to Go types.
func ctyToGo(value cty.Value) any {
	switch value.Type() {
	case cty.String:
		return value.AsString()
	case cty.Number:
		if n, _ := value.AsBigFloat().Int64(); true {
			return n
		}
		return value.AsBigFloat()
	case cty.Bool:
		return value.True()
	}

	if value.Type().IsObjectType() {
		m := map[string]any{}
		for k, v := range value.AsValueMap() {
			m[k] = ctyToGo(v)
		}
		return m
	}

	if value.Type().IsListType() || value.Type().IsTupleType() {
		var list []any
		for _, v := range value.AsValueSlice() {
			list = append(list, ctyToGo(v))
		}
		return list
	}

	return value
}
