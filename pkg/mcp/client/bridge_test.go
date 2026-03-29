package client

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Verify BridgedTool implements tools.Tool at compile time.
var _ tools.Tool = (*BridgedTool)(nil)

func TestBridgedTool_Name(t *testing.T) {
	tool := &mcpsdk.Tool{Name: "list_clusters", Description: "List EKS clusters"}
	bt := NewBridgedTool("aws-eks", tool, nil)

	assert.Equal(t, "aws-eks__list_clusters", bt.Name())
}

func TestBridgedTool_Description(t *testing.T) {
	tool := &mcpsdk.Tool{Name: "list_clusters", Description: "List EKS clusters"}
	bt := NewBridgedTool("aws-eks", tool, nil)

	assert.Equal(t, "List EKS clusters", bt.Description())
}

func TestBridgedTool_ServerName(t *testing.T) {
	tool := &mcpsdk.Tool{Name: "list_clusters"}
	bt := NewBridgedTool("aws-eks", tool, nil)

	assert.Equal(t, "aws-eks", bt.ServerName())
}

func TestBridgedTool_OriginalName(t *testing.T) {
	tool := &mcpsdk.Tool{Name: "list_clusters"}
	bt := NewBridgedTool("aws-eks", tool, nil)

	assert.Equal(t, "list_clusters", bt.OriginalName())
}

func TestBridgedTool_RequiresPermission(t *testing.T) {
	bt := NewBridgedTool("test", &mcpsdk.Tool{}, nil)
	assert.True(t, bt.RequiresPermission())
}

func TestBridgedTool_IsRestricted(t *testing.T) {
	bt := NewBridgedTool("test", &mcpsdk.Tool{}, nil)
	assert.False(t, bt.IsRestricted())
}

func TestBridgedTool_Parameters_WithSchema(t *testing.T) {
	tool := &mcpsdk.Tool{
		Name: "test_tool",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"region": map[string]any{
					"type":        "string",
					"description": "AWS region",
				},
				"count": map[string]any{
					"type":        "integer",
					"description": "Number of items",
				},
				"verbose": map[string]any{
					"type": "boolean",
				},
			},
			"required": []any{"region"},
		},
	}
	bt := NewBridgedTool("test", tool, nil)

	params := bt.Parameters()
	assert.Len(t, params, 3)

	// Find region param.
	var regionParam *tools.Parameter
	for i := range params {
		if params[i].Name == "region" {
			regionParam = &params[i]
			break
		}
	}
	assert.NotNil(t, regionParam)
	assert.True(t, regionParam.Required)
	assert.Equal(t, tools.ParamTypeString, regionParam.Type)
	assert.Equal(t, "AWS region", regionParam.Description)
}

func TestBridgedTool_Parameters_NoSchema(t *testing.T) {
	tool := &mcpsdk.Tool{Name: "test"}
	bt := NewBridgedTool("test", tool, nil)

	params := bt.Parameters()
	assert.Nil(t, params)
}

func TestBridgedTool_Parameters_NoProperties(t *testing.T) {
	tool := &mcpsdk.Tool{
		Name:        "test",
		InputSchema: map[string]any{"type": "object"},
	}
	bt := NewBridgedTool("test", tool, nil)

	params := bt.Parameters()
	assert.Nil(t, params)
}

func TestBridgedTool_Parameters_NonMapProperty(t *testing.T) {
	tool := &mcpsdk.Tool{
		Name: "test",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"valid":   map[string]any{"type": "string"},
				"invalid": "not-a-map", // Non-map property — should be skipped.
			},
		},
	}
	bt := NewBridgedTool("test", tool, nil)

	params := bt.Parameters()
	assert.Len(t, params, 1, "non-map properties should be skipped")
	assert.Equal(t, "valid", params[0].Name)
}

func TestBridgedTool_Parameters_NoTypeField(t *testing.T) {
	tool := &mcpsdk.Tool{
		Name: "test",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"untyped": map[string]any{"description": "No type field"},
			},
		},
	}
	bt := NewBridgedTool("test", tool, nil)

	params := bt.Parameters()
	assert.Len(t, params, 1)
	assert.Equal(t, tools.ParamTypeString, params[0].Type, "missing type should default to string")
}

func TestExtractTextContent_Nil(t *testing.T) {
	assert.Equal(t, "", ExtractTextContent(nil))
}

func TestExtractTextContent_EmptyContent(t *testing.T) {
	result := &mcpsdk.CallToolResult{}
	assert.Equal(t, "", ExtractTextContent(result))
}

func TestExtractTextContent_TextContent(t *testing.T) {
	result := &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{
			&mcpsdk.TextContent{Text: "Hello"},
			&mcpsdk.TextContent{Text: "World"},
		},
	}
	assert.Equal(t, "Hello\nWorld", ExtractTextContent(result))
}

// TestExtractTextContent_MixedContentTypes tests that non-text content is skipped
// and only TextContent values are extracted.
func TestExtractTextContent_MixedContentTypes(t *testing.T) {
	// imageContent implements mcpsdk.Content but is not TextContent.
	result := &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{
			&mcpsdk.TextContent{Text: "First text"},
			&mcpsdk.ImageContent{Data: []byte("base64data"), MIMEType: "image/png"},
			&mcpsdk.TextContent{Text: "Second text"},
		},
	}
	output := ExtractTextContent(result)
	assert.Equal(t, "First text\nSecond text", output)
}

