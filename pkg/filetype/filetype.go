package filetype

import (
	"encoding/json"
	"strings"

	"github.com/hashicorp/hcl"
	"gopkg.in/yaml.v2"
)

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
