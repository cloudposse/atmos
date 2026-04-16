package base

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/ai/types"
)

// mockTool implements the tools.Tool interface for testing.
type mockTool struct {
	name               string
	description        string
	parameters         []tools.Parameter
	requiresPermission bool
	isRestricted       bool
}

func (m *mockTool) Name() string                  { return m.name }
func (m *mockTool) Description() string           { return m.description }
func (m *mockTool) Parameters() []tools.Parameter { return m.parameters }
func (m *mockTool) RequiresPermission() bool      { return m.requiresPermission }
func (m *mockTool) IsRestricted() bool            { return m.isRestricted }
func (m *mockTool) Execute(_ context.Context, _ map[string]interface{}) (*tools.Result, error) {
	return &tools.Result{Success: true}, nil
}

// PrependSystemMessages tests.

func TestPrependSystemMessages_BothEmpty(t *testing.T) {
	messages := []types.Message{
		{Role: types.RoleUser, Content: "Hello"},
	}

	result := PrependSystemMessages("", "", messages)

	require.Len(t, result, 1)
	assert.Equal(t, types.RoleUser, result[0].Role)
	assert.Equal(t, "Hello", result[0].Content)
}

func TestPrependSystemMessages_OnlySystemPrompt(t *testing.T) {
	messages := []types.Message{
		{Role: types.RoleUser, Content: "Hello"},
	}

	result := PrependSystemMessages("You are helpful.", "", messages)

	require.Len(t, result, 2)
	assert.Equal(t, types.RoleSystem, result[0].Role)
	assert.Equal(t, "You are helpful.", result[0].Content)
	assert.Equal(t, types.RoleUser, result[1].Role)
	assert.Equal(t, "Hello", result[1].Content)
}

func TestPrependSystemMessages_OnlyAtmosMemory(t *testing.T) {
	messages := []types.Message{
		{Role: types.RoleUser, Content: "Hello"},
	}

	result := PrependSystemMessages("", "ATMOS.md content here.", messages)

	require.Len(t, result, 2)
	assert.Equal(t, types.RoleSystem, result[0].Role)
	assert.Equal(t, "ATMOS.md content here.", result[0].Content)
	assert.Equal(t, types.RoleUser, result[1].Role)
}

func TestPrependSystemMessages_BothProvided(t *testing.T) {
	messages := []types.Message{
		{Role: types.RoleUser, Content: "Hello"},
		{Role: types.RoleAssistant, Content: "Hi there!"},
	}

	result := PrependSystemMessages("You are helpful.", "ATMOS.md memory.", messages)

	require.Len(t, result, 4)
	// System prompt first.
	assert.Equal(t, types.RoleSystem, result[0].Role)
	assert.Equal(t, "You are helpful.", result[0].Content)
	// Atmos memory second.
	assert.Equal(t, types.RoleSystem, result[1].Role)
	assert.Equal(t, "ATMOS.md memory.", result[1].Content)
	// Original messages follow.
	assert.Equal(t, types.RoleUser, result[2].Role)
	assert.Equal(t, "Hello", result[2].Content)
	assert.Equal(t, types.RoleAssistant, result[3].Role)
	assert.Equal(t, "Hi there!", result[3].Content)
}

func TestPrependSystemMessages_EmptyHistory(t *testing.T) {
	result := PrependSystemMessages("System prompt.", "Memory.", []types.Message{})

	require.Len(t, result, 2)
	assert.Equal(t, types.RoleSystem, result[0].Role)
	assert.Equal(t, "System prompt.", result[0].Content)
	assert.Equal(t, types.RoleSystem, result[1].Role)
	assert.Equal(t, "Memory.", result[1].Content)
}

func TestPrependSystemMessages_NilHistory(t *testing.T) {
	result := PrependSystemMessages("System prompt.", "", nil)

	require.Len(t, result, 1)
	assert.Equal(t, types.RoleSystem, result[0].Role)
	assert.Equal(t, "System prompt.", result[0].Content)
}

func TestPrependSystemMessages_AllEmpty(t *testing.T) {
	result := PrependSystemMessages("", "", []types.Message{})

	assert.NotNil(t, result)
	assert.Empty(t, result)
}

func TestPrependSystemMessages_PreservesOrder(t *testing.T) {
	messages := []types.Message{
		{Role: types.RoleUser, Content: "First"},
		{Role: types.RoleAssistant, Content: "Second"},
		{Role: types.RoleUser, Content: "Third"},
	}

	result := PrependSystemMessages("Prompt", "Memory", messages)

	require.Len(t, result, 5)
	assert.Equal(t, "Prompt", result[0].Content)
	assert.Equal(t, "Memory", result[1].Content)
	assert.Equal(t, "First", result[2].Content)
	assert.Equal(t, "Second", result[3].Content)
	assert.Equal(t, "Third", result[4].Content)
}

