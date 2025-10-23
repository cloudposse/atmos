package transport

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/mcp/protocol"
)

// StdioTransport implements MCP transport over stdin/stdout.
type StdioTransport struct {
	stdin  io.Reader
	stdout io.Writer
	mu     sync.Mutex
}

// NewStdioTransport creates a new stdio transport.
func NewStdioTransport() *StdioTransport {
	return &StdioTransport{
		stdin:  os.Stdin,
		stdout: os.Stdout,
	}
}

// NewStdioTransportWithStreams creates a new stdio transport with custom streams.
// Useful for testing.
func NewStdioTransportWithStreams(stdin io.Reader, stdout io.Writer) *StdioTransport {
	return &StdioTransport{
		stdin:  stdin,
		stdout: stdout,
	}
}

// Serve starts the transport and processes messages until context is cancelled.
func (t *StdioTransport) Serve(ctx context.Context, handler protocol.Handler) error {
	scanner := bufio.NewScanner(t.stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // 64KB initial, 1MB max

	for scanner.Scan() {
		// Check if context is cancelled.
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		t.processMessage(ctx, handler, line)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scanner error: %w", err)
	}

	return nil
}

// processMessage parses and handles a single message.
func (t *StdioTransport) processMessage(ctx context.Context, handler protocol.Handler, line []byte) {
	// Parse message.
	msg, err := protocol.ParseMessage(line)
	if err != nil {
		t.handleParseError(err)
		return
	}

	// Handle based on message type.
	switch m := msg.(type) {
	case *protocol.Request:
		t.handleRequest(ctx, handler, m)
	case *protocol.Notification:
		t.handleNotification(ctx, handler, m)
	default:
		log.Warn(fmt.Sprintf("Unknown message type: %T", msg))
	}
}

// handleParseError handles errors from parsing messages.
func (t *StdioTransport) handleParseError(err error) {
	var mcpErr *protocol.Error
	if errors.As(err, &mcpErr) {
		errResp := protocol.NewErrorResponse(nil, mcpErr.Code, mcpErr.Message, mcpErr.Data)
		if writeErr := t.WriteResponse(errResp); writeErr != nil {
			log.Error(fmt.Sprintf("Failed to write error response: %v", writeErr))
		}
	}
}

// handleRequest processes a request and sends the response.
func (t *StdioTransport) handleRequest(ctx context.Context, handler protocol.Handler, req *protocol.Request) {
	resp := handler.HandleRequest(ctx, req)
	if err := t.WriteResponse(resp); err != nil {
		log.Error(fmt.Sprintf("Failed to write response: %v", err))
	}
}

// handleNotification processes a notification (no response).
func (t *StdioTransport) handleNotification(ctx context.Context, handler protocol.Handler, notif *protocol.Notification) {
	if err := handler.HandleNotification(ctx, notif); err != nil {
		log.Error(fmt.Sprintf("Failed to handle notification: %v", err))
	}
}

// WriteResponse writes a JSON-RPC response to stdout.
func (t *StdioTransport) WriteResponse(resp *protocol.Response) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	data, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("failed to marshal response: %w", err)
	}

	// Write message with newline delimiter.
	if _, err := t.stdout.Write(data); err != nil {
		return fmt.Errorf("failed to write response: %w", err)
	}

	if _, err := t.stdout.Write([]byte("\n")); err != nil {
		return fmt.Errorf("failed to write newline: %w", err)
	}

	return nil
}

// WriteNotification writes a JSON-RPC notification to stdout.
func (t *StdioTransport) WriteNotification(notif *protocol.Notification) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	data, err := json.Marshal(notif)
	if err != nil {
		return fmt.Errorf("failed to marshal notification: %w", err)
	}

	// Write message with newline delimiter.
	if _, err := t.stdout.Write(data); err != nil {
		return fmt.Errorf("failed to write notification: %w", err)
	}

	if _, err := t.stdout.Write([]byte("\n")); err != nil {
		return fmt.Errorf("failed to write newline: %w", err)
	}

	return nil
}

// SendLogMessage sends a logging message to the client.
func (t *StdioTransport) SendLogMessage(level protocol.LoggingLevel, logger string, data interface{}) error {
	notif, err := protocol.NewNotification(protocol.MethodLoggingMessage, &protocol.LoggingMessageParams{
		Level:  level,
		Logger: logger,
		Data:   data,
	})
	if err != nil {
		return fmt.Errorf("failed to create log notification: %w", err)
	}

	return t.WriteNotification(notif)
}
