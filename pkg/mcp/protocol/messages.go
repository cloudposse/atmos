package protocol

import (
	"encoding/json"
)

// Request represents a JSON-RPC request.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response represents a JSON-RPC response.
type Response struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *Error      `json:"error,omitempty"`
}

// Notification represents a JSON-RPC notification (request without ID).
type Notification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// NewRequest creates a new JSON-RPC request.
func NewRequest(id interface{}, method string, params interface{}) (*Request, error) {
	var paramsJSON json.RawMessage
	if params != nil {
		data, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		paramsJSON = data
	}

	return &Request{
		JSONRPC: JSONRPCVersion,
		ID:      id,
		Method:  method,
		Params:  paramsJSON,
	}, nil
}

// NewResponse creates a new JSON-RPC response.
func NewResponse(id interface{}, result interface{}) *Response {
	return &Response{
		JSONRPC: JSONRPCVersion,
		ID:      id,
		Result:  result,
	}
}

// NewErrorResponse creates a new JSON-RPC error response.
func NewErrorResponse(id interface{}, code ErrorCode, message string, data interface{}) *Response {
	return &Response{
		JSONRPC: JSONRPCVersion,
		ID:      id,
		Error: &Error{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
}

// NewNotification creates a new JSON-RPC notification.
func NewNotification(method string, params interface{}) (*Notification, error) {
	var paramsJSON json.RawMessage
	if params != nil {
		data, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		paramsJSON = data
	}

	return &Notification{
		JSONRPC: JSONRPCVersion,
		Method:  method,
		Params:  paramsJSON,
	}, nil
}

// IsNotification returns true if this is a notification (no ID).
func (r *Request) IsNotification() bool {
	return r.ID == nil
}

// UnmarshalParams unmarshals the params into the provided value.
func (r *Request) UnmarshalParams(v interface{}) error {
	if len(r.Params) == 0 {
		return nil
	}
	return json.Unmarshal(r.Params, v)
}

// UnmarshalParams unmarshals the params into the provided value.
func (n *Notification) UnmarshalParams(v interface{}) error {
	if len(n.Params) == 0 {
		return nil
	}
	return json.Unmarshal(n.Params, v)
}

// Methods defines the standard MCP method names.
const (
	MethodInitialize         = "initialize"
	MethodInitialized        = "notifications/initialized"
	MethodPing               = "ping"
	MethodToolsList          = "tools/list"
	MethodToolsCall          = "tools/call"
	MethodResourcesList      = "resources/list"
	MethodResourcesRead      = "resources/read"
	MethodResourcesSubscribe = "resources/subscribe"
	MethodPromptsList        = "prompts/list"
	MethodPromptsGet         = "prompts/get"
	MethodLoggingSetLevel    = "logging/setLevel"
	MethodLoggingMessage     = "notifications/message"
	MethodCancelled          = "notifications/cancelled"
	MethodProgress           = "notifications/progress"
)
