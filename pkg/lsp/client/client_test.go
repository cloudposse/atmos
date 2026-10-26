package client

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/lsp"
	"github.com/cloudposse/atmos/pkg/schema"
)

// newTestClient creates a Client with mock I/O for unit testing.
// It does not start a real process.
func newTestClient(t *testing.T) (*Client, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	c := &Client{
		diagnostics:  make(map[string][]lsp.Diagnostic),
		pendingCalls: make(map[int64]chan *jsonRPCResponse),
		rootURI:      "file:///test",
		name:         "test-server",
		config: &schema.LSPServer{
			Command:   "echo",
			FileTypes: []string{"yaml", "yml"},
		},
		ctx:    ctx,
		cancel: cancel,
	}
	return c, cancel
}

// TestClientNewClient_NilConfig verifies that NewClient returns an error when config is nil.
func TestClientNewClient_NilConfig(t *testing.T) {
	_, err := NewClient(context.Background(), "test", nil, "file:///root")
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrAIInvalidConfiguration),
		"Expected ErrAIInvalidConfiguration, got: %v", err)
	assert.Contains(t, err.Error(), "LSP server config is nil")
}

// TestClientNewClient_EmptyCommand verifies that NewClient returns an error when command is empty.
func TestClientNewClient_EmptyCommand(t *testing.T) {
	config := &schema.LSPServer{
		Command: "",
	}
	_, err := NewClient(context.Background(), "test", config, "file:///root")
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrAIInvalidConfiguration),
		"Expected ErrAIInvalidConfiguration, got: %v", err)
	assert.Contains(t, err.Error(), "LSP server command is empty")
}

// TestClientNewClient_InvalidCommand verifies that NewClient returns an error when the command is not found.
func TestClientNewClient_InvalidCommand(t *testing.T) {
	config := &schema.LSPServer{
		Command: "nonexistent-binary-that-does-not-exist-12345",
	}
	_, err := NewClient(context.Background(), "test", config, "file:///root")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to start LSP server")
}

// TestClientGetDiagnostics_EmptyMap verifies GetDiagnostics returns empty slice for unknown URI.
func TestClientGetDiagnostics_EmptyMap(t *testing.T) {
	c, cancel := newTestClient(t)
	defer cancel()

	result := c.GetDiagnostics("file:///unknown.yaml")
	assert.NotNil(t, result)
	assert.Empty(t, result)
}

// TestClientGetDiagnostics_WithData verifies GetDiagnostics returns a copy of stored diagnostics.
func TestClientGetDiagnostics_WithData(t *testing.T) {
	c, cancel := newTestClient(t)
	defer cancel()

	uri := "file:///test/file.yaml"
	diags := []lsp.Diagnostic{
		{
			Range:    lsp.Range{Start: lsp.Position{Line: 1, Character: 0}},
			Severity: lsp.DiagnosticSeverityError,
			Message:  "test error",
			Source:   "test-source",
		},
		{
			Range:    lsp.Range{Start: lsp.Position{Line: 5, Character: 3}},
			Severity: lsp.DiagnosticSeverityWarning,
			Message:  "test warning",
		},
	}
	c.diagnostics[uri] = diags

	result := c.GetDiagnostics(uri)
	assert.Len(t, result, 2)
	assert.Equal(t, "test error", result[0].Message)
	assert.Equal(t, "test warning", result[1].Message)

	// Verify it is a copy (modifying result does not affect original).
	result[0].Message = "modified"
	assert.Equal(t, "test error", c.diagnostics[uri][0].Message)
}

// TestClientGetAllDiagnostics_Empty verifies GetAllDiagnostics returns empty map when no diagnostics exist.
func TestClientGetAllDiagnostics_Empty(t *testing.T) {
	c, cancel := newTestClient(t)
	defer cancel()

	result := c.GetAllDiagnostics()
	assert.NotNil(t, result)
	assert.Empty(t, result)
}

