package lsp

import (
	"bytes"
	"strconv"
)

// Protocol types for LSP (Language Server Protocol) communication.
// Based on the Language Server Protocol Specification.

// Position represents a position in a text document.
type Position struct {
	Line      int `json:"line"`      // Line position in a document (zero-based)
	Character int `json:"character"` // Character offset on a line (zero-based)
}

// Range represents a range in a text document.
type Range struct {
	Start Position `json:"start"` // Start position (inclusive)
	End   Position `json:"end"`   // End position (exclusive)
}

// Location represents a location inside a resource.
type Location struct {
	URI   string `json:"uri"`   // Resource URI
	Range Range  `json:"range"` // Range in the resource
}

// DiagnosticSeverity represents the severity of a diagnostic.
type DiagnosticSeverity int

const (
	DiagnosticSeverityError       DiagnosticSeverity = 1
	DiagnosticSeverityWarning     DiagnosticSeverity = 2
	DiagnosticSeverityInformation DiagnosticSeverity = 3
	DiagnosticSeverityHint        DiagnosticSeverity = 4
)

// Diagnostic represents a diagnostic (error, warning, etc.).
type Diagnostic struct {
	Range              Range              `json:"range"`              // Range of the diagnostic
	Severity           DiagnosticSeverity `json:"severity"`           // Severity level
	Code               interface{}        `json:"code,omitempty"`     // Diagnostic code
	Source             string             `json:"source,omitempty"`   // Source (e.g., "yaml-language-server")
	Message            string             `json:"message"`            // Diagnostic message
	RelatedInformation []DiagnosticInfo   `json:"relatedInformation"` // Related diagnostic information
}

// DiagnosticInfo represents related information for a diagnostic.
type DiagnosticInfo struct {
	Location Location `json:"location"` // Location of related information
	Message  string   `json:"message"`  // Related message
}

// PublishDiagnosticsParams represents params for textDocument/publishDiagnostics notification.
type PublishDiagnosticsParams struct {
	URI         string       `json:"uri"`         // Document URI
	Diagnostics []Diagnostic `json:"diagnostics"` // Diagnostics array
}

// TextDocumentItem represents a text document.
type TextDocumentItem struct {
	URI        string `json:"uri"`        // Document URI
	LanguageID string `json:"languageId"` // Language identifier
	Version    int    `json:"version"`    // Document version
	Text       string `json:"text"`       // Document content
}

// TextDocumentIdentifier identifies a text document.
type TextDocumentIdentifier struct {
	URI string `json:"uri"` // Document URI
}

// VersionedTextDocumentIdentifier identifies a versioned text document.
type VersionedTextDocumentIdentifier struct {
	TextDocumentIdentifier
	Version int `json:"version"` // Document version
}

// DidOpenTextDocumentParams represents params for textDocument/didOpen notification.
type DidOpenTextDocumentParams struct {
	TextDocument TextDocumentItem `json:"textDocument"` // The document that was opened
}

// DidChangeTextDocumentParams represents params for textDocument/didChange notification.
type DidChangeTextDocumentParams struct {
	TextDocument   VersionedTextDocumentIdentifier  `json:"textDocument"`   // The document that changed
	ContentChanges []TextDocumentContentChangeEvent `json:"contentChanges"` // Content changes
}

// TextDocumentContentChangeEvent represents a change to a text document.
type TextDocumentContentChangeEvent struct {
	Range       *Range `json:"range,omitempty"`       // Optional range (if nil, text is full content)
	RangeLength int    `json:"rangeLength,omitempty"` // Optional length of range
	Text        string `json:"text"`                  // New text
}

// DidCloseTextDocumentParams represents params for textDocument/didClose notification.
type DidCloseTextDocumentParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"` // The document that was closed
}

// InitializeParams represents initialization parameters.
type InitializeParams struct {
	ProcessID             int                `json:"processId"`             // Process ID of parent
	RootURI               string             `json:"rootUri"`               // Root URI of workspace
	InitializationOptions interface{}        `json:"initializationOptions"` // Custom initialization options
	Capabilities          ClientCapabilities `json:"capabilities"`          // Client capabilities
}

// ClientCapabilities represents client capabilities.
type ClientCapabilities struct {
	TextDocument TextDocumentClientCapabilities `json:"textDocument"` // Text document capabilities
}

// TextDocumentClientCapabilities represents text document capabilities.
type TextDocumentClientCapabilities struct {
	PublishDiagnostics PublishDiagnosticsCapabilities `json:"publishDiagnostics"` // Diagnostics capabilities
}

// PublishDiagnosticsCapabilities represents diagnostics capabilities.
type PublishDiagnosticsCapabilities struct {
	RelatedInformation bool `json:"relatedInformation"` // Support for related information
}

// InitializeResult represents the result of initialization.
type InitializeResult struct {
	Capabilities ServerCapabilities `json:"capabilities"` // Server capabilities
}

// ServerCapabilities represents server capabilities.
type ServerCapabilities struct {
	TextDocumentSync interface{} `json:"textDocumentSync"` // Text document sync kind
}

// MarshalText implements encoding.TextMarshaler.
func (d DiagnosticSeverity) MarshalText() ([]byte, error) {
	return []byte(strconv.Itoa(int(d))), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (d *DiagnosticSeverity) UnmarshalText(text []byte) error {
	val, err := strconv.Atoi(string(bytes.TrimSpace(text)))
	if err != nil {
		return err
	}
	*d = DiagnosticSeverity(val)
	return nil
}
