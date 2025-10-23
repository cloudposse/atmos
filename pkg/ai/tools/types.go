package tools

import (
	"context"
)

// Tool represents an executable operation that AI can perform.
type Tool interface {
	// Name returns the unique tool name.
	Name() string

	// Description returns a description of what the tool does.
	Description() string

	// Parameters returns the list of parameters this tool accepts.
	Parameters() []Parameter

	// Execute runs the tool with the given parameters.
	Execute(ctx context.Context, params map[string]interface{}) (*Result, error)

	// RequiresPermission returns true if this tool needs user permission.
	RequiresPermission() bool

	// IsRestricted returns true if this tool is always restricted (requires confirmation).
	IsRestricted() bool
}

// Parameter defines a tool parameter.
type Parameter struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Type        ParamType   `json:"type"`
	Required    bool        `json:"required"`
	Default     interface{} `json:"default,omitempty"`
}

// ParamType represents the type of a parameter.
type ParamType string

const (
	// ParamTypeString is a string parameter.
	ParamTypeString ParamType = "string"
	// ParamTypeInt is an integer parameter.
	ParamTypeInt ParamType = "int"
	// ParamTypeBool is a boolean parameter.
	ParamTypeBool ParamType = "bool"
	// ParamTypeArray is an array parameter.
	ParamTypeArray ParamType = "array"
	// ParamTypeObject is an object parameter.
	ParamTypeObject ParamType = "object"
)

// Result contains the result of tool execution.
type Result struct {
	Success bool                   `json:"success"`
	Output  string                 `json:"output"`
	Error   error                  `json:"error,omitempty"`
	Data    map[string]interface{} `json:"data,omitempty"`
}

// Category represents a tool category.
type Category string

const (
	// CategoryAtmos represents Atmos-specific tools.
	CategoryAtmos Category = "atmos"
	// CategoryFile represents file operation tools.
	CategoryFile Category = "file"
	// CategorySystem represents system operation tools.
	CategorySystem Category = "system"
	// CategoryMCP represents MCP-provided tools.
	CategoryMCP Category = "mcp"
)