func TestPrependSystemMessages_LargeHistory(t *testing.T) {
	messages := make([]types.Message, 100)
	for i := range messages {
		messages[i] = types.Message{Role: types.RoleUser, Content: "msg"}
	}

	result := PrependSystemMessages("Prompt", "Memory", messages)

	assert.Len(t, result, 102)
}

// BuildToolParameterSchema tests.

func TestBuildToolParameterSchema_Empty(t *testing.T) {
	properties, required := BuildToolParameterSchema([]tools.Parameter{})

	assert.NotNil(t, properties)
	assert.Empty(t, properties)
	assert.NotNil(t, required)
	assert.Empty(t, required)
}

func TestBuildToolParameterSchema_SingleRequiredParam(t *testing.T) {
	params := []tools.Parameter{
		{Name: "query", Type: tools.ParamTypeString, Description: "Search query", Required: true},
	}

	properties, required := BuildToolParameterSchema(params)

	assert.Len(t, properties, 1)
	assert.Contains(t, properties, "query")

	prop, ok := properties["query"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "string", prop["type"])
	assert.Equal(t, "Search query", prop["description"])

	require.Len(t, required, 1)
	assert.Equal(t, "query", required[0])
}

func TestBuildToolParameterSchema_SingleOptionalParam(t *testing.T) {
	params := []tools.Parameter{
		{Name: "limit", Type: tools.ParamTypeInt, Description: "Result limit", Required: false},
	}

	properties, required := BuildToolParameterSchema(params)

	assert.Len(t, properties, 1)
	assert.Contains(t, properties, "limit")
	assert.Empty(t, required)
}

func TestBuildToolParameterSchema_MixedParams(t *testing.T) {
	params := []tools.Parameter{
		{Name: "query", Type: tools.ParamTypeString, Description: "Search query", Required: true},
		{Name: "limit", Type: tools.ParamTypeInt, Description: "Result limit", Required: false},
		{Name: "verbose", Type: tools.ParamTypeBool, Description: "Verbose output", Required: true},
		{Name: "tags", Type: tools.ParamTypeArray, Description: "Tag list", Required: false},
	}

	properties, required := BuildToolParameterSchema(params)

	assert.Len(t, properties, 4)
	assert.Len(t, required, 2)
	assert.Contains(t, required, "query")
	assert.Contains(t, required, "verbose")
	assert.NotContains(t, required, "limit")
	assert.NotContains(t, required, "tags")
}

func TestBuildToolParameterSchema_AllParamTypes(t *testing.T) {
	params := []tools.Parameter{
		{Name: "str_param", Type: tools.ParamTypeString, Description: "String", Required: false},
		{Name: "int_param", Type: tools.ParamTypeInt, Description: "Integer", Required: false},
		{Name: "bool_param", Type: tools.ParamTypeBool, Description: "Boolean", Required: false},
		{Name: "array_param", Type: tools.ParamTypeArray, Description: "Array", Required: false},
		{Name: "obj_param", Type: tools.ParamTypeObject, Description: "Object", Required: false},
	}

	properties, required := BuildToolParameterSchema(params)

	assert.Len(t, properties, 5)
	assert.Empty(t, required)

	strProp := properties["str_param"].(map[string]interface{})
	assert.Equal(t, "string", strProp["type"])

	intProp := properties["int_param"].(map[string]interface{})
	assert.Equal(t, "integer", intProp["type"])

	boolProp := properties["bool_param"].(map[string]interface{})
	assert.Equal(t, "boolean", boolProp["type"])

	arrayProp := properties["array_param"].(map[string]interface{})
	assert.Equal(t, "array", arrayProp["type"])

	objProp := properties["obj_param"].(map[string]interface{})
	assert.Equal(t, "object", objProp["type"])
}

func TestBuildToolParameterSchema_AllRequired(t *testing.T) {
	params := []tools.Parameter{
		{Name: "a", Type: tools.ParamTypeString, Description: "A", Required: true},
		{Name: "b", Type: tools.ParamTypeString, Description: "B", Required: true},
		{Name: "c", Type: tools.ParamTypeString, Description: "C", Required: true},
	}

	properties, required := BuildToolParameterSchema(params)

	assert.Len(t, properties, 3)
	assert.Len(t, required, 3)
	assert.Contains(t, required, "a")
	assert.Contains(t, required, "b")
	assert.Contains(t, required, "c")
}

