package protocol

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDefaultHandler(t *testing.T) {
	handler := NewDefaultHandler()
	assert.NotNil(t, handler)
	assert.NotNil(t, handler.methods)
	assert.NotNil(t, handler.notifications)
}

func TestRegisterMethod(t *testing.T) {
	handler := NewDefaultHandler()

	called := false
	testHandler := func(ctx context.Context, params json.RawMessage) (interface{}, error) {
		called = true
		return "result", nil
	}

	handler.RegisterMethod("test", testHandler)

	req := &Request{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "test",
	}

	resp := handler.HandleRequest(context.Background(), req)
	assert.True(t, called)
	assert.Nil(t, resp.Error)
	assert.Equal(t, "result", resp.Result)
}

func TestRegisterNotification(t *testing.T) {
	handler := NewDefaultHandler()

	called := false
	testHandler := func(ctx context.Context, params json.RawMessage) error {
		called = true
		return nil
	}

	handler.RegisterNotification("test_notif", testHandler)

	notif := &Notification{
		JSONRPC: "2.0",
		Method:  "test_notif",
	}

	err := handler.HandleNotification(context.Background(), notif)
	assert.True(t, called)
	assert.NoError(t, err)
}

func TestHandleRequest_MethodNotFound(t *testing.T) {
	handler := NewDefaultHandler()

	req := &Request{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "nonexistent",
	}

	resp := handler.HandleRequest(context.Background(), req)
	require.NotNil(t, resp.Error)
	assert.Equal(t, ErrorCodeMethodNotFound, resp.Error.Code)
	assert.Contains(t, resp.Error.Message, "Method not found")
}

func TestHandleRequest_Success(t *testing.T) {
	handler := NewDefaultHandler()

	handler.RegisterMethod("echo", func(ctx context.Context, params json.RawMessage) (interface{}, error) {
		var input map[string]string
		if err := json.Unmarshal(params, &input); err != nil {
			return nil, err
		}
		return input, nil
	})

	params := map[string]string{"message": "hello"}
	paramsJSON, _ := json.Marshal(params)

	req := &Request{
		JSONRPC: "2.0",
		ID:      "test-1",
		Method:  "echo",
		Params:  paramsJSON,
	}

	resp := handler.HandleRequest(context.Background(), req)
	assert.Nil(t, resp.Error)
	assert.Equal(t, params, resp.Result)
	assert.Equal(t, "test-1", resp.ID)
}

func TestHandleRequest_HandlerError(t *testing.T) {
	handler := NewDefaultHandler()

	handler.RegisterMethod("fail", func(ctx context.Context, params json.RawMessage) (interface{}, error) {
		return nil, errors.New("intentional error")
	})

	req := &Request{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "fail",
	}

	resp := handler.HandleRequest(context.Background(), req)
	require.NotNil(t, resp.Error)
	assert.Equal(t, ErrorCodeInternalError, resp.Error.Code)
	assert.Contains(t, resp.Error.Message, "intentional error")
}

func TestHandleRequest_WithParams(t *testing.T) {
	handler := NewDefaultHandler()

	handler.RegisterMethod("add", func(ctx context.Context, params json.RawMessage) (interface{}, error) {
		var input struct {
			A int `json:"a"`
			B int `json:"b"`
		}
		if err := json.Unmarshal(params, &input); err != nil {
			return nil, err
		}
		return map[string]int{"sum": input.A + input.B}, nil
	})

	params := map[string]int{"a": 5, "b": 3}
	paramsJSON, _ := json.Marshal(params)

	req := &Request{
		JSONRPC: "2.0",
		ID:      3,
		Method:  "add",
		Params:  paramsJSON,
	}

	resp := handler.HandleRequest(context.Background(), req)
	assert.Nil(t, resp.Error)
	assert.Equal(t, map[string]int{"sum": 8}, resp.Result)
}

