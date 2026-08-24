package server

import (
	"sync"

	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

// Handler implements LSP protocol handlers.
type Handler struct {
	server      *Server
	documents   *DocumentManager
	initialized bool
	mu          sync.RWMutex
}

// NewHandler creates a new LSP handler.
func NewHandler(server *Server) *Handler {
	return &Handler{
		server:    server,
		documents: NewDocumentManager(),
	}
}

// Initialize handles the initialize request.
func (h *Handler) Initialize(context *glsp.Context, params *protocol.InitializeParams) (any, error) {
	capabilities := h.createServerCapabilities()

	version := "0.1.0"
	return protocol.InitializeResult{
		Capabilities: capabilities,
		ServerInfo: &protocol.InitializeResultServerInfo{
			Name:    "atmos-lsp",
			Version: &version,
		},
	}, nil
}

// Initialized handles the initialized notification.
func (h *Handler) Initialized(context *glsp.Context, params *protocol.InitializedParams) error {
	h.mu.Lock()
	h.initialized = true
	h.mu.Unlock()
	return nil
}

// Shutdown handles the shutdown request.
func (h *Handler) Shutdown(context *glsp.Context) error {
	h.mu.Lock()
	h.initialized = false
	h.mu.Unlock()
	return nil
}

// SetTrace handles the setTrace notification.
func (h *Handler) SetTrace(context *glsp.Context, params *protocol.SetTraceParams) error {
	// Log trace value changes if needed.
	return nil
}

// createServerCapabilities returns the server's capabilities.
func (h *Handler) createServerCapabilities() protocol.ServerCapabilities {
	trueVal := true
	falseVal := false
	syncKind := protocol.TextDocumentSyncKindFull

	return protocol.ServerCapabilities{
		// Text document synchronization.
		TextDocumentSync: protocol.TextDocumentSyncOptions{
			OpenClose: &trueVal,
			Change:    &syncKind,
			Save: &protocol.SaveOptions{
				IncludeText: &falseVal,
			},
		},
		// Completion support.
		CompletionProvider: &protocol.CompletionOptions{
			TriggerCharacters: []string{".", ":", "/"},
			ResolveProvider:   &falseVal,
		},
		// Hover support.
		HoverProvider: &trueVal,
		// Definition support (stub implementation - returns empty).
		DefinitionProvider: &trueVal,
		// Note: DocumentSymbolProvider and FileOperations removed until implemented.
		// See docs/prd/atmos-lsp-verification-report.md for details.
	}
}

// IsInitialized returns whether the server is initialized.
func (h *Handler) IsInitialized() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.initialized
}
