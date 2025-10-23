package protocol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
)

// Handler processes JSON-RPC requests and returns responses.
type Handler interface {
	// HandleRequest processes a request and returns a response.
	HandleRequest(ctx context.Context, req *Request) *Response

	// HandleNotification processes a notification (no response).
	HandleNotification(ctx context.Context, notif *Notification) error
}

// MethodHandler handles a specific JSON-RPC method.
type MethodHandler func(ctx context.Context, params json.RawMessage) (interface{}, error)

// NotificationHandler handles a specific JSON-RPC notification.
type NotificationHandler func(ctx context.Context, params json.RawMessage) error

// DefaultHandler is a default implementation of the Handler interface.
type DefaultHandler struct {
	methods       map[string]MethodHandler
	notifications map[string]NotificationHandler
}

// NewDefaultHandler creates a new default handler.
func NewDefaultHandler() *DefaultHandler {
	return &DefaultHandler{
		methods:       make(map[string]MethodHandler),
		notifications: make(map[string]NotificationHandler),
	}
}

// RegisterMethod registers a handler for a specific method.
func (h *DefaultHandler) RegisterMethod(method string, handler MethodHandler) {
	h.methods[method] = handler
}

// RegisterNotification registers a handler for a specific notification.
func (h *DefaultHandler) RegisterNotification(method string, handler NotificationHandler) {
	h.notifications[method] = handler
}

// HandleRequest processes a JSON-RPC request.
func (h *DefaultHandler) HandleRequest(ctx context.Context, req *Request) *Response {
	// Validate JSON-RPC version.
	if req.JSONRPC != JSONRPCVersion {
		return NewErrorResponse(
			req.ID,
			ErrorCodeInvalidRequest,
			fmt.Sprintf("invalid JSON-RPC version: %s", req.JSONRPC),
			nil,
		)
	}

	// Look up method handler.
	handler, ok := h.methods[req.Method]
	if !ok {
		return NewErrorResponse(
			req.ID,
			ErrorCodeMethodNotFound,
			fmt.Sprintf("method not found: %s", req.Method),
			nil,
		)
	}

	// Execute handler.
	result, err := handler(ctx, req.Params)
	if err != nil {
		// Check if it's an MCP error.
		var mcpErr *Error
		if errors.As(err, &mcpErr) {
			return NewErrorResponse(req.ID, mcpErr.Code, mcpErr.Message, mcpErr.Data)
		}
		// Return internal error.
		return NewErrorResponse(req.ID, ErrorCodeInternalError, err.Error(), nil)
	}

	return NewResponse(req.ID, result)
}

// HandleNotification processes a JSON-RPC notification.
func (h *DefaultHandler) HandleNotification(ctx context.Context, notif *Notification) error {
	// Validate JSON-RPC version.
	if notif.JSONRPC != JSONRPCVersion {
		return fmt.Errorf("%w: %s", errUtils.ErrMCPInvalidJSONRPCVersion, notif.JSONRPC)
	}

	// Look up notification handler.
	handler, ok := h.notifications[notif.Method]
	if !ok {
		// Notifications without handlers are silently ignored.
		return nil
	}

	// Execute handler.
	return handler(ctx, notif.Params)
}

// ParseMessage parses a JSON message into a Request or Notification.
func ParseMessage(data []byte) (interface{}, error) {
	// First, try to parse as a generic message to check for ID.
	var msg struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      interface{}     `json:"id"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params,omitempty"`
	}

	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, &Error{
			Code:    ErrorCodeParseError,
			Message: "failed to parse JSON",
			Data:    err.Error(),
		}
	}

	// Check if it's a notification (no ID field).
	if msg.ID == nil && msg.Method != "" {
		return &Notification{
			JSONRPC: msg.JSONRPC,
			Method:  msg.Method,
			Params:  msg.Params,
		}, nil
	}

	// Otherwise, it's a request.
	return &Request{
		JSONRPC: msg.JSONRPC,
		ID:      msg.ID,
		Method:  msg.Method,
		Params:  msg.Params,
	}, nil
}
