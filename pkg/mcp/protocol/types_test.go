package protocol

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProtocolConstants(t *testing.T) {
	assert.Equal(t, "2025-03-26", ProtocolVersion)
	assert.Equal(t, "2.0", JSONRPCVersion)
}

func TestToolMarshal(t *testing.T) {
	tests := []struct {
		name string
		tool Tool
		want string
	}{
		{
			name: "tool with all fields",
			tool: Tool{
				Name:        "test_tool",
				Description: "A test tool",
				InputSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"arg1": map[string]interface{}{
							"type":        "string",
							"description": "First argument",
						},
					},
				},
			},
			want: `{"name":"test_tool","description":"A test tool","inputSchema":{"properties":{"arg1":{"description":"First argument","type":"string"}},"type":"object"}}`,
		},
		{
			name: "tool without description",
			tool: Tool{
				Name: "simple_tool",
				InputSchema: map[string]interface{}{
					"type": "object",
				},
			},
			want: `{"name":"simple_tool","inputSchema":{"type":"object"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.tool)
			require.NoError(t, err)
			assert.JSONEq(t, tt.want, string(data))
		})
	}
}

func TestContentMarshal(t *testing.T) {
	tests := []struct {
		name    string
		content Content
		want    string
	}{
		{
			name: "text content",
			content: Content{
				Type: "text",
				Text: "Hello, world!",
			},
			want: `{"type":"text","text":"Hello, world!"}`,
		},
		{
			name: "image content",
			content: Content{
				Type:     "image",
				Data:     "base64data",
				MimeType: "image/png",
			},
			want: `{"type":"image","data":"base64data","mimeType":"image/png"}`,
		},
		{
			name: "resource content",
			content: Content{
				Type: "resource",
				Resource: &EmbeddedResource{
					Type: "resource",
					Resource: Resource{
						URI:      "file:///test.txt",
						MimeType: "text/plain",
						Name:     "test.txt",
					},
				},
			},
			want: `{"type":"resource","resource":{"type":"resource","resource":{"uri":"file:///test.txt","name":"test.txt","mimeType":"text/plain"}}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.content)
			require.NoError(t, err)
			assert.JSONEq(t, tt.want, string(data))
		})
	}
}

func TestCallToolParamsMarshal(t *testing.T) {
	tests := []struct {
		name   string
		params CallToolParams
		want   string
	}{
		{
			name: "with arguments",
			params: CallToolParams{
				Name: "describe_component",
				Arguments: map[string]interface{}{
					"component": "vpc",
					"stack":     "prod",
				},
			},
			want: `{"name":"describe_component","arguments":{"component":"vpc","stack":"prod"}}`,
		},
		{
			name: "without arguments",
			params: CallToolParams{
				Name: "list_stacks",
			},
			want: `{"name":"list_stacks"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.params)
			require.NoError(t, err)
			assert.JSONEq(t, tt.want, string(data))
		})
	}
}

func TestCallToolResultMarshal(t *testing.T) {
	tests := []struct {
		name   string
		result CallToolResult
		want   string
	}{
		{
			name: "success result",
			result: CallToolResult{
				Content: []Content{
					{
						Type: "text",
						Text: "Component described successfully",
					},
				},
				IsError: false,
			},
			want: `{"content":[{"type":"text","text":"Component described successfully"}]}`,
		},
		{
			name: "error result",
			result: CallToolResult{
				Content: []Content{
					{
						Type: "text",
						Text: "Tool execution failed",
					},
				},
				IsError: true,
			},
			want: `{"content":[{"type":"text","text":"Tool execution failed"}],"isError":true}`,
		},
		{
			name: "multiple content items",
			result: CallToolResult{
				Content: []Content{
					{
						Type: "text",
						Text: "Output:",
					},
					{
						Type: "text",
						Text: "Data: value1",
					},
				},
			},
			want: `{"content":[{"type":"text","text":"Output:"},{"type":"text","text":"Data: value1"}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.result)
			require.NoError(t, err)
			assert.JSONEq(t, tt.want, string(data))
		})
	}
}

