package server

import (
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

// TextDocumentDidOpen handles the textDocument/didOpen notification.
func (h *Handler) TextDocumentDidOpen(context *glsp.Context, params *protocol.DidOpenTextDocumentParams) error {
	// Register the opened document.
	doc := h.documents.Open(
		params.TextDocument.URI,
		params.TextDocument.LanguageID,
		params.TextDocument.Version,
		params.TextDocument.Text,
	)

	// Validate the document and send diagnostics.
	go h.validateDocument(context, doc)

	return nil
}

// TextDocumentDidChange handles the textDocument/didChange notification.
func (h *Handler) TextDocumentDidChange(context *glsp.Context, params *protocol.DidChangeTextDocumentParams) error {
	// For full sync, we expect one content change with the full text.
	if len(params.ContentChanges) == 0 {
		return nil
	}

	// Get the full text from the first change (full sync mode).
	var text string
	if change, ok := params.ContentChanges[0].(protocol.TextDocumentContentChangeEventWhole); ok {
		text = change.Text
	} else {
		// If not a whole change, skip (shouldn't happen with TextDocumentSyncKindFull).
		return nil
	}

	// Update the document.
	doc := h.documents.Update(
		params.TextDocument.URI,
		params.TextDocument.Version,
		text,
	)

	if doc == nil {
		return nil
	}

	// Validate the updated document.
	go h.validateDocument(context, doc)

	return nil
}

// TextDocumentDidSave handles the textDocument/didSave notification.
func (h *Handler) TextDocumentDidSave(context *glsp.Context, params *protocol.DidSaveTextDocumentParams) error {
	// Re-validate on save.
	doc, exists := h.documents.Get(params.TextDocument.URI)
	if !exists {
		return nil
	}

	go h.validateDocument(context, doc)

	return nil
}

// TextDocumentDidClose handles the textDocument/didClose notification.
func (h *Handler) TextDocumentDidClose(context *glsp.Context, params *protocol.DidCloseTextDocumentParams) error {
	// Remove the document from the manager.
	h.documents.Close(params.TextDocument.URI)

	// Clear diagnostics for the closed document.
	context.Notify(protocol.ServerTextDocumentPublishDiagnostics, protocol.PublishDiagnosticsParams{
		URI:         params.TextDocument.URI,
		Diagnostics: []protocol.Diagnostic{},
	})

	return nil
}