func TestBuildToolParameterSchema_DescriptionPreserved(t *testing.T) {
	const desc = "This is a detailed description of the parameter."
	params := []tools.Parameter{
		{Name: "param", Type: tools.ParamTypeString, Description: desc, Required: false},
	}

	properties, _ := BuildToolParameterSchema(params)

	prop := properties["param"].(map[string]interface{})
	assert.Equal(t, desc, prop["description"])
}

// ExtractToolInfo tests.

func TestExtractToolInfo_BasicTool(t *testing.T) {
	tool := &mockTool{
		name:        "my_tool",
		description: "My test tool.",
		parameters: []tools.Parameter{
			{Name: "input", Type: tools.ParamTypeString, Description: "Input value", Required: true},
		},
	}

	info := ExtractToolInfo(tool)

	assert.Equal(t, "my_tool", info.Name)
	assert.Equal(t, "My test tool.", info.Description)
	assert.Len(t, info.Properties, 1)
	assert.Contains(t, info.Properties, "input")
	assert.Len(t, info.Required, 1)
	assert.Equal(t, "input", info.Required[0])
}

func TestExtractToolInfo_NoParameters(t *testing.T) {
	tool := &mockTool{
		name:        "no_params_tool",
		description: "Tool with no parameters.",
		parameters:  []tools.Parameter{},
	}

	info := ExtractToolInfo(tool)

	assert.Equal(t, "no_params_tool", info.Name)
	assert.Equal(t, "Tool with no parameters.", info.Description)
	assert.Empty(t, info.Properties)
	assert.Empty(t, info.Required)
}

func TestExtractToolInfo_MixedParameters(t *testing.T) {
	tool := &mockTool{
		name:        "mixed_tool",
		description: "Tool with mixed parameters.",
		parameters: []tools.Parameter{
			{Name: "required_str", Type: tools.ParamTypeString, Description: "Required string", Required: true},
			{Name: "optional_int", Type: tools.ParamTypeInt, Description: "Optional int", Required: false},
			{Name: "required_bool", Type: tools.ParamTypeBool, Description: "Required bool", Required: true},
		},
	}

	info := ExtractToolInfo(tool)

	assert.Equal(t, "mixed_tool", info.Name)
	assert.Len(t, info.Properties, 3)
	assert.Len(t, info.Required, 2)
	assert.Contains(t, info.Required, "required_str")
	assert.Contains(t, info.Required, "required_bool")
	assert.NotContains(t, info.Required, "optional_int")
}

func TestExtractToolInfo_ToolInfoFields(t *testing.T) {
	tool := &mockTool{
		name:        "search",
		description: "Search for files.",
		parameters:  []tools.Parameter{},
	}

	info := ExtractToolInfo(tool)

	// Verify ToolInfo struct has all expected fields.
	assert.Equal(t, "search", info.Name)
	assert.Equal(t, "Search for files.", info.Description)
	assert.NotNil(t, info.Properties)
	assert.NotNil(t, info.Required)
}

// ExtractAllToolInfo tests.

func TestExtractAllToolInfo_Empty(t *testing.T) {
	result := ExtractAllToolInfo([]tools.Tool{})

	assert.NotNil(t, result)
	assert.Empty(t, result)
}

func TestExtractAllToolInfo_SingleTool(t *testing.T) {
	availableTools := []tools.Tool{
		&mockTool{
			name:        "tool_one",
			description: "The first tool.",
			parameters: []tools.Parameter{
				{Name: "param", Type: tools.ParamTypeString, Description: "A param", Required: true},
			},
		},
	}

	result := ExtractAllToolInfo(availableTools)

	require.Len(t, result, 1)
	assert.Equal(t, "tool_one", result[0].Name)
	assert.Equal(t, "The first tool.", result[0].Description)
	assert.Len(t, result[0].Required, 1)
}