// TestClientGetAllDiagnostics_WithMultipleURIs verifies GetAllDiagnostics returns copies of all diagnostics.
func TestClientGetAllDiagnostics_WithMultipleURIs(t *testing.T) {
	c, cancel := newTestClient(t)
	defer cancel()

	uri1 := "file:///test/a.yaml"
	uri2 := "file:///test/b.yaml"
	c.diagnostics[uri1] = []lsp.Diagnostic{
		{Severity: lsp.DiagnosticSeverityError, Message: "error in a"},
	}
	c.diagnostics[uri2] = []lsp.Diagnostic{
		{Severity: lsp.DiagnosticSeverityWarning, Message: "warning in b"},
		{Severity: lsp.DiagnosticSeverityHint, Message: "hint in b"},
	}

	result := c.GetAllDiagnostics()
	assert.Len(t, result, 2)
	assert.Len(t, result[uri1], 1)
	assert.Len(t, result[uri2], 2)

	// Verify it is a copy.
	result[uri1][0].Message = "modified"
	assert.Equal(t, "error in a", c.diagnostics[uri1][0].Message)
}

// TestClientUpdateDiagnostics verifies updateDiagnostics stores diagnostics correctly.
func TestClientUpdateDiagnostics(t *testing.T) {
	c, cancel := newTestClient(t)
	defer cancel()

	uri := "file:///test/update.yaml"

	// Initial update.
	diags1 := []lsp.Diagnostic{
		{Severity: lsp.DiagnosticSeverityError, Message: "first error"},
	}
	c.updateDiagnostics(uri, diags1)
	assert.Len(t, c.diagnostics[uri], 1)
	assert.Equal(t, "first error", c.diagnostics[uri][0].Message)

	// Replace with new diagnostics.
	diags2 := []lsp.Diagnostic{
		{Severity: lsp.DiagnosticSeverityWarning, Message: "warning"},
		{Severity: lsp.DiagnosticSeverityHint, Message: "hint"},
	}
	c.updateDiagnostics(uri, diags2)
	assert.Len(t, c.diagnostics[uri], 2)
	assert.Equal(t, "warning", c.diagnostics[uri][0].Message)

	// Clear diagnostics.
	c.updateDiagnostics(uri, nil)
	assert.Nil(t, c.diagnostics[uri])
}

// TestClientUpdateDiagnostics_ConcurrentAccess verifies updateDiagnostics is safe for concurrent use.
func TestClientUpdateDiagnostics_ConcurrentAccess(t *testing.T) {
	c, cancel := newTestClient(t)
	defer cancel()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			uri := "file:///test/concurrent.yaml"
			diags := []lsp.Diagnostic{
				{Message: "diagnostic"},
			}
			c.updateDiagnostics(uri, diags)
			_ = c.GetDiagnostics(uri)
		}()
	}
	wg.Wait()
}

