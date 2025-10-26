package client

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/lsp"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Client represents an LSP client that communicates with an LSP server.
type Client struct {
	cmd            *exec.Cmd
	stdin          io.WriteCloser
	stdout         io.ReadCloser
	stderr         io.ReadCloser
	diagnostics    map[string][]lsp.Diagnostic // URI -> diagnostics
	diagnosticsMu  sync.RWMutex
	messageID      atomic.Int64
	pendingCalls   map[int64]chan *jsonRPCResponse
	pendingCallsMu sync.RWMutex
	rootURI        string
	name           string            // Server name (for logging/identification)
	config         *schema.LSPServer // Server configuration
	initialized    bool
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
}

// jsonRPCRequest represents a JSON-RPC 2.0 request.
type jsonRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int64       `json:"id,omitempty"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// jsonRPCResponse represents a JSON-RPC 2.0 response.
type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

// jsonRPCError represents a JSON-RPC 2.0 error.
type jsonRPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// jsonRPCNotification represents a JSON-RPC 2.0 notification (no ID).
type jsonRPCNotification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// NewClient creates a new LSP client for the specified server configuration.
func NewClient(ctx context.Context, name string, config *schema.LSPServer, rootURI string) (*Client, error) {
	if config == nil {
		return nil, fmt.Errorf("%w: LSP server config is nil", errUtils.ErrAIInvalidConfiguration)
	}

	if config.Command == "" {
		return nil, fmt.Errorf("%w: LSP server command is empty", errUtils.ErrAIInvalidConfiguration)
	}

	// Create context with cancel
	clientCtx, cancel := context.WithCancel(ctx)

	cmd := exec.CommandContext(clientCtx, config.Command, config.Args...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	client := &Client{
		cmd:          cmd,
		stdin:        stdin,
		stdout:       stdout,
		stderr:       stderr,
		diagnostics:  make(map[string][]lsp.Diagnostic),
		pendingCalls: make(map[int64]chan *jsonRPCResponse),
		rootURI:      rootURI,
		name:         name,
		config:       config,
		ctx:          clientCtx,
		cancel:       cancel,
	}

	// Start LSP server process
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to start LSP server %s: %w", name, err)
	}

	// Start reading stdout and stderr
	client.wg.Add(2)
	go client.readStdout()
	go client.readStderr()

	return client, nil
}

// Initialize sends the initialize request to the LSP server.
func (c *Client) Initialize() error {
	params := lsp.InitializeParams{
		ProcessID: -1, // Parent process ID (not applicable)
		RootURI:   c.rootURI,
		Capabilities: lsp.ClientCapabilities{
			TextDocument: lsp.TextDocumentClientCapabilities{
				PublishDiagnostics: lsp.PublishDiagnosticsCapabilities{
					RelatedInformation: true,
				},
			},
		},
		InitializationOptions: c.config.InitializationOptions,
	}

	var result lsp.InitializeResult
	if err := c.call("initialize", params, &result); err != nil {
		return fmt.Errorf("initialize request failed: %w", err)
	}

	// Send initialized notification
	if err := c.notify("initialized", struct{}{}); err != nil {
		return fmt.Errorf("initialized notification failed: %w", err)
	}

	c.initialized = true
	return nil
}

// OpenDocument notifies the server that a document was opened.
func (c *Client) OpenDocument(uri, languageID, text string) error {
	if !c.initialized {
		return fmt.Errorf("%w: client not initialized", errUtils.ErrAINotInitialized)
	}

	params := lsp.DidOpenTextDocumentParams{
		TextDocument: lsp.TextDocumentItem{
			URI:        uri,
			LanguageID: languageID,
			Version:    1,
			Text:       text,
		},
	}

	return c.notify("textDocument/didOpen", params)
}

// CloseDocument notifies the server that a document was closed.
func (c *Client) CloseDocument(uri string) error {
	if !c.initialized {
		return fmt.Errorf("%w: client not initialized", errUtils.ErrAINotInitialized)
	}

	params := lsp.DidCloseTextDocumentParams{
		TextDocument: lsp.TextDocumentIdentifier{
			URI: uri,
		},
	}

	return c.notify("textDocument/didClose", params)
}

// GetDiagnostics returns diagnostics for the specified URI.
func (c *Client) GetDiagnostics(uri string) []lsp.Diagnostic {
	c.diagnosticsMu.RLock()
	defer c.diagnosticsMu.RUnlock()

	diagnostics := c.diagnostics[uri]
	if diagnostics == nil {
		return []lsp.Diagnostic{}
	}

	// Return copy to avoid data races
	result := make([]lsp.Diagnostic, len(diagnostics))
	copy(result, diagnostics)
	return result
}

// GetAllDiagnostics returns all diagnostics from all documents.
func (c *Client) GetAllDiagnostics() map[string][]lsp.Diagnostic {
	c.diagnosticsMu.RLock()
	defer c.diagnosticsMu.RUnlock()

	// Return copy to avoid data races
	result := make(map[string][]lsp.Diagnostic)
	for uri, diagnostics := range c.diagnostics {
		copied := make([]lsp.Diagnostic, len(diagnostics))
		copy(copied, diagnostics)
		result[uri] = copied
	}
	return result
}

// Close shuts down the LSP client and server.
func (c *Client) Close() error {
	// Send shutdown request
	if c.initialized {
		_ = c.call("shutdown", nil, nil)
		_ = c.notify("exit", nil)
	}

	// Cancel context to stop goroutines
	c.cancel()

	// Close pipes
	_ = c.stdin.Close()
	_ = c.stdout.Close()
	_ = c.stderr.Close()

	// Wait for goroutines
	c.wg.Wait()

	// Kill process if still running
	if c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}

	return nil
}

// call sends a JSON-RPC request and waits for response.
func (c *Client) call(method string, params interface{}, result interface{}) error {
	id := c.messageID.Add(1)

	// Create response channel
	responseChan := make(chan *jsonRPCResponse, 1)
	c.pendingCallsMu.Lock()
	c.pendingCalls[id] = responseChan
	c.pendingCallsMu.Unlock()

	// Clean up when done
	defer func() {
		c.pendingCallsMu.Lock()
		delete(c.pendingCalls, id)
		c.pendingCallsMu.Unlock()
		close(responseChan)
	}()

	request := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	if err := c.sendMessage(request); err != nil {
		return err
	}

	// Wait for response
	select {
	case <-c.ctx.Done():
		return c.ctx.Err()
	case response := <-responseChan:
		if response.Error != nil {
			return fmt.Errorf("RPC error %d: %s", response.Error.Code, response.Error.Message)
		}
		if result != nil && response.Result != nil {
			if err := json.Unmarshal(response.Result, result); err != nil {
				return fmt.Errorf("failed to unmarshal result: %w", err)
			}
		}
		return nil
	}
}

// notify sends a JSON-RPC notification (no response expected).
func (c *Client) notify(method string, params interface{}) error {
	notification := jsonRPCNotification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  mustMarshalJSON(params),
	}

	return c.sendMessage(notification)
}

// sendMessage sends a JSON-RPC message using LSP framing (Content-Length header).
func (c *Client) sendMessage(message interface{}) error {
	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	_, err = c.stdin.Write([]byte(header))
	if err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}

	_, err = c.stdin.Write(data)
	if err != nil {
		return fmt.Errorf("failed to write body: %w", err)
	}

	return nil
}

// readStdout reads and processes messages from the LSP server's stdout.
func (c *Client) readStdout() {
	defer c.wg.Done()

	reader := bufio.NewReader(c.stdout)
	for {
		select {
		case <-c.ctx.Done():
			return
		default:
			// Read Content-Length header
			contentLength, err := c.readContentLength(reader)
			if err != nil {
				if err != io.EOF {
					// Log error but continue
				}
				return
			}

			// Read message body
			body := make([]byte, contentLength)
			if _, err := io.ReadFull(reader, body); err != nil {
				if err != io.EOF {
					// Log error but continue
				}
				return
			}

			// Process message
			c.processMessage(body)
		}
	}
}

// readStderr reads and logs stderr output from the LSP server.
func (c *Client) readStderr() {
	defer c.wg.Done()

	scanner := bufio.NewScanner(c.stderr)
	for scanner.Scan() {
		select {
		case <-c.ctx.Done():
			return
		default:
			// Log stderr output (for debugging)
			// Can be enabled via debug mode in the future
			_ = scanner.Text()
		}
	}
}

// readContentLength reads the Content-Length header from LSP message.
func (c *Client) readContentLength(reader *bufio.Reader) (int, error) {
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return 0, err
		}

		line = strings.TrimSpace(line)
		if line == "" {
			return 0, fmt.Errorf("no Content-Length header found")
		}

		if strings.HasPrefix(line, "Content-Length:") {
			lengthStr := strings.TrimSpace(strings.TrimPrefix(line, "Content-Length:"))
			contentLength, err := strconv.Atoi(lengthStr)
			if err != nil {
				return 0, fmt.Errorf("invalid Content-Length: %w", err)
			}

			// Read the blank line after headers
			_, err = reader.ReadString('\n')
			if err != nil {
				return 0, err
			}

			return contentLength, nil
		}
	}
}

// processMessage processes incoming JSON-RPC messages.
func (c *Client) processMessage(data []byte) {
	// Try to parse as notification first (has method, no id)
	var notification jsonRPCNotification
	if err := json.Unmarshal(data, &notification); err == nil && notification.Method != "" {
		c.handleNotification(&notification)
		return
	}

	// Try to parse as response (has id)
	var response jsonRPCResponse
	if err := json.Unmarshal(data, &response); err == nil && response.ID > 0 {
		c.handleResponse(&response)
		return
	}

	// Unknown message type
}

// handleNotification handles JSON-RPC notifications from the server.
func (c *Client) handleNotification(notification *jsonRPCNotification) {
	switch notification.Method {
	case "textDocument/publishDiagnostics":
		var params lsp.PublishDiagnosticsParams
		if err := json.Unmarshal(notification.Params, &params); err != nil {
			return
		}
		c.updateDiagnostics(params.URI, params.Diagnostics)
	}
}

// handleResponse handles JSON-RPC responses from the server.
func (c *Client) handleResponse(response *jsonRPCResponse) {
	c.pendingCallsMu.RLock()
	responseChan, exists := c.pendingCalls[response.ID]
	c.pendingCallsMu.RUnlock()

	if exists && responseChan != nil {
		select {
		case responseChan <- response:
		case <-c.ctx.Done():
		}
	}
}

// updateDiagnostics updates diagnostics for a document.
func (c *Client) updateDiagnostics(uri string, diagnostics []lsp.Diagnostic) {
	c.diagnosticsMu.Lock()
	defer c.diagnosticsMu.Unlock()
	c.diagnostics[uri] = diagnostics
}

// mustMarshalJSON marshals value to JSON RawMessage (panics on error).
func mustMarshalJSON(v interface{}) json.RawMessage {
	if v == nil {
		return nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}

// extractConfig extracts LSP configuration from AtmosConfiguration.
func extractConfig(atmosConfig *schema.AtmosConfiguration) *schema.LSPSettings {
	return &atmosConfig.Settings.LSP
}
