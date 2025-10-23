package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/mcp/protocol"
)

// mockHandler implements protocol.Handler for testing.
type mockHandler struct {
	requestFunc      func(ctx context.Context, req *protocol.Request) *protocol.Response
	notificationFunc func(ctx context.Context, notif *protocol.Notification) error
}

func (m *mockHandler) HandleRequest(ctx context.Context, req *protocol.Request) *protocol.Response {
	if m.requestFunc != nil {
		return m.requestFunc(ctx, req)
	}
	return protocol.NewResponse(req.ID, map[string]string{"status": "ok"})
}

func (m *mockHandler) HandleNotification(ctx context.Context, notif *protocol.Notification) error {
	if m.notificationFunc != nil {
		return m.notificationFunc(ctx, notif)
	}
	return nil
}

func TestNewStdioTransport(t *testing.T) {
	transport := NewStdioTransport()
	assert.NotNil(t, transport)
}

func TestNewStdioTransportWithStreams(t *testing.T) {
	stdin := &bytes.Buffer{}
	stdout := &bytes.Buffer{}

	transport := NewStdioTransportWithStreams(stdin, stdout)
	assert.NotNil(t, transport)
}

func TestWriteResponse(t *testing.T) {
	stdout := &bytes.Buffer{}
	transport := NewStdioTransportWithStreams(nil, stdout)

	resp := &protocol.Response{
		JSONRPC: "2.0",
		ID:      1,
		Result:  map[string]string{"status": "success"},
	}

	err := transport.WriteResponse(resp)
	require.NoError(t, err)

	// Verify output.
	output := stdout.String()
	assert.Contains(t, output, `"jsonrpc":"2.0"`)
	assert.Contains(t, output, `"id":1`)
	assert.Contains(t, output, `"status":"success"`)
	assert.True(t, strings.HasSuffix(output, "\n"), "output should end with newline")
}

func TestWriteNotification(t *testing.T) {
	stdout := &bytes.Buffer{}
	transport := NewStdioTransportWithStreams(nil, stdout)

	notif := &protocol.Notification{
		JSONRPC: "2.0",
		Method:  "test/notification",
	}

	err := transport.WriteNotification(notif)
	require.NoError(t, err)

	// Verify output.
	output := stdout.String()
	assert.Contains(t, output, `"jsonrpc":"2.0"`)
	assert.Contains(t, output, `"method":"test/notification"`)
	assert.True(t, strings.HasSuffix(output, "\n"), "output should end with newline")
}

func TestServe_Request(t *testing.T) {
	// Prepare input with a request.
	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "test",
	}
	requestJSON, _ := json.Marshal(request)
	stdin := bytes.NewBuffer(append(requestJSON, '\n'))
	stdout := &bytes.Buffer{}

	transport := NewStdioTransportWithStreams(stdin, stdout)

	handler := &mockHandler{
		requestFunc: func(ctx context.Context, req *protocol.Request) *protocol.Response {
			assert.Equal(t, "test", req.Method)
			assert.Equal(t, float64(1), req.ID)
			return protocol.NewResponse(req.ID, map[string]string{"result": "ok"})
		},
	}

	ctx := context.Background()

	err := transport.Serve(ctx, handler)
	// Should return nil when stdin EOF is reached.
	assert.NoError(t, err)

	// Verify response was written.
	output := stdout.String()
	assert.Contains(t, output, `"result":"ok"`)
}

func TestServe_Notification(t *testing.T) {
	// Prepare input with a notification.
	notification := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "test/notif",
	}
	notifJSON, _ := json.Marshal(notification)
	stdin := bytes.NewBuffer(append(notifJSON, '\n'))
	stdout := &bytes.Buffer{}

	transport := NewStdioTransportWithStreams(stdin, stdout)

	notifReceived := false
	handler := &mockHandler{
		notificationFunc: func(ctx context.Context, notif *protocol.Notification) error {
			assert.Equal(t, "test/notif", notif.Method)
			notifReceived = true
			return nil
		},
	}

	ctx := context.Background()

	err := transport.Serve(ctx, handler)
	assert.NoError(t, err)
	assert.True(t, notifReceived)
}