// TestClientMustMarshalJSON_ValidInput verifies mustMarshalJSON marshals valid input.
func TestClientMustMarshalJSON_ValidInput(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{
			name:     "string value",
			input:    "hello",
			expected: `"hello"`,
		},
		{
			name:     "integer value",
			input:    42,
			expected: "42",
		},
		{
			name:     "struct value",
			input:    struct{ Name string }{"test"},
			expected: `{"Name":"test"}`,
		},
		{
			name:     "map value",
			input:    map[string]int{"a": 1},
			expected: `{"a":1}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mustMarshalJSON(tt.input)
			assert.JSONEq(t, tt.expected, string(result))
		})
	}
}

// TestClientMustMarshalJSON_Nil verifies mustMarshalJSON returns nil for nil input.
func TestClientMustMarshalJSON_Nil(t *testing.T) {
	result := mustMarshalJSON(nil)
	assert.Nil(t, result)
}

// TestClientMustMarshalJSON_Panics verifies mustMarshalJSON panics on unmarshallable input.
func TestClientMustMarshalJSON_Panics(t *testing.T) {
	assert.Panics(t, func() {
		// Channels cannot be marshaled to JSON.
		mustMarshalJSON(make(chan int))
	})
}

// TestClientReadContentLength_ValidHeader verifies readContentLength parses valid LSP headers.
func TestClientReadContentLength_ValidHeader(t *testing.T) {
	c, cancel := newTestClient(t)
	defer cancel()

	tests := []struct {
		name           string
		input          string
		expectedLength int
	}{
		{
			name:           "standard header",
			input:          "Content-Length: 42\r\n\r\n",
			expectedLength: 42,
		},
		{
			name:           "header with extra whitespace",
			input:          "Content-Length:  100 \r\n\r\n",
			expectedLength: 100,
		},
		{
			name:           "zero length",
			input:          "Content-Length: 0\r\n\r\n",
			expectedLength: 0,
		},
		{
			name:           "large content length",
			input:          "Content-Length: 999999\r\n\r\n",
			expectedLength: 999999,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := bufio.NewReader(strings.NewReader(tt.input))
			length, err := c.readContentLength(reader)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedLength, length)
		})
	}
}

// TestClientReadContentLength_EmptyLine verifies readContentLength returns error for empty line without header.
func TestClientReadContentLength_EmptyLine(t *testing.T) {
	c, cancel := newTestClient(t)
	defer cancel()

	reader := bufio.NewReader(strings.NewReader("\r\n"))
	_, err := c.readContentLength(reader)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrLSPNoContentLengthHeader))
}

// TestClientReadContentLength_InvalidLength verifies readContentLength returns error for non-numeric length.
func TestClientReadContentLength_InvalidLength(t *testing.T) {
	c, cancel := newTestClient(t)
	defer cancel()

	reader := bufio.NewReader(strings.NewReader("Content-Length: abc\r\n\r\n"))
	_, err := c.readContentLength(reader)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid Content-Length")
}

// TestClientReadContentLength_EOF verifies readContentLength returns error on unexpected EOF.
func TestClientReadContentLength_EOF(t *testing.T) {
	c, cancel := newTestClient(t)
	defer cancel()

	reader := bufio.NewReader(strings.NewReader(""))
	_, err := c.readContentLength(reader)
	require.Error(t, err)
}

// TestClientReadContentLength_MissingBlankLine verifies readContentLength handles missing blank line after header.
func TestClientReadContentLength_MissingBlankLine(t *testing.T) {
	c, cancel := newTestClient(t)
	defer cancel()

	// Content-Length header present but no trailing blank line (EOF after header).
	reader := bufio.NewReader(strings.NewReader("Content-Length: 10\r\n"))
	_, err := c.readContentLength(reader)
	require.Error(t, err)
}

// TestClientProcessMessage_Notification verifies processMessage handles notification messages.
func TestClientProcessMessage_Notification(t *testing.T) {
	c, cancel := newTestClient(t)
	defer cancel()

	diags := []lsp.Diagnostic{
		{
			Range:    lsp.Range{Start: lsp.Position{Line: 0, Character: 0}},
			Severity: lsp.DiagnosticSeverityError,
			Message:  "test notification error",
		},
	}
	params := lsp.PublishDiagnosticsParams{
		URI:         "file:///test/notif.yaml",
		Diagnostics: diags,
	}
	paramsJSON, err := json.Marshal(params)
	require.NoError(t, err)

	notification := jsonRPCNotification{
		JSONRPC: "2.0",
		Method:  "textDocument/publishDiagnostics",
		Params:  paramsJSON,
	}
	data, err := json.Marshal(notification)
	require.NoError(t, err)

	c.processMessage(data)

	result := c.GetDiagnostics("file:///test/notif.yaml")
	require.Len(t, result, 1)
	assert.Equal(t, "test notification error", result[0].Message)
}

// TestClientProcessMessage_Response verifies processMessage handles response messages.
func TestClientProcessMessage_Response(t *testing.T) {
	c, cancel := newTestClient(t)
	defer cancel()

	// Register a pending call.
	responseChan := make(chan *jsonRPCResponse, 1)
	c.pendingCallsMu.Lock()
	c.pendingCalls[1] = responseChan
	c.pendingCallsMu.Unlock()

	resultJSON, err := json.Marshal(map[string]string{"status": "ok"})
	require.NoError(t, err)

	response := jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      1,
		Result:  resultJSON,
	}
	data, err := json.Marshal(response)
	require.NoError(t, err)

	c.processMessage(data)

	// Verify response was delivered to the channel.
	received := <-responseChan
	assert.Equal(t, int64(1), received.ID)
	assert.Nil(t, received.Error)
}

// TestClientProcessMessage_UnknownMessage verifies processMessage handles unknown message types gracefully.
func TestClientProcessMessage_UnknownMessage(t *testing.T) {
	c, cancel := newTestClient(t)
	defer cancel()

	// A message with no method and no positive ID is treated as unknown.
	data := []byte(`{"jsonrpc":"2.0"}`)
	// Should not panic.
	c.processMessage(data)
}

// TestClientProcessMessage_InvalidJSON verifies processMessage handles invalid JSON gracefully.
func TestClientProcessMessage_InvalidJSON(t *testing.T) {
	c, cancel := newTestClient(t)
	defer cancel()

	// Should not panic.
	c.processMessage([]byte("not json"))
}

// TestClientHandleNotification_PublishDiagnostics verifies handleNotification processes publishDiagnostics.
func TestClientHandleNotification_PublishDiagnostics(t *testing.T) {
	c, cancel := newTestClient(t)
	defer cancel()

	uri := "file:///test/handle.yaml"
	params := lsp.PublishDiagnosticsParams{
		URI: uri,
		Diagnostics: []lsp.Diagnostic{
			{Severity: lsp.DiagnosticSeverityWarning, Message: "handled warning"},
		},
	}
	paramsJSON, err := json.Marshal(params)
	require.NoError(t, err)

	notification := &jsonRPCNotification{
		JSONRPC: "2.0",
		Method:  "textDocument/publishDiagnostics",
		Params:  paramsJSON,
	}

	c.handleNotification(notification)

	result := c.GetDiagnostics(uri)
	require.Len(t, result, 1)
	assert.Equal(t, "handled warning", result[0].Message)
}

// TestClientHandleNotification_UnknownMethod verifies handleNotification ignores unknown methods.
func TestClientHandleNotification_UnknownMethod(t *testing.T) {
	c, cancel := newTestClient(t)
	defer cancel()

	notification := &jsonRPCNotification{
		JSONRPC: "2.0",
		Method:  "window/logMessage",
		Params:  json.RawMessage(`{"type":3,"message":"log"}`),
	}

	// Should not panic and should not modify diagnostics.
	c.handleNotification(notification)
	assert.Empty(t, c.diagnostics)
}

// TestClientHandleNotification_InvalidParams verifies handleNotification handles invalid params gracefully.
func TestClientHandleNotification_InvalidParams(t *testing.T) {
	c, cancel := newTestClient(t)
	defer cancel()

	notification := &jsonRPCNotification{
		JSONRPC: "2.0",
		Method:  "textDocument/publishDiagnostics",
		Params:  json.RawMessage(`{invalid json`),
	}

	// Should not panic and should not modify diagnostics.
	c.handleNotification(notification)
	assert.Empty(t, c.diagnostics)
}

// TestClientHandleResponse_ExistingPendingCall verifies handleResponse delivers to pending call channel.
func TestClientHandleResponse_ExistingPendingCall(t *testing.T) {
	c, cancel := newTestClient(t)
	defer cancel()

	ch := make(chan *jsonRPCResponse, 1)
	c.pendingCallsMu.Lock()
	c.pendingCalls[42] = ch
	c.pendingCallsMu.Unlock()

	response := &jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      42,
		Result:  json.RawMessage(`{"ok":true}`),
	}

	c.handleResponse(response)

	received := <-ch
	assert.Equal(t, int64(42), received.ID)
}

// TestClientHandleResponse_NoPendingCall verifies handleResponse handles response with no pending call.
func TestClientHandleResponse_NoPendingCall(t *testing.T) {
	c, cancel := newTestClient(t)
	defer cancel()

	response := &jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      999,
		Result:  json.RawMessage(`{}`),
	}

	// Should not panic when no pending call exists.
	c.handleResponse(response)
}

// TestClientHandleResponse_CancelledContext verifies handleResponse handles cancelled context.
func TestClientHandleResponse_CancelledContext(t *testing.T) {
	c, cancel := newTestClient(t)

	// Create a full (unbuffered) channel so send would block.
	ch := make(chan *jsonRPCResponse)
	c.pendingCallsMu.Lock()
	c.pendingCalls[7] = ch
	c.pendingCallsMu.Unlock()

	// Cancel context so the select falls through.
	cancel()

	response := &jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      7,
		Result:  json.RawMessage(`{}`),
	}

	// Should not block or panic.
	c.handleResponse(response)
}

// TestClientSendMessage_WritesLSPFrame verifies sendMessage writes correct LSP framing.
func TestClientSendMessage_WritesLSPFrame(t *testing.T) {
	c, cancel := newTestClient(t)
	defer cancel()

	pr, pw := io.Pipe()
	c.stdin = pw

	request := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
	}

	// Read in a goroutine to avoid blocking.
	done := make(chan string, 1)
	go func() {
		buf := make([]byte, 4096)
		n, _ := pr.Read(buf)
		// May need a second read for the body.
		n2, _ := pr.Read(buf[n:])
		done <- string(buf[:n+n2])
	}()

	err := c.sendMessage(request)
	require.NoError(t, err)

	received := <-done
	assert.Contains(t, received, "Content-Length:")
	assert.Contains(t, received, `"jsonrpc":"2.0"`)
	assert.Contains(t, received, `"method":"initialize"`)
	assert.Contains(t, received, "\r\n\r\n")
	pr.Close()
}

// TestClientSendMessage_WriteError verifies sendMessage returns error when write fails.
func TestClientSendMessage_WriteError(t *testing.T) {
	c, cancel := newTestClient(t)
	defer cancel()

	pr, pw := io.Pipe()
	c.stdin = pw
	// Close the reader so writes will fail.
	pr.Close()

	request := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "test",
	}

	err := c.sendMessage(request)
	require.Error(t, err)
}

// TestClientNotify_SendsNotification verifies notify sends a properly formatted notification.
func TestClientNotify_SendsNotification(t *testing.T) {
	c, cancel := newTestClient(t)
	defer cancel()

	pr, pw := io.Pipe()
	c.stdin = pw

	done := make(chan string, 1)
	go func() {
		buf := make([]byte, 4096)
		total := 0
		for total < 50 {
			n, err := pr.Read(buf[total:])
			total += n
			if err != nil {
				break
			}
		}
		done <- string(buf[:total])
	}()

	err := c.notify("initialized", struct{}{})
	require.NoError(t, err)

	received := <-done
	assert.Contains(t, received, "Content-Length:")
	assert.Contains(t, received, `"method":"initialized"`)
	// Notifications should not have an ID field in the output.
	pr.Close()
}

// TestClientOpenDocument_NotInitialized verifies OpenDocument returns error when client is not initialized.
func TestClientOpenDocument_NotInitialized(t *testing.T) {
	c, cancel := newTestClient(t)
	defer cancel()

	c.initialized = false
	err := c.OpenDocument("file:///test.yaml", "yaml", "content: true")
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrAINotInitialized))
}

// TestClientCloseDocument_NotInitialized verifies CloseDocument returns error when client is not initialized.
func TestClientCloseDocument_NotInitialized(t *testing.T) {
	c, cancel := newTestClient(t)
	defer cancel()

	c.initialized = false
	err := c.CloseDocument("file:///test.yaml")
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrAINotInitialized))
}

// TestClientCall_ContextCancelled verifies call returns error when context is cancelled.
func TestClientCall_ContextCancelled(t *testing.T) {
	c, cancel := newTestClient(t)

	pr, pw := io.Pipe()
	c.stdin = pw

	// Drain writes so sendMessage does not block.
	go func() {
		buf := make([]byte, 4096)
		for {
			_, err := pr.Read(buf)
			if err != nil {
				return
			}
		}
	}()

	// Cancel context before response arrives.
	cancel()

	err := c.call("test/method", nil, nil)
	require.Error(t, err)
	pr.Close()
}

// TestClientCall_RPCError verifies call returns error when server responds with an RPC error.
func TestClientCall_RPCError(t *testing.T) {
	c, cancel := newTestClient(t)
	defer cancel()

	pr, pw := io.Pipe()
	c.stdin = pw

	// Drain writes so sendMessage does not block.
	go func() {
		buf := make([]byte, 4096)
		for {
			_, err := pr.Read(buf)
			if err != nil {
				return
			}
		}
	}()

	// Simulate server response in a goroutine.
	go func() {
		// Wait for the pending call to be registered.
		for {
			c.pendingCallsMu.RLock()
			_, exists := c.pendingCalls[1]
			c.pendingCallsMu.RUnlock()
			if exists {
				break
			}
		}

		c.pendingCallsMu.RLock()
		ch := c.pendingCalls[1]
		c.pendingCallsMu.RUnlock()

		ch <- &jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      1,
			Error: &jsonRPCError{
				Code:    -32600,
				Message: "Invalid Request",
			},
		}
	}()

	err := c.call("test/method", nil, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrLSPRPCError))
	assert.Contains(t, err.Error(), "Invalid Request")
	pr.Close()
}

// TestClientCall_SuccessWithResult verifies call unmarshals result on success.
func TestClientCall_SuccessWithResult(t *testing.T) {
	c, cancel := newTestClient(t)
	defer cancel()

	pr, pw := io.Pipe()
	c.stdin = pw

	// Drain writes so sendMessage does not block.
	go func() {
		buf := make([]byte, 4096)
		for {
			_, err := pr.Read(buf)
			if err != nil {
				return
			}
		}
	}()

	// Simulate server response in a goroutine.
	go func() {
		// Wait for the pending call to be registered.
		for {
			c.pendingCallsMu.RLock()
			_, exists := c.pendingCalls[1]
			c.pendingCallsMu.RUnlock()
			if exists {
				break
			}
		}

		c.pendingCallsMu.RLock()
		ch := c.pendingCalls[1]
		c.pendingCallsMu.RUnlock()

		ch <- &jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      1,
			Result:  json.RawMessage(`{"capabilities":{}}`),
		}
	}()

	var result lsp.InitializeResult
	err := c.call("initialize", nil, &result)
	require.NoError(t, err)
	pr.Close()
}

// TestClientCall_SuccessNilResult verifies call works when result pointer is nil.
func TestClientCall_SuccessNilResult(t *testing.T) {
	c, cancel := newTestClient(t)
	defer cancel()

	pr, pw := io.Pipe()
	c.stdin = pw

	// Drain writes.
	go func() {
		buf := make([]byte, 4096)
		for {
			_, err := pr.Read(buf)
			if err != nil {
				return
			}
		}
	}()

	// Simulate server response.
	go func() {
		for {
			c.pendingCallsMu.RLock()
			_, exists := c.pendingCalls[1]
			c.pendingCallsMu.RUnlock()
			if exists {
				break
			}
		}

		c.pendingCallsMu.RLock()
		ch := c.pendingCalls[1]
		c.pendingCallsMu.RUnlock()

		ch <- &jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      1,
			Result:  json.RawMessage(`null`),
		}
	}()

	err := c.call("shutdown", nil, nil)
	require.NoError(t, err)
	pr.Close()
}

// TestClientClose_NotInitialized verifies Close works when client is not initialized.
func TestClientClose_NotInitialized(t *testing.T) {
	c, cancel := newTestClient(t)
	_ = cancel

	// Set up mock I/O pipes so Close can close them.
	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	stderrR, stderrW := io.Pipe()

	c.stdin = stdinW
	c.stdout = stdoutR
	c.stderr = stderrR
	c.initialized = false

	// Set up a dummy exec.Cmd so Close does not nil-pointer on c.cmd.Process.
	c.cmd = exec.Command("echo")

	// Close the write ends so the read ends are not blocked.
	stdoutW.Close()
	stderrW.Close()

	// Close should not attempt shutdown/exit when not initialized.
	err := c.Close()
	assert.NoError(t, err)
	stdinR.Close()
}

// TestClientReadContentLength_MultipleHeaderLines verifies readContentLength skips non-Content-Length headers.
func TestClientReadContentLength_MultipleHeaderLines(t *testing.T) {
	c, cancel := newTestClient(t)
	defer cancel()

	// The readContentLength function reads lines until it finds Content-Length.
	// If it encounters a non-Content-Length, non-empty line, it continues.
	input := "Content-Type: application/json\r\nContent-Length: 55\r\n\r\n"
	reader := bufio.NewReader(strings.NewReader(input))
	length, err := c.readContentLength(reader)
	require.NoError(t, err)
	assert.Equal(t, 55, length)
}

// TestClientMessageIDIncrement verifies that message IDs increment atomically.
func TestClientMessageIDIncrement(t *testing.T) {
	c, cancel := newTestClient(t)
	defer cancel()

	id1 := c.messageID.Add(1)
	id2 := c.messageID.Add(1)
	id3 := c.messageID.Add(1)

	assert.Equal(t, int64(1), id1)
	assert.Equal(t, int64(2), id2)
	assert.Equal(t, int64(3), id3)
}

// TestClientProcessMessage_ResponseWithError verifies processMessage handles error responses.
func TestClientProcessMessage_ResponseWithError(t *testing.T) {
	c, cancel := newTestClient(t)
	defer cancel()

	ch := make(chan *jsonRPCResponse, 1)
	c.pendingCallsMu.Lock()
	c.pendingCalls[5] = ch
	c.pendingCallsMu.Unlock()

	response := jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      5,
		Error: &jsonRPCError{
			Code:    -32601,
			Message: "Method not found",
		},
	}
	data, err := json.Marshal(response)
	require.NoError(t, err)

	c.processMessage(data)

	received := <-ch
	assert.NotNil(t, received.Error)
	assert.Equal(t, -32601, received.Error.Code)
	assert.Equal(t, "Method not found", received.Error.Message)
}

// TestClientOpenDocument_Initialized verifies OpenDocument sends notification when initialized.
func TestClientOpenDocument_Initialized(t *testing.T) {
	c, cancel := newTestClient(t)
	defer cancel()

	pr, pw := io.Pipe()
	c.stdin = pw
	c.initialized = true

	// Drain writes.
	done := make(chan string, 1)
	go func() {
		buf := make([]byte, 4096)
		total := 0
		for total < 50 {
			n, err := pr.Read(buf[total:])
			total += n
			if err != nil {
				break
			}
		}
		done <- string(buf[:total])
	}()

	err := c.OpenDocument("file:///test.yaml", "yaml", "key: value")
	require.NoError(t, err)

	received := <-done
	assert.Contains(t, received, "Content-Length:")
	assert.Contains(t, received, `"method":"textDocument/didOpen"`)
	pr.Close()
}

// TestClientCloseDocument_Initialized verifies CloseDocument sends notification when initialized.
func TestClientCloseDocument_Initialized(t *testing.T) {
	c, cancel := newTestClient(t)
	defer cancel()

	pr, pw := io.Pipe()
	c.stdin = pw
	c.initialized = true

	// Drain writes.
	done := make(chan string, 1)
	go func() {
		buf := make([]byte, 4096)
		total := 0
		for total < 50 {
			n, err := pr.Read(buf[total:])
			total += n
			if err != nil {
				break
			}
		}
		done <- string(buf[:total])
	}()

	err := c.CloseDocument("file:///test.yaml")
	require.NoError(t, err)

	received := <-done
	assert.Contains(t, received, "Content-Length:")
	assert.Contains(t, received, `"method":"textDocument/didClose"`)
	pr.Close()
}

// TestClientClose_Initialized verifies Close sends shutdown and exit when initialized.
func TestClientClose_Initialized(t *testing.T) {
	c, cancel := newTestClient(t)

	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	stderrR, stderrW := io.Pipe()

	c.stdin = stdinW
	c.stdout = stdoutR
	c.stderr = stderrR
	c.initialized = true
	c.cmd = exec.Command("echo")

	// Close the write ends so the read ends are not blocked.
	stdoutW.Close()
	stderrW.Close()

	// Cancel context so the call("shutdown") inside Close does not block
	// waiting for a response.
	cancel()

	// Drain stdin writes so the notify/call writes do not block.
	go func() {
		buf := make([]byte, 8192)
		for {
			_, err := stdinR.Read(buf)
			if err != nil {
				return
			}
		}
	}()

	err := c.Close()
	assert.NoError(t, err)
	stdinR.Close()
}

// TestClientCall_UnmarshalError verifies call returns error when result unmarshal fails.
func TestClientCall_UnmarshalError(t *testing.T) {
	c, cancel := newTestClient(t)
	defer cancel()

	pr, pw := io.Pipe()
	c.stdin = pw

	// Drain writes.
	go func() {
		buf := make([]byte, 4096)
		for {
			_, err := pr.Read(buf)
			if err != nil {
				return
			}
		}
	}()

	// Simulate server response with invalid JSON in result.
	go func() {
		for {
			c.pendingCallsMu.RLock()
			_, exists := c.pendingCalls[1]
			c.pendingCallsMu.RUnlock()
			if exists {
				break
			}
		}

		c.pendingCallsMu.RLock()
		ch := c.pendingCalls[1]
		c.pendingCallsMu.RUnlock()

		ch <- &jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      1,
			Result:  json.RawMessage(`{invalid`),
		}
	}()

	var result lsp.InitializeResult
	err := c.call("initialize", nil, &result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal result")
	pr.Close()
}

// TestClientSendMessage_MarshalError verifies sendMessage returns error for unmarshallable message.
func TestClientSendMessage_MarshalError(t *testing.T) {
	c, cancel := newTestClient(t)
	defer cancel()

	pr, pw := io.Pipe()
	c.stdin = pw

	// Use a value that cannot be marshaled.
	err := c.sendMessage(make(chan int))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to marshal message")
	pr.Close()
}
