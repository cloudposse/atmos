package transport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/mcp/protocol"
)

const (
	readHeaderTimeout = 10 * time.Second
	idleTimeout       = 120 * time.Second
	messageBufferSize = 100
)

// HTTPTransport implements MCP transport over HTTP with Server-Sent Events (SSE).
type HTTPTransport struct {
	addr    string
	server  *http.Server
	handler protocol.Handler

	// Client connections (SSE streams).
	clientsMu sync.RWMutex
	clients   map[string]*sseClient
}

// sseClient represents a connected SSE client.
type sseClient struct {
	id       string
	w        http.ResponseWriter
	flusher  http.Flusher
	done     chan struct{}
	messages chan []byte
}

// NewHTTPTransport creates a new HTTP transport.
func NewHTTPTransport(addr string) *HTTPTransport {
	return &HTTPTransport{
		addr:    addr,
		clients: make(map[string]*sseClient),
	}
}

// Serve starts the HTTP server and processes requests.
func (t *HTTPTransport) Serve(ctx context.Context, handler protocol.Handler) error {
	t.handler = handler

	mux := http.NewServeMux()
	mux.HandleFunc("/sse", t.handleSSE)
	mux.HandleFunc("/message", t.handleMessage)
	mux.HandleFunc("/health", t.handleHealth)

	t.server = &http.Server{
		Addr:              t.addr,
		Handler:           mux,
		ReadHeaderTimeout: readHeaderTimeout,
		WriteTimeout:      0, // No timeout for SSE.
		IdleTimeout:       idleTimeout,
	}

	// Start server in goroutine.
	errChan := make(chan error, 1)
	go func() {
		log.Info(fmt.Sprintf("MCP HTTP server listening on %s", t.addr))
		if err := t.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errChan <- err
		}
	}()

	// Wait for context cancellation or error.
	select {
	case <-ctx.Done():
		return t.shutdown()
	case err := <-errChan:
		return err
	}
}

// handleSSE handles Server-Sent Events connections.
func (t *HTTPTransport) handleSSE(w http.ResponseWriter, r *http.Request) {
	// Verify SSE support.
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	// Set SSE headers.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Create client.
	clientID := fmt.Sprintf("client-%d", time.Now().UnixNano())
	client := &sseClient{
		id:       clientID,
		w:        w,
		flusher:  flusher,
		done:     make(chan struct{}),
		messages: make(chan []byte, messageBufferSize),
	}

	// Register client.
	t.clientsMu.Lock()
	t.clients[clientID] = client
	t.clientsMu.Unlock()

	log.Info(fmt.Sprintf("SSE client connected: %s", clientID))

	// Send welcome message.
	if err := t.sendSSEMessage(client, []byte(fmt.Sprintf(`{"type":"connected","clientId":"%s"}`, clientID))); err != nil {
		log.Error(fmt.Sprintf("Failed to send welcome message: %v", err))
		t.removeClient(clientID)
		return
	}

	// Stream messages to client.
	for {
		select {
		case <-r.Context().Done():
			t.removeClient(clientID)
			return
		case <-client.done:
			t.removeClient(clientID)
			return
		case msg := <-client.messages:
			if err := t.sendSSEMessage(client, msg); err != nil {
				log.Error(fmt.Sprintf("Failed to send SSE message: %v", err))
				t.removeClient(clientID)
				return
			}
		}
	}
}

// handleMessage handles incoming JSON-RPC messages via POST.
func (t *HTTPTransport) handleMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read request body.
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Parse message.
	msg, err := protocol.ParseMessage(body)
	if err != nil {
		t.sendErrorResponse(w, nil, protocol.ErrorCodeParseError, "Failed to parse message")
		return
	}

	// Handle based on message type.
	switch m := msg.(type) {
	case *protocol.Request:
		resp := t.handler.HandleRequest(r.Context(), m)
		t.sendJSONResponse(w, resp)
	case *protocol.Notification:
		if err := t.handler.HandleNotification(r.Context(), m); err != nil {
			log.Error(fmt.Sprintf("Notification handling error: %v", err))
		}
		// Notifications don't get responses.
		w.WriteHeader(http.StatusAccepted)
	default:
		t.sendErrorResponse(w, nil, protocol.ErrorCodeInvalidRequest, "Unknown message type")
	}
}

// handleHealth handles health check requests.
func (t *HTTPTransport) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "healthy",
		"clients": len(t.clients),
	}); err != nil {
		log.Error(fmt.Sprintf("Failed to encode health response: %v", err))
	}
}

// sendSSEMessage sends a message to an SSE client.
func (t *HTTPTransport) sendSSEMessage(client *sseClient, data []byte) error {
	if _, err := fmt.Fprintf(client.w, "data: %s\n\n", data); err != nil {
		return err
	}
	client.flusher.Flush()
	return nil
}

// sendJSONResponse sends a JSON response.
func (t *HTTPTransport) sendJSONResponse(w http.ResponseWriter, resp *protocol.Response) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Error(fmt.Sprintf("Failed to encode response: %v", err))
	}
}

// sendErrorResponse sends an error response.
func (t *HTTPTransport) sendErrorResponse(w http.ResponseWriter, id interface{}, code protocol.ErrorCode, message string) {
	resp := protocol.NewErrorResponse(id, code, message, nil)
	t.sendJSONResponse(w, resp)
}

// removeClient removes a client from the active clients map.
func (t *HTTPTransport) removeClient(clientID string) {
	t.clientsMu.Lock()
	defer t.clientsMu.Unlock()

	if client, ok := t.clients[clientID]; ok {
		close(client.done)
		close(client.messages)
		delete(t.clients, clientID)
		log.Info(fmt.Sprintf("SSE client disconnected: %s", clientID))
	}
}

// shutdown gracefully shuts down the HTTP server.
func (t *HTTPTransport) shutdown() error {
	log.Info("Shutting down MCP HTTP server...")

	// Close all SSE clients.
	t.clientsMu.Lock()
	for id := range t.clients {
		if client, ok := t.clients[id]; ok {
			close(client.done)
			close(client.messages)
		}
	}
	t.clients = make(map[string]*sseClient)
	t.clientsMu.Unlock()

	// Shutdown HTTP server with timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := t.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("HTTP server shutdown error: %w", err)
	}

	log.Info("MCP HTTP server stopped")
	return nil
}
