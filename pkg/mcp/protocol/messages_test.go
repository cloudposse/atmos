package protocol

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseMessage_Request(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		want    *Request
		wantErr bool
	}{
		{
			name: "valid request with params",
			data: `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"test_tool","arguments":{"arg":"value"}}}`,
			want: &Request{
				JSONRPC: "2.0",
				ID:      float64(1),
				Method:  "tools/call",
				Params:  json.RawMessage(`{"name":"test_tool","arguments":{"arg":"value"}}`),
			},
			wantErr: false,
		},
		{
			name: "valid request without params",
			data: `{"jsonrpc":"2.0","id":"abc","method":"tools/list"}`,
			want: &Request{
				JSONRPC: "2.0",
				ID:      "abc",
				Method:  "tools/list",
			},
			wantErr: false,
		},
		{
			name:    "invalid json",
			data:    `{invalid}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := ParseMessage([]byte(tt.data))
			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			req, ok := msg.(*Request)
			require.True(t, ok, "expected *Request")
			assert.Equal(t, tt.want.JSONRPC, req.JSONRPC)
			assert.Equal(t, tt.want.ID, req.ID)
			assert.Equal(t, tt.want.Method, req.Method)
		})
	}
}

func TestParseMessage_Notification(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		want    *Notification
		wantErr bool
	}{
		{
			name: "valid notification with params",
			data: `{"jsonrpc":"2.0","method":"notifications/initialized","params":{"status":"ready"}}`,
			want: &Notification{
				JSONRPC: "2.0",
				Method:  "notifications/initialized",
				Params:  json.RawMessage(`{"status":"ready"}`),
			},
			wantErr: false,
		},
		{
			name: "valid notification without params",
			data: `{"jsonrpc":"2.0","method":"ping"}`,
			want: &Notification{
				JSONRPC: "2.0",
				Method:  "ping",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := ParseMessage([]byte(tt.data))
			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			notif, ok := msg.(*Notification)
			require.True(t, ok, "expected *Notification")
			assert.Equal(t, tt.want.JSONRPC, notif.JSONRPC)
			assert.Equal(t, tt.want.Method, notif.Method)
		})
	}
}

func TestNewResponse(t *testing.T) {
	result := map[string]string{"status": "ok"}
	resp := NewResponse(42, result)

	assert.Equal(t, JSONRPCVersion, resp.JSONRPC)
	assert.Equal(t, 42, resp.ID)
	assert.Equal(t, result, resp.Result)
	assert.Nil(t, resp.Error)
}

func TestNewErrorResponse(t *testing.T) {
	resp := NewErrorResponse("test-id", ErrorCodeInvalidRequest, "Invalid request", nil)

	assert.Equal(t, JSONRPCVersion, resp.JSONRPC)
	assert.Equal(t, "test-id", resp.ID)
	assert.Nil(t, resp.Result)
	require.NotNil(t, resp.Error)
	assert.Equal(t, ErrorCodeInvalidRequest, resp.Error.Code)
	assert.Equal(t, "Invalid request", resp.Error.Message)
	assert.Nil(t, resp.Error.Data)
}

func TestNewErrorResponse_WithData(t *testing.T) {
	data := map[string]string{"field": "value"}
	resp := NewErrorResponse(123, ErrorCodeInternalError, "Internal error", data)

	require.NotNil(t, resp.Error)
	assert.Equal(t, ErrorCodeInternalError, resp.Error.Code)
	assert.Equal(t, "Internal error", resp.Error.Message)
	assert.Equal(t, data, resp.Error.Data)
}

func TestResponseMarshal(t *testing.T) {
	tests := []struct {
		name string
		resp *Response
		want string
	}{
		{
			name: "success response",
			resp: &Response{
				JSONRPC: "2.0",
				ID:      1,
				Result:  map[string]string{"status": "ok"},
			},
			want: `{"jsonrpc":"2.0","id":1,"result":{"status":"ok"}}`,
		},
		{
			name: "error response",
			resp: &Response{
				JSONRPC: "2.0",
				ID:      2,
				Error: &Error{
					Code:    ErrorCodeMethodNotFound,
					Message: "Method not found",
				},
			},
			want: `{"jsonrpc":"2.0","id":2,"error":{"code":-32601,"message":"Method not found"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.resp)
			require.NoError(t, err)
			assert.JSONEq(t, tt.want, string(data))
		})
	}
}

func TestErrorCodes(t *testing.T) {
	assert.Equal(t, -32700, ErrorCodeParseError)
	assert.Equal(t, -32600, ErrorCodeInvalidRequest)
	assert.Equal(t, -32601, ErrorCodeMethodNotFound)
	assert.Equal(t, -32602, ErrorCodeInvalidParams)
	assert.Equal(t, -32603, ErrorCodeInternalError)
}

func TestMethodConstants(t *testing.T) {
	assert.Equal(t, "initialize", MethodInitialize)
	assert.Equal(t, "initialized", MethodInitialized)
	assert.Equal(t, "ping", MethodPing)
	assert.Equal(t, "tools/list", MethodToolsList)
	assert.Equal(t, "tools/call", MethodToolsCall)
	assert.Equal(t, "resources/list", MethodResourcesList)
	assert.Equal(t, "resources/read", MethodResourcesRead)
	assert.Equal(t, "prompts/list", MethodPromptsList)
	assert.Equal(t, "prompts/get", MethodPromptsGet)
}

func TestNotificationMarshal(t *testing.T) {
	notif := &Notification{
		JSONRPC: "2.0",
		Method:  "notifications/tools/listChanged",
	}

	data, err := json.Marshal(notif)
	require.NoError(t, err)
	assert.JSONEq(t, `{"jsonrpc":"2.0","method":"notifications/tools/listChanged"}`, string(data))
}

func TestRequestMarshal(t *testing.T) {
	params := map[string]interface{}{
		"name": "test_tool",
		"arguments": map[string]interface{}{
			"arg1": "value1",
		},
	}
	paramsJSON, _ := json.Marshal(params)

	req := &Request{
		JSONRPC: "2.0",
		ID:      "req-1",
		Method:  "tools/call",
		Params:  paramsJSON,
	}

	data, err := json.Marshal(req)
	require.NoError(t, err)

	want := `{
		"jsonrpc": "2.0",
		"id": "req-1",
		"method": "tools/call",
		"params": {"name":"test_tool","arguments":{"arg1":"value1"}}
	}`
	assert.JSONEq(t, want, string(data))
}

func TestParseMessage_EmptyData(t *testing.T) {
	_, err := ParseMessage([]byte{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected end of JSON input")
}

func TestParseMessage_NullID(t *testing.T) {
	// Notification with null ID (should still be parsed as notification).
	data := `{"jsonrpc":"2.0","method":"test","id":null}`
	msg, err := ParseMessage([]byte(data))
	require.NoError(t, err)

	// With id:null, it's parsed as a request but logically can be treated as notification.
	_, ok := msg.(*Request)
	assert.True(t, ok)
}

func TestErrorMarshal(t *testing.T) {
	errObj := &Error{
		Code:    ErrorCodeInvalidParams,
		Message: "Invalid parameters",
		Data: map[string]interface{}{
			"missing": []string{"arg1", "arg2"},
		},
	}

	data, err := json.Marshal(errObj)
	require.NoError(t, err)

	want := `{
		"code": -32602,
		"message": "Invalid parameters",
		"data": {"missing": ["arg1", "arg2"]}
	}`
	assert.JSONEq(t, want, string(data))
}