// TestExtractTextContent_SingleTextContent tests extraction of a single text content item.
func TestExtractTextContent_SingleTextContent(t *testing.T) {
	result := &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{
			&mcpsdk.TextContent{Text: "Only text"},
		},
	}
	output := ExtractTextContent(result)
	assert.Equal(t, "Only text", output)
}

// TestExtractTextContent_OnlyNonTextContent tests that result with only non-text content
// returns an empty string.
func TestExtractTextContent_OnlyNonTextContent(t *testing.T) {
	result := &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{
			&mcpsdk.ImageContent{Data: []byte("base64data"), MIMEType: "image/png"},
		},
	}
	output := ExtractTextContent(result)
	assert.Equal(t, "", output)
}

// TestExtractTextContent_EmptyTextContent tests that empty text strings are included in output.
func TestExtractTextContent_EmptyTextContent(t *testing.T) {
	result := &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{
			&mcpsdk.TextContent{Text: ""},
			&mcpsdk.TextContent{Text: "non-empty"},
		},
	}
	output := ExtractTextContent(result)
	assert.Equal(t, "\nnon-empty", output)
}

func TestBridgeTools(t *testing.T) {
	cfg := &ParsedConfig{Name: "test-server", Command: "echo"}
	session := NewSession(cfg)

	// Manually set tools on the session for testing.
	session.tools = []*mcpsdk.Tool{
		{Name: "tool_a", Description: "Tool A"},
		{Name: "tool_b", Description: "Tool B"},
	}

	bridged := BridgeTools(session)
	assert.Len(t, bridged, 2)
	assert.Equal(t, "test-server__tool_a", bridged[0].Name())
	assert.Equal(t, "test-server__tool_b", bridged[1].Name())
}

func TestBridgeTools_Empty(t *testing.T) {
	cfg := &ParsedConfig{Name: "test", Command: "echo"}
	session := NewSession(cfg)

	bridged := BridgeTools(session)
	assert.Empty(t, bridged)
}

func TestSanitizeToolName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"alphanumeric", "list_clusters", "list_clusters"},
		{"dots replaced", "aws.search_documentation", "aws_search_documentation"},
		{"hyphens preserved", "aws-eks", "aws-eks"},
		{"spaces replaced", "my tool name", "my_tool_name"},
		{"special chars replaced", "tool@v1.2/path", "tool_v1_2_path"},
		{"empty string", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, sanitizeToolName(tt.input))
		})
	}

	// Long name test uses generated data, tested separately.
	t.Run("long name truncated", func(t *testing.T) {
		input := make([]byte, 200)
		for i := range input {
			input[i] = 'a'
		}
		expected := make([]byte, maxToolNameLen)
		for i := range expected {
			expected[i] = 'a'
		}
		assert.Equal(t, string(expected), sanitizeToolName(string(input)))
	})
}

func TestIsToolNameChar(t *testing.T) {
	// Allowed characters.
	for _, r := range "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-" {
		assert.True(t, isToolNameChar(r), "expected %c to be allowed", r)
	}
	// Disallowed characters.
	for _, r := range "./@#$%^&*() " {
		assert.False(t, isToolNameChar(r), "expected %c to be disallowed", r)
	}
}

func TestBridgedTool_ImplementsBridgedToolInfo(t *testing.T) {
	// Verify the interface is properly implemented via type assertion.
	tool := &mcpsdk.Tool{Name: "search_docs", Description: "Search"}
	bt := NewBridgedTool("aws-docs", tool, nil)

	info, ok := interface{}(bt).(tools.BridgedToolInfo)
	assert.True(t, ok, "BridgedTool should implement BridgedToolInfo")
	assert.Equal(t, "aws-docs", info.ServerName())
	assert.Equal(t, "search_docs", info.OriginalName())
}

// TestBridgedTool_Execute_NilSession tests that Execute with a nil session panics
// rather than silently succeeding. This documents the expected behavior.
func TestBridgedTool_Execute_NilSession(t *testing.T) {
	tool := &mcpsdk.Tool{Name: "test_tool", Description: "Test"}
	bt := NewBridgedTool("test", tool, nil)

	// Execute with nil session should panic (nil pointer dereference).
	assert.Panics(t, func() {
		_, _ = bt.Execute(context.Background(), map[string]interface{}{"key": "value"})
	}, "Execute with nil session should panic")
}

// TestBridgedTool_Execute_SessionNotRunning tests that Execute returns an error
// when the session exists but is not running.
func TestBridgedTool_Execute_SessionNotRunning(t *testing.T) {
	cfg := &ParsedConfig{Name: "test", Command: "echo"}
	session := NewSession(cfg)

	tool := &mcpsdk.Tool{Name: "test_tool", Description: "Test"}
	bt := NewBridgedTool("test", tool, session)

	result, err := bt.Execute(context.Background(), map[string]interface{}{"key": "value"})
	require.NoError(t, err, "Execute should not return a Go error; it wraps errors in Result")
	assert.False(t, result.Success)
	assert.Contains(t, result.Output, "execution failed")
	assert.ErrorIs(t, result.Error, errUtils.ErrMCPServerNotRunning)
}

func TestMapJSONSchemaType(t *testing.T) {
	tests := []struct {
		input string
		want  tools.ParamType
	}{
		{"string", tools.ParamTypeString},
		{"integer", tools.ParamTypeInt},
		{"number", tools.ParamTypeInt},
		{"boolean", tools.ParamTypeBool},
		{"array", tools.ParamTypeArray},
		{"object", tools.ParamTypeObject},
		{"unknown", tools.ParamTypeString},
		{"", tools.ParamTypeString},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, mapJSONSchemaType(tt.input))
		})
	}
}