func TestExtractAllToolInfo_MultipleTools(t *testing.T) {
	availableTools := []tools.Tool{
		&mockTool{
			name:        "tool_a",
			description: "Tool A.",
			parameters:  []tools.Parameter{},
		},
		&mockTool{
			name:        "tool_b",
			description: "Tool B.",
			parameters: []tools.Parameter{
				{Name: "input", Type: tools.ParamTypeString, Description: "Input", Required: true},
			},
		},
		&mockTool{
			name:        "tool_c",
			description: "Tool C.",
			parameters: []tools.Parameter{
				{Name: "x", Type: tools.ParamTypeInt, Description: "X value", Required: false},
				{Name: "y", Type: tools.ParamTypeInt, Description: "Y value", Required: true},
			},
		},
	}

	result := ExtractAllToolInfo(availableTools)

	require.Len(t, result, 3)
	assert.Equal(t, "tool_a", result[0].Name)
	assert.Equal(t, "tool_b", result[1].Name)
	assert.Equal(t, "tool_c", result[2].Name)

	assert.Empty(t, result[0].Required)
	assert.Len(t, result[1].Required, 1)
	assert.Len(t, result[2].Required, 1)
	assert.Contains(t, result[2].Required, "y")
}

func TestExtractAllToolInfo_OrderPreserved(t *testing.T) {
	names := []string{"alpha", "beta", "gamma", "delta"}
	availableTools := make([]tools.Tool, len(names))
	for i, name := range names {
		availableTools[i] = &mockTool{name: name, description: name + " tool.", parameters: []tools.Parameter{}}
	}

	result := ExtractAllToolInfo(availableTools)

	require.Len(t, result, len(names))
	for i, name := range names {
		assert.Equal(t, name, result[i].Name)
	}
}

func TestExtractAllToolInfo_CapacityOptimized(t *testing.T) {
	// Verify that large slices work correctly (tests the capacity pre-allocation path).
	const toolCount = 50
	availableTools := make([]tools.Tool, toolCount)
	for i := range toolCount {
		availableTools[i] = &mockTool{
			name:        "tool",
			description: "A tool.",
			parameters:  []tools.Parameter{},
		}
	}

	result := ExtractAllToolInfo(availableTools)

	assert.Len(t, result, toolCount)
}

// FormatMessagesAsPrompt tests.

func TestFormatMessagesAsPrompt(t *testing.T) {
	messages := []types.Message{
		{Role: types.RoleUser, Content: "What stacks?"},
		{Role: types.RoleAssistant, Content: "You have 4."},
		{Role: types.RoleUser, Content: "Describe vpc."},
	}
	result := FormatMessagesAsPrompt(messages)
	assert.Contains(t, result, "What stacks?")
	assert.Contains(t, result, "Assistant: You have 4.")
	assert.Contains(t, result, "Describe vpc.")
}

func TestFormatMessagesAsPrompt_Empty(t *testing.T) {
	result := FormatMessagesAsPrompt(nil)
	assert.Empty(t, result)
}

func TestFormatMessagesAsPrompt_SingleUser(t *testing.T) {
	messages := []types.Message{{Role: types.RoleUser, Content: "Hello"}}
	assert.Equal(t, "Hello", FormatMessagesAsPrompt(messages))
}

func TestFormatMessagesAsPrompt_SkipsUnknownRoles(t *testing.T) {
	messages := []types.Message{
		{Role: types.RoleUser, Content: "Hello"},
		{Role: "system", Content: "ignored"},
		{Role: types.RoleAssistant, Content: "Hi"},
	}
	result := FormatMessagesAsPrompt(messages)
	assert.Contains(t, result, "Hello")
	assert.Contains(t, result, "Assistant: Hi")
	assert.NotContains(t, result, "ignored")
}

// ToolPropertySchema and ToolParameterSchema struct tests.

func TestToolPropertySchema_Fields(t *testing.T) {
	prop := ToolPropertySchema{
		Type:        "string",
		Description: "A string property.",
	}

	assert.Equal(t, "string", prop.Type)
	assert.Equal(t, "A string property.", prop.Description)
}

func TestToolParameterSchema_Fields(t *testing.T) {
	schema := ToolParameterSchema{
		Type: "object",
		Properties: map[string]ToolPropertySchema{
			"name": {Type: "string", Description: "The name."},
		},
		Required: []string{"name"},
	}

	assert.Equal(t, "object", schema.Type)
	assert.Contains(t, schema.Properties, "name")
	assert.Len(t, schema.Required, 1)
	assert.Equal(t, "name", schema.Required[0])
}

// ToolInfo struct test.

func TestToolInfo_Fields(t *testing.T) {
	info := ToolInfo{
		Name:        "test_tool",
		Description: "A test tool.",
		Properties: map[string]interface{}{
			"param": map[string]interface{}{"type": "string", "description": "A param"},
		},
		Required: []string{"param"},
	}

	assert.Equal(t, "test_tool", info.Name)
	assert.Equal(t, "A test tool.", info.Description)
	assert.Contains(t, info.Properties, "param")
	assert.Len(t, info.Required, 1)
}
