package server

import (
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

// TextDocumentDefinition handles the textDocument/definition request.
func (h *Handler) TextDocumentDefinition(context *glsp.Context, params *protocol.DefinitionParams) (any, error) {
	doc, exists := h.documents.Get(params.TextDocument.URI)
	if !exists {
		return nil, nil
	}

	// Get definition locations for the position.
	locations := h.getDefinitionLocations(doc, params.Position)

	if len(locations) == 0 {
		return nil, nil
	}

	// Return single location or array of locations.
	if len(locations) == 1 {
		return locations[0], nil
	}

	return locations, nil
}

// getDefinitionLocations returns definition locations for the given position.
func (h *Handler) getDefinitionLocations(doc *Document, pos protocol.Position) []protocol.Location {
	var locations []protocol.Location

	// TODO: Implement definition lookup.
	// This would involve:
	// 1. Parsing the YAML structure
	// 2. Identifying if cursor is on an import path, component reference, etc.
	// 3. Resolving the reference to actual file location
	// 4. Returning the location
	//
	// For now, return empty locations.

	return locations
}