func TestInitializeParamsUnmarshal(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		want    InitializeParams
		wantErr bool
	}{
		{
			name: "complete params",
			json: `{
				"protocolVersion": "2025-03-26",
				"capabilities": {
					"roots": {
						"listChanged": true
					}
				},
				"clientInfo": {
					"name": "claude-desktop",
					"version": "1.0.0"
				}
			}`,
			want: InitializeParams{
				ProtocolVersion: "2025-03-26",
				Capabilities: ClientCapabilities{
					Roots: &RootsCapability{
						ListChanged: true,
					},
				},
				ClientInfo: Implementation{
					Name:    "claude-desktop",
					Version: "1.0.0",
				},
			},
			wantErr: false,
		},
		{
			name: "minimal params",
			json: `{
				"protocolVersion": "2025-03-26",
				"capabilities": {},
				"clientInfo": {
					"name": "test-client",
					"version": "0.1.0"
				}
			}`,
			want: InitializeParams{
				ProtocolVersion: "2025-03-26",
				Capabilities:    ClientCapabilities{},
				ClientInfo: Implementation{
					Name:    "test-client",
					Version: "0.1.0",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got InitializeParams
			err := json.Unmarshal([]byte(tt.json), &got)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestServerCapabilitiesMarshal(t *testing.T) {
	caps := ServerCapabilities{
		Tools: &ToolsCapability{
			ListChanged: false,
		},
		Resources: &ResourcesCapability{
			Subscribe:   true,
			ListChanged: false,
		},
		Prompts: &PromptsCapability{
			ListChanged: false,
		},
	}

	data, err := json.Marshal(caps)
	require.NoError(t, err)

	want := `{
		"tools": {"listChanged": false},
		"resources": {"subscribe": true, "listChanged": false},
		"prompts": {"listChanged": false}
	}`
	assert.JSONEq(t, want, string(data))
}

func TestInitializeResultMarshal(t *testing.T) {
	result := InitializeResult{
		ProtocolVersion: ProtocolVersion,
		ServerInfo: Implementation{
			Name:    "atmos-mcp-server",
			Version: "1.0.0",
		},
		Capabilities: ServerCapabilities{
			Tools: &ToolsCapability{
				ListChanged: false,
			},
		},
		Instructions: "Test instructions",
	}

	data, err := json.Marshal(result)
	require.NoError(t, err)

	want := `{
		"protocolVersion": "2025-03-26",
		"serverInfo": {
			"name": "atmos-mcp-server",
			"version": "1.0.0"
		},
		"capabilities": {
			"tools": {"listChanged": false}
		},
		"instructions": "Test instructions"
	}`
	assert.JSONEq(t, want, string(data))
}

func TestResourceUnmarshal(t *testing.T) {
	jsonData := `{
		"uri": "file:///etc/atmos.yaml",
		"name": "Atmos Configuration",
		"description": "Main Atmos configuration file",
		"mimeType": "application/yaml"
	}`

	var resource Resource
	err := json.Unmarshal([]byte(jsonData), &resource)
	require.NoError(t, err)

	assert.Equal(t, "file:///etc/atmos.yaml", resource.URI)
	assert.Equal(t, "Atmos Configuration", resource.Name)
	assert.Equal(t, "Main Atmos configuration file", resource.Description)
	assert.Equal(t, "application/yaml", resource.MimeType)
}

func TestPromptUnmarshal(t *testing.T) {
	jsonData := `{
		"name": "analyze_stack",
		"description": "Analyze a stack configuration",
		"arguments": [
			{
				"name": "stack",
				"description": "Stack name",
				"required": true
			}
		]
	}`

	var prompt Prompt
	err := json.Unmarshal([]byte(jsonData), &prompt)
	require.NoError(t, err)

	assert.Equal(t, "analyze_stack", prompt.Name)
	assert.Equal(t, "Analyze a stack configuration", prompt.Description)
	require.Len(t, prompt.Arguments, 1)
	assert.Equal(t, "stack", prompt.Arguments[0].Name)
	assert.Equal(t, "Stack name", prompt.Arguments[0].Description)
	assert.True(t, prompt.Arguments[0].Required)
}
