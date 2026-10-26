package server

import (
	"context"

	protocol "github.com/tliron/glsp/protocol_3_16"
	glspServer "github.com/tliron/glsp/server"

	"github.com/cloudposse/atmos/pkg/schema"
)

// Server represents the Atmos LSP server.
type Server struct {
	server      *glspServer.Server
	handler     *Handler
	atmosConfig *schema.AtmosConfiguration
	ctx         context.Context
}

// NewServer creates a new Atmos LSP server.
func NewServer(ctx context.Context, atmosConfig *schema.AtmosConfiguration) (*Server, error) {
	if atmosConfig == nil {
		atmosConfig = &schema.AtmosConfiguration{}
	}

	s := &Server{
		atmosConfig: atmosConfig,
		ctx:         ctx,
	}

	// Create handler.
	handler := NewHandler(s)
	s.handler = handler

	// Create GLSP server.
	glspHandler := protocol.Handler{
		Initialize:             handler.Initialize,
		Initialized:            handler.Initialized,
		Shutdown:               handler.Shutdown,
		SetTrace:               handler.SetTrace,
		TextDocumentDidOpen:    handler.TextDocumentDidOpen,
		TextDocumentDidChange:  handler.TextDocumentDidChange,
		TextDocumentDidSave:    handler.TextDocumentDidSave,
		TextDocumentDidClose:   handler.TextDocumentDidClose,
		TextDocumentCompletion: handler.TextDocumentCompletion,
		TextDocumentHover:      handler.TextDocumentHover,
		TextDocumentDefinition: handler.TextDocumentDefinition,
	}

	s.server = glspServer.NewServer(&glspHandler, "atmos-lsp", false)

	return s, nil
}

// RunStdio runs the LSP server using stdio transport.
func (s *Server) RunStdio() error {
	return s.server.RunStdio()
}

// RunTCP runs the LSP server using TCP transport.
func (s *Server) RunTCP(address string) error {
	return s.server.RunTCP(address)
}

// RunWebSocket runs the LSP server using WebSocket transport.
func (s *Server) RunWebSocket(address string) error {
	return s.server.RunWebSocket(address)
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown() error {
	// Cleanup resources if needed.
	return nil
}

// GetHandler returns the server's handler.
func (s *Server) GetHandler() *Handler {
	return s.handler
}
