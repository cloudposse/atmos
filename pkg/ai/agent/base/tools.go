package base

import (
	"github.com/cloudposse/atmos/pkg/ai/tools"
)

// ToolPropertySchema represents a JSON Schema property for a tool parameter.
type ToolPropertySchema struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

// ToolParameterSchema represents the full JSON Schema for tool parameters.
type ToolParameterSchema struct {
	Type       string                        `json:"type"`
	Properties map[string]ToolPropertySchema `json:"properties"`
	Required   []string                      `json:"required"`
}

// BuildToolParameterSchema builds a JSON Schema from tool parameters.
// This is the common logic used by all providers for tool parameter conversion.
func BuildToolParameterSchema(toolParams []tools.Parameter) (properties map[string]interface{}, required []string) {
	properties = make(map[string]interface{})
	required = make([]string, 0)

	for _, param := range toolParams {
		properties[param.Name] = map[string]interface{}{
			"type":        string(param.Type),
			"description": param.Description,
		}
		if param.Required {
			required = append(required, param.Name)
		}
	}

	return properties, required
}

// ToolInfo contains basic tool information extracted for conversion.
type ToolInfo struct {
	Name        string
	Description string
	Properties  map[string]interface{}
	Required    []string
}

// ExtractToolInfo extracts common tool information from a Tool interface.
// This provides a single point of extraction that all providers can use.
func ExtractToolInfo(tool tools.Tool) ToolInfo {
	properties, required := BuildToolParameterSchema(tool.Parameters())
	return ToolInfo{
		Name:        tool.Name(),
		Description: tool.Description(),
		Properties:  properties,
		Required:    required,
	}
}

// ExtractAllToolInfo extracts tool information from a slice of tools.
func ExtractAllToolInfo(availableTools []tools.Tool) []ToolInfo {
	result := make([]ToolInfo, 0, len(availableTools))
	for _, tool := range availableTools {
		result = append(result, ExtractToolInfo(tool))
	}
	return result
}
