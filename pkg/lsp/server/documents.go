package server

import (
	"sync"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

// Document represents an open document in the LSP server.
type Document struct {
	URI        protocol.DocumentUri
	LanguageID string
	Version    int32
	Text       string
}

// DocumentManager manages open documents.
type DocumentManager struct {
	documents map[protocol.DocumentUri]*Document
	mu        sync.RWMutex
}

// NewDocumentManager creates a new document manager.
func NewDocumentManager() *DocumentManager {
	return &DocumentManager{
		documents: make(map[protocol.DocumentUri]*Document),
	}
}

// Open adds a new document to the manager.
func (dm *DocumentManager) Open(uri protocol.DocumentUri, languageID string, version int32, text string) *Document {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	doc := &Document{
		URI:        uri,
		LanguageID: languageID,
		Version:    version,
		Text:       text,
	}

	dm.documents[uri] = doc
	return doc
}

// Update updates an existing document's content.
func (dm *DocumentManager) Update(uri protocol.DocumentUri, version int32, text string) *Document {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	doc, exists := dm.documents[uri]
	if !exists {
		// Document not open, ignore.
		return nil
	}

	doc.Version = version
	doc.Text = text

	return doc
}

// Close removes a document from the manager.
func (dm *DocumentManager) Close(uri protocol.DocumentUri) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	delete(dm.documents, uri)
}

// Get retrieves a document by URI.
func (dm *DocumentManager) Get(uri protocol.DocumentUri) (*Document, bool) {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	doc, exists := dm.documents[uri]
	return doc, exists
}

// GetAll returns all open documents.
func (dm *DocumentManager) GetAll() []*Document {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	docs := make([]*Document, 0, len(dm.documents))
	for _, doc := range dm.documents {
		docs = append(docs, doc)
	}

	return docs
}

// Count returns the number of open documents.
func (dm *DocumentManager) Count() int {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	return len(dm.documents)
}

// Set sets a document directly (used for testing).
func (dm *DocumentManager) Set(uri protocol.DocumentUri, doc *Document) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	dm.documents[uri] = doc
}
