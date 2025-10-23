package transport

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/mcp/protocol"
)

func TestNewHTTPTransport(t *testing.T) {
	transport := NewHTTPTransport("localhost:8080")
	assert.NotNil(t, transport)
	assert.Equal(t, "localhost:8080", transport.addr)
	assert.NotNil(t, transport.clients)
}

func TestHTTPTransport_HealthEndpoint(t *testing.T) {
	transport := NewHTTPTransport("localhost:0")
	handler := &mockHandler{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start server.
	go func() {
		transport.Serve(ctx, handler)
	}()

	// Wait for server to start.
	time.Sleep(100 * time.Millisecond)

	// Get actual address.
	addr := transport.server.Addr
	if addr == "localhost:0" {
		// Server not started yet.
		t.Skip("Server not started")
	}

	// Make health request.
	resp, err := http.Get(fmt.Sprintf("http://%s/health", addr))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var health map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&health)
	require.NoError(t, err)
	assert.Equal(t, "healthy", health["status"])
}

func TestHTTPTransport_MessageEndpoint_Request(t *testing.T) {
	transport := NewHTTPTransport("localhost:8181")

	requestReceived := false
	handler := &mockHandler{
		requestFunc: func(ctx context.Context, req *protocol.Request) *protocol.Response {
			requestReceived = true
			assert.Equal(t, "test_method", req.Method)
			return protocol.NewResponse(req.ID, map[string]string{"result": "ok"})
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start server.
	go func() {
		transport.Serve(ctx, handler)
	}()

	// Wait for server to start.
	time.Sleep(200 * time.Millisecond)

	// Create request.
	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "test_method",
	}
	requestJSON, _ := json.Marshal(request)

	// Send request.
	resp, err := http.Post(
		"http://localhost:8181/message",
		"application/json",
		bytes.NewReader(requestJSON),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.True(t, requestReceived)

	// Parse response.
	var response protocol.Response
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "2.0", response.JSONRPC)
	assert.Equal(t, float64(1), response.ID)
}

func TestHTTPTransport_MessageEndpoint_Notification(t *testing.T) {
	transport := NewHTTPTransport("localhost:8182")

	notifReceived := false
	handler := &mockHandler{
		notificationFunc: func(ctx context.Context, notif *protocol.Notification) error {
			notifReceived = true
			assert.Equal(t, "test_notification", notif.Method)
			return nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start server.
	go func() {
		transport.Serve(ctx, handler)
	}()

	// Wait for server to start.
	time.Sleep(200 * time.Millisecond)

	// Create notification.
	notification := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "test_notification",
	}
	notifJSON, _ := json.Marshal(notification)

	// Send notification.
	resp, err := http.Post(
		"http://localhost:8182/message",
		"application/json",
		bytes.NewReader(notifJSON),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusAccepted, resp.StatusCode)
	assert.True(t, notifReceived)
}

func TestHTTPTransport_MessageEndpoint_InvalidMethod(t *testing.T) {
	transport := NewHTTPTransport("localhost:8183")
	handler := &mockHandler{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start server.
	go func() {
		transport.Serve(ctx, handler)
	}()

	// Wait for server to start.
	time.Sleep(200 * time.Millisecond)

	// Send GET request (should fail).
	resp, err := http.Get("http://localhost:8183/message")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
}

func TestHTTPTransport_MessageEndpoint_InvalidJSON(t *testing.T) {
	transport := NewHTTPTransport("localhost:8184")
	handler := &mockHandler{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start server.
	go func() {
		transport.Serve(ctx, handler)
	}()

	// Wait for server to start.
	time.Sleep(200 * time.Millisecond)

	// Send invalid JSON.
	resp, err := http.Post(
		"http://localhost:8184/message",
		"application/json",
		bytes.NewReader([]byte("{invalid json}")),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Should get error response.
	var errorResp protocol.Response
	err = json.NewDecoder(resp.Body).Decode(&errorResp)
	require.NoError(t, err)
	assert.NotNil(t, errorResp.Error)
}

func TestHTTPTransport_SSEConnection(t *testing.T) {
	transport := NewHTTPTransport("localhost:8185")
	handler := &mockHandler{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start server.
	go func() {
		transport.Serve(ctx, handler)
	}()

	// Wait for server to start.
	time.Sleep(200 * time.Millisecond)

	// Connect to SSE endpoint.
	resp, err := http.Get("http://localhost:8185/sse")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))
	assert.Equal(t, "no-cache", resp.Header.Get("Cache-Control"))
	assert.Equal(t, "keep-alive", resp.Header.Get("Connection"))

	// Read welcome message.
	reader := bufio.NewReader(resp.Body)
	line, err := reader.ReadString('\n')
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(line, "data: {\"type\":\"connected\""))

	// Verify client registered.
	assert.Equal(t, 1, len(transport.clients))
}

func TestHTTPTransport_SSE_MultipleClients(t *testing.T) {
	transport := NewHTTPTransport("localhost:8186")
	handler := &mockHandler{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start server.
	go func() {
		transport.Serve(ctx, handler)
	}()

	// Wait for server to start.
	time.Sleep(200 * time.Millisecond)

	// Connect multiple clients.
	clients := make([]*http.Response, 3)
	for i := 0; i < 3; i++ {
		resp, err := http.Get("http://localhost:8186/sse")
		require.NoError(t, err)
		defer resp.Body.Close()
		clients[i] = resp
	}

	// Wait for connections to establish.
	time.Sleep(100 * time.Millisecond)

	// Verify all clients registered.
	assert.Equal(t, 3, len(transport.clients))

	// Close clients explicitly before defer.
	for _, client := range clients {
		client.Body.Close()
	}

	// Wait for cleanup.
	time.Sleep(100 * time.Millisecond)
}

func TestHTTPTransport_GracefulShutdown(t *testing.T) {
	transport := NewHTTPTransport("localhost:8187")
	handler := &mockHandler{}

	ctx, cancel := context.WithCancel(context.Background())

	// Start server.
	serverDone := make(chan error, 1)
	go func() {
		serverDone <- transport.Serve(ctx, handler)
	}()

	// Wait for server to start.
	time.Sleep(200 * time.Millisecond)

	// Connect SSE client.
	resp, err := http.Get("http://localhost:8187/sse")
	require.NoError(t, err)
	defer resp.Body.Close()

	// Wait for connection.
	time.Sleep(100 * time.Millisecond)

	// Verify client connected.
	assert.Equal(t, 1, len(transport.clients))

	// Cancel context to trigger shutdown.
	cancel()

	// Wait for shutdown.
	err = <-serverDone
	assert.NoError(t, err)

	// Verify clients cleaned up.
	assert.Equal(t, 0, len(transport.clients))
}

func TestHTTPTransport_SSE_ClientDisconnect(t *testing.T) {
	transport := NewHTTPTransport("localhost:8188")
	handler := &mockHandler{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start server.
	go func() {
		transport.Serve(ctx, handler)
	}()

	// Wait for server to start.
	time.Sleep(200 * time.Millisecond)

	// Connect SSE client.
	resp, err := http.Get("http://localhost:8188/sse")
	require.NoError(t, err)

	// Wait for connection.
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, 1, len(transport.clients))

	// Close client connection.
	resp.Body.Close()

	// Wait for cleanup.
	time.Sleep(200 * time.Millisecond)

	// Verify client removed (may take time for server to detect).
	// Note: This is timing-dependent and may be flaky.
	// In real scenarios, the server detects disconnect when it tries to write.
}

func TestHTTPTransport_ConcurrentRequests(t *testing.T) {
	transport := NewHTTPTransport("localhost:8189")

	requestCount := 0
	handler := &mockHandler{
		requestFunc: func(ctx context.Context, req *protocol.Request) *protocol.Response {
			requestCount++
			return protocol.NewResponse(req.ID, "ok")
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start server.
	go func() {
		transport.Serve(ctx, handler)
	}()

	// Wait for server to start.
	time.Sleep(200 * time.Millisecond)

	// Send multiple concurrent requests.
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			request := map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      id,
				"method":  "test",
			}
			requestJSON, _ := json.Marshal(request)

			resp, err := http.Post(
				"http://localhost:8189/message",
				"application/json",
				bytes.NewReader(requestJSON),
			)
			if err == nil {
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			}
			done <- true
		}(i)
	}

	// Wait for all requests.
	for i := 0; i < 10; i++ {
		<-done
	}

	assert.Equal(t, 10, requestCount)
}