func TestHandleNotification_Success(t *testing.T) {
	handler := NewDefaultHandler()

	var receivedParams json.RawMessage
	handler.RegisterNotification("update", func(ctx context.Context, params json.RawMessage) error {
		receivedParams = params
		return nil
	})

	params := map[string]string{"status": "updated"}
	paramsJSON, _ := json.Marshal(params)

	notif := &Notification{
		JSONRPC: "2.0",
		Method:  "update",
		Params:  paramsJSON,
	}

	err := handler.HandleNotification(context.Background(), notif)
	assert.NoError(t, err)
	assert.Equal(t, paramsJSON, receivedParams)
}

func TestHandleNotification_NotFound(t *testing.T) {
	handler := NewDefaultHandler()

	notif := &Notification{
		JSONRPC: "2.0",
		Method:  "unknown",
	}

	err := handler.HandleNotification(context.Background(), notif)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "notification handler not found")
}

func TestHandleNotification_HandlerError(t *testing.T) {
	handler := NewDefaultHandler()

	handler.RegisterNotification("fail", func(ctx context.Context, params json.RawMessage) error {
		return errors.New("notification failed")
	})

	notif := &Notification{
		JSONRPC: "2.0",
		Method:  "fail",
	}

	err := handler.HandleNotification(context.Background(), notif)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "notification failed")
}

func TestHandleRequest_NilParams(t *testing.T) {
	handler := NewDefaultHandler()

	called := false
	handler.RegisterMethod("no_params", func(ctx context.Context, params json.RawMessage) (interface{}, error) {
		called = true
		assert.Nil(t, params)
		return "ok", nil
	})

	req := &Request{
		JSONRPC: "2.0",
		ID:      4,
		Method:  "no_params",
		Params:  nil,
	}

	resp := handler.HandleRequest(context.Background(), req)
	assert.True(t, called)
	assert.Nil(t, resp.Error)
	assert.Equal(t, "ok", resp.Result)
}

func TestHandleRequest_ContextCancellation(t *testing.T) {
	handler := NewDefaultHandler()

	handler.RegisterMethod("slow", func(ctx context.Context, params json.RawMessage) (interface{}, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	req := &Request{
		JSONRPC: "2.0",
		ID:      5,
		Method:  "slow",
	}

	resp := handler.HandleRequest(ctx, req)
	require.NotNil(t, resp.Error)
	assert.Equal(t, ErrorCodeInternalError, resp.Error.Code)
	assert.Contains(t, resp.Error.Message, "context canceled")
}

func TestMultipleMethodRegistrations(t *testing.T) {
	handler := NewDefaultHandler()

	handler.RegisterMethod("method1", func(ctx context.Context, params json.RawMessage) (interface{}, error) {
		return "result1", nil
	})

	handler.RegisterMethod("method2", func(ctx context.Context, params json.RawMessage) (interface{}, error) {
		return "result2", nil
	})

	// Call method1.
	req1 := &Request{JSONRPC: "2.0", ID: 1, Method: "method1"}
	resp1 := handler.HandleRequest(context.Background(), req1)
	assert.Equal(t, "result1", resp1.Result)

	// Call method2.
	req2 := &Request{JSONRPC: "2.0", ID: 2, Method: "method2"}
	resp2 := handler.HandleRequest(context.Background(), req2)
	assert.Equal(t, "result2", resp2.Result)
}

func TestMultipleNotificationRegistrations(t *testing.T) {
	handler := NewDefaultHandler()

	count1 := 0
	count2 := 0

	handler.RegisterNotification("notif1", func(ctx context.Context, params json.RawMessage) error {
		count1++
		return nil
	})

	handler.RegisterNotification("notif2", func(ctx context.Context, params json.RawMessage) error {
		count2++
		return nil
	})

	// Send notif1.
	notif1 := &Notification{JSONRPC: "2.0", Method: "notif1"}
	err := handler.HandleNotification(context.Background(), notif1)
	assert.NoError(t, err)
	assert.Equal(t, 1, count1)
	assert.Equal(t, 0, count2)

	// Send notif2.
	notif2 := &Notification{JSONRPC: "2.0", Method: "notif2"}
	err = handler.HandleNotification(context.Background(), notif2)
	assert.NoError(t, err)
	assert.Equal(t, 1, count1)
	assert.Equal(t, 1, count2)
}
