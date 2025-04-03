package filetype

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/hcl"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/zclconf/go-cty/cty"
	"gopkg.in/yaml.v3"
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

	// Ensure that the parsed result is not nil and has some meaningful content
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

// DetectFormatAndParseFile detects the format of the file (JSON, YAML, HCL) and parses the file into a Go type
// For all other formats, it just reads the file and returns the content as a string
func DetectFormatAndParseFile(readFileFunc func(string) ([]byte, error), filename string) (any, error) {
	var v any

	var err error

	d, err := readFileFunc(filename)
	if err != nil {
		return nil, err
	}

	data := string(d)
	switch {
	case IsJSON(data):
		err = json.Unmarshal(d, &v)
		if err != nil {
			return nil, err
		}
	case IsHCL(data):
		parser := hclparse.NewParser()
		file, diags := parser.ParseHCL(d, filename)
		if diags != nil && diags.HasErrors() {
			return nil, fmt.Errorf("%w, file: %s, error: %s", ErrFailedToProcessHclFile, filename, diags.Error())
		}
		if file == nil {
			return nil, fmt.Errorf("%w, file: %s, file parsing returned nil", ErrFailedToProcessHclFile, filename)
		}

		// Extract all attributes from the file body
		attributes, diags := file.Body.JustAttributes()
		if diags != nil && diags.HasErrors() {
			return nil, fmt.Errorf("%w, file: %s, error: %s", ErrFailedToProcessHclFile, filename, diags.Error())
		}

		// Map to store the parsed attribute values
		result := make(map[string]any)

		// Evaluate each attribute and store it in the result map
		for name, attr := range attributes {
			ctyValue, diags := attr.Expr.Value(nil)
			if diags != nil && diags.HasErrors() {
				return nil, fmt.Errorf("%w, file: %s, error: %s", ErrFailedToProcessHclFile, filename, diags.Error())
			}

			// Convert cty.Value to appropriate Go type
			result[name] = ctyToGo(ctyValue)
		}
		v = result
	case IsYAML(data):
		err = yaml.Unmarshal(d, &v)
		if err != nil {
			return nil, err
		}
	default:
		v = data
	}

	return v, nil
}

// CtyToGo converts cty.Value to Go types.
func ctyToGo(value cty.Value) any {
	switch {
	case value.Type().IsObjectType(): // Handle maps
		m := map[string]any{}
		for k, v := range value.AsValueMap() {
			m[k] = ctyToGo(v)
		}
		return m

	case value.Type().IsListType() || value.Type().IsTupleType(): // Handle lists
		var list []any
		for _, v := range value.AsValueSlice() {
			list = append(list, ctyToGo(v))
		}
		return list

	case value.Type() == cty.String: // Handle strings
		return value.AsString()

	case value.Type() == cty.Number: // Handle numbers
		if n, _ := value.AsBigFloat().Int64(); true {
			return n // Convert to int64 if possible
		}
		return value.AsBigFloat() // Otherwise, keep as float64

	case value.Type() == cty.Bool: // Handle booleans
		return value.True()

	default:
		return value // Return as-is for unsupported types
	}
}