func TestServe_MultipleMessages(t *testing.T) {
	// Prepare multiple messages.
	req1 := map[string]interface{}{"jsonrpc": "2.0", "id": 1, "method": "method1"}
	req2 := map[string]interface{}{"jsonrpc": "2.0", "id": 2, "method": "method2"}
	notif := map[string]interface{}{"jsonrpc": "2.0", "method": "notif1"}

	req1JSON, _ := json.Marshal(req1)
	req2JSON, _ := json.Marshal(req2)
	notifJSON, _ := json.Marshal(notif)

	input := string(req1JSON) + "\n" + string(req2JSON) + "\n" + string(notifJSON) + "\n"
	stdin := bytes.NewBufferString(input)
	stdout := &bytes.Buffer{}

	transport := NewStdioTransportWithStreams(stdin, stdout)

	requestCount := 0
	notifCount := 0

	handler := &mockHandler{
		requestFunc: func(ctx context.Context, req *protocol.Request) *protocol.Response {
			requestCount++
			return protocol.NewResponse(req.ID, "ok")
		},
		notificationFunc: func(ctx context.Context, notif *protocol.Notification) error {
			notifCount++
			return nil
		},
	}

	ctx := context.Background()

	err := transport.Serve(ctx, handler)
	assert.NoError(t, err)
	assert.Equal(t, 2, requestCount, "should handle 2 requests")
	assert.Equal(t, 1, notifCount, "should handle 1 notification")
}

func TestServe_InvalidJSON(t *testing.T) {
	stdin := bytes.NewBufferString("{invalid json}\n")
	stdout := &bytes.Buffer{}

	transport := NewStdioTransportWithStreams(stdin, stdout)
	handler := &mockHandler{}

	ctx := context.Background()

	err := transport.Serve(ctx, handler)
	assert.NoError(t, err)

	// Invalid JSON should not cause panic, just be ignored or logged.
}

func TestServe_ContextCancellation(t *testing.T) {
	// Test that context cancellation is checked between messages.
	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "test",
	}
	requestJSON, _ := json.Marshal(request)
	stdin := bytes.NewBuffer(append(requestJSON, '\n'))
	stdout := &bytes.Buffer{}

	transport := NewStdioTransportWithStreams(stdin, stdout)

	ctxCanceled := false
	handler := &mockHandler{
		requestFunc: func(ctx context.Context, req *protocol.Request) *protocol.Response {
			// Check if context is already canceled.
			select {
			case <-ctx.Done():
				ctxCanceled = true
			default:
			}
			return protocol.NewResponse(req.ID, "ok")
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel before serving.

	err := transport.Serve(ctx, handler)
	// Should return context.Canceled since we canceled before serving.
	assert.ErrorIs(t, err, context.Canceled)
	// Handler shouldn't have been called since context was already canceled.
	assert.False(t, ctxCanceled, "handler should not be called when context is pre-canceled")
}

func TestWriteResponse_ConcurrentWrites(t *testing.T) {
	stdout := &bytes.Buffer{}
	transport := NewStdioTransportWithStreams(nil, stdout)

	// Write multiple responses concurrently.
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			resp := &protocol.Response{
				JSONRPC: "2.0",
				ID:      id,
				Result:  map[string]int{"id": id},
			}
			err := transport.WriteResponse(resp)
			assert.NoError(t, err)
			done <- true
		}(i)
	}

	// Wait for all writes.
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all responses were written (count newlines).
	output := stdout.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	assert.Len(t, lines, 10, "should have 10 response lines")
}

func TestWriteNotification_EmptyParams(t *testing.T) {
	stdout := &bytes.Buffer{}
	transport := NewStdioTransportWithStreams(nil, stdout)

	notif := &protocol.Notification{
		JSONRPC: "2.0",
		Method:  "ping",
	}

	err := transport.WriteNotification(notif)
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, `"method":"ping"`)
	// Should not have params field if empty.
	var parsed map[string]interface{}
	err = json.Unmarshal([]byte(strings.TrimSpace(output)), &parsed)
	require.NoError(t, err)
	_, hasParams := parsed["params"]
	assert.False(t, hasParams, "should not include params field when nil")
}

func TestServe_EmptyInput(t *testing.T) {
	stdin := bytes.NewBuffer([]byte{})
	stdout := &bytes.Buffer{}

	transport := NewStdioTransportWithStreams(stdin, stdout)
	handler := &mockHandler{}

	ctx := context.Background()

	err := transport.Serve(ctx, handler)
	// Should return nil when stdin EOF is reached with no messages.
	assert.NoError(t, err)
}
