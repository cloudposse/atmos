package server

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

func TestTextDocumentDidOpen(t *testing.T) {
	tests := []struct {
		name       string
		params     *protocol.DidOpenTextDocumentParams
		wantErr    bool
		checkDoc   func(t *testing.T, dm *DocumentManager, uri protocol.DocumentUri)
	}{
		{
			name: "open new document",
			params: &protocol.DidOpenTextDocumentParams{
				TextDocument: protocol.TextDocumentItem{
					URI:        "file:///test.yaml",
					LanguageID: "yaml",
					Version:    1,
					Text:       "key: value",
				},
			},
			wantErr: false,
			checkDoc: func(t *testing.T, dm *DocumentManager, uri protocol.DocumentUri) {
				doc, exists := dm.Get(uri)
				assert.True(t, exists)
				assert.Equal(t, "key: value", doc.Text)
				assert.Equal(t, "yaml", doc.LanguageID)
				assert.Equal(t, int32(1), doc.Version)
			},
		},
		{
			name: "open empty document",
			params: &protocol.DidOpenTextDocumentParams{
				TextDocument: protocol.TextDocumentItem{
					URI:        "file:///empty.yaml",
					LanguageID: "yaml",
					Version:    1,
					Text:       "",
				},
			},
			wantErr: false,
			checkDoc: func(t *testing.T, dm *DocumentManager, uri protocol.DocumentUri) {
				doc, exists := dm.Get(uri)
				assert.True(t, exists)
				assert.Equal(t, "", doc.Text)
			},
		},
		{
			name: "open with complex content",
			params: &protocol.DidOpenTextDocumentParams{
				TextDocument: protocol.TextDocumentItem{
					URI:        "file:///complex.yaml",
					LanguageID: "yaml",
					Version:    1,
					Text:       "components:\n  terraform:\n    vpc:\n      vars:\n        cidr: 10.0.0.0/16",
				},
			},
			wantErr: false,
			checkDoc: func(t *testing.T, dm *DocumentManager, uri protocol.DocumentUri) {
				doc, exists := dm.Get(uri)
				assert.True(t, exists)
				assert.Contains(t, doc.Text, "components")
				assert.Contains(t, doc.Text, "terraform")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			server, err := NewServer(ctx, nil)
			require.NoError(t, err)

			handler := server.GetHandler()
			glspContext := &glsp.Context{}

			err = handler.TextDocumentDidOpen(glspContext, tt.params)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			// Give validation goroutine time to start.
			time.Sleep(10 * time.Millisecond)

			if tt.checkDoc != nil {
				tt.checkDoc(t, handler.documents, tt.params.TextDocument.URI)
			}
		})
	}
}

func TestTextDocumentDidChange(t *testing.T) {
	tests := []struct {
		name         string
		initialText  string
		changeParams *protocol.DidChangeTextDocumentParams
		wantErr      bool
		checkDoc     func(t *testing.T, dm *DocumentManager, uri protocol.DocumentUri)
	}{
		{
			name:        "change document content",
			initialText: "original content",
			changeParams: &protocol.DidChangeTextDocumentParams{
				TextDocument: protocol.VersionedTextDocumentIdentifier{
					TextDocumentIdentifier: protocol.TextDocumentIdentifier{
						URI: "file:///test.yaml",
					},
					Version: 2,
				},
				ContentChanges: []interface{}{
					protocol.TextDocumentContentChangeEventWhole{
						Text: "updated content",
					},
				},
			},
			wantErr: false,
			checkDoc: func(t *testing.T, dm *DocumentManager, uri protocol.DocumentUri) {
				doc, exists := dm.Get(uri)
				assert.True(t, exists)
				assert.Equal(t, "updated content", doc.Text)
				assert.Equal(t, int32(2), doc.Version)
			},
		},
		{
			name:        "change to empty content",
			initialText: "original content",
			changeParams: &protocol.DidChangeTextDocumentParams{
				TextDocument: protocol.VersionedTextDocumentIdentifier{
					TextDocumentIdentifier: protocol.TextDocumentIdentifier{
						URI: "file:///test.yaml",
					},
					Version: 2,
				},
				ContentChanges: []interface{}{
					protocol.TextDocumentContentChangeEventWhole{
						Text: "",
					},
				},
			},
			wantErr: false,
			checkDoc: func(t *testing.T, dm *DocumentManager, uri protocol.DocumentUri) {
				doc, exists := dm.Get(uri)
				assert.True(t, exists)
				assert.Equal(t, "", doc.Text)
			},
		},
		{
			name:        "change with no content changes",
			initialText: "original content",
			changeParams: &protocol.DidChangeTextDocumentParams{
				TextDocument: protocol.VersionedTextDocumentIdentifier{
					TextDocumentIdentifier: protocol.TextDocumentIdentifier{
						URI: "file:///test.yaml",
					},
					Version: 2,
				},
				ContentChanges: []interface{}{},
			},
			wantErr: false,
			checkDoc: func(t *testing.T, dm *DocumentManager, uri protocol.DocumentUri) {
				doc, exists := dm.Get(uri)
				assert.True(t, exists)
				// Content should remain unchanged.
				assert.Equal(t, "original content", doc.Text)
				assert.Equal(t, int32(1), doc.Version) // Version not updated
			},
		},
		{
			name:        "change non-existent document",
			initialText: "",
			changeParams: &protocol.DidChangeTextDocumentParams{
				TextDocument: protocol.VersionedTextDocumentIdentifier{
					TextDocumentIdentifier: protocol.TextDocumentIdentifier{
						URI: "file:///nonexistent.yaml",
					},
					Version: 2,
				},
				ContentChanges: []interface{}{
					protocol.TextDocumentContentChangeEventWhole{
						Text: "new content",
					},
				},
			},
			wantErr: false,
			checkDoc: func(t *testing.T, dm *DocumentManager, uri protocol.DocumentUri) {
				_, exists := dm.Get(uri)
				// Document should not be created.
				assert.False(t, exists)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			server, err := NewServer(ctx, nil)
			require.NoError(t, err)

			handler := server.GetHandler()
			glspContext := &glsp.Context{}

			// Open document first if initial text provided.
			if tt.initialText != "" {
				handler.documents.Open(
					tt.changeParams.TextDocument.URI,
					"yaml",
					1,
					tt.initialText,
				)
			}

			err = handler.TextDocumentDidChange(glspContext, tt.changeParams)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			// Give validation goroutine time to start.
			time.Sleep(10 * time.Millisecond)

			if tt.checkDoc != nil {
				tt.checkDoc(t, handler.documents, tt.changeParams.TextDocument.URI)
			}
		})
	}
}

func TestTextDocumentDidSave(t *testing.T) {
	tests := []struct {
		name      string
		docExists bool
		params    *protocol.DidSaveTextDocumentParams
		wantErr   bool
	}{
		{
			name:      "save existing document",
			docExists: true,
			params: &protocol.DidSaveTextDocumentParams{
				TextDocument: protocol.TextDocumentIdentifier{
					URI: "file:///test.yaml",
				},
			},
			wantErr: false,
		},
		{
			name:      "save non-existent document",
			docExists: false,
			params: &protocol.DidSaveTextDocumentParams{
				TextDocument: protocol.TextDocumentIdentifier{
					URI: "file:///nonexistent.yaml",
				},
			},
			wantErr: false, // Should not error, just no-op
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			server, err := NewServer(ctx, nil)
			require.NoError(t, err)

			handler := server.GetHandler()
			glspContext := &glsp.Context{}

			// Open document if needed.
			if tt.docExists {
				handler.documents.Open(tt.params.TextDocument.URI, "yaml", 1, "content")
			}

			err = handler.TextDocumentDidSave(glspContext, tt.params)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			// Give validation goroutine time to start.
			time.Sleep(10 * time.Millisecond)
		})
	}
}

func TestTextDocumentDidClose(t *testing.T) {
	tests := []struct {
		name        string
		docExists   bool
		params      *protocol.DidCloseTextDocumentParams
		wantErr     bool
		checkClosed func(t *testing.T, dm *DocumentManager, uri protocol.DocumentUri)
	}{
		{
			name:      "close existing document",
			docExists: true,
			params: &protocol.DidCloseTextDocumentParams{
				TextDocument: protocol.TextDocumentIdentifier{
					URI: "file:///test.yaml",
				},
			},
			wantErr: false,
			checkClosed: func(t *testing.T, dm *DocumentManager, uri protocol.DocumentUri) {
				_, exists := dm.Get(uri)
				assert.False(t, exists, "Document should be removed")
			},
		},
		{
			name:      "close non-existent document",
			docExists: false,
			params: &protocol.DidCloseTextDocumentParams{
				TextDocument: protocol.TextDocumentIdentifier{
					URI: "file:///nonexistent.yaml",
				},
			},
			wantErr: false, // Should not error
			checkClosed: func(t *testing.T, dm *DocumentManager, uri protocol.DocumentUri) {
				_, exists := dm.Get(uri)
				assert.False(t, exists)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			server, err := NewServer(ctx, nil)
			require.NoError(t, err)

			handler := server.GetHandler()
			glspContext := &glsp.Context{}

			// Open document if needed.
			if tt.docExists {
				handler.documents.Open(tt.params.TextDocument.URI, "yaml", 1, "content")
			}

			err = handler.TextDocumentDidClose(glspContext, tt.params)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			if tt.checkClosed != nil {
				tt.checkClosed(t, handler.documents, tt.params.TextDocument.URI)
			}
		})
	}
}

func TestTextDocumentLifecycle(t *testing.T) {
	// Test complete document lifecycle: open → change → save → close.
	ctx := context.Background()
	server, err := NewServer(ctx, nil)
	require.NoError(t, err)

	handler := server.GetHandler()
	glspContext := &glsp.Context{}
	uri := protocol.DocumentUri("file:///test.yaml")

	// 1. Open document.
	err = handler.TextDocumentDidOpen(glspContext, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:        uri,
			LanguageID: "yaml",
			Version:    1,
			Text:       "initial content",
		},
	})
	require.NoError(t, err)

	doc, exists := handler.documents.Get(uri)
	assert.True(t, exists)
	assert.Equal(t, "initial content", doc.Text)

	// 2. Change document.
	err = handler.TextDocumentDidChange(glspContext, &protocol.DidChangeTextDocumentParams{
		TextDocument: protocol.VersionedTextDocumentIdentifier{
			TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: uri},
			Version:                2,
		},
		ContentChanges: []interface{}{
			protocol.TextDocumentContentChangeEventWhole{Text: "updated content"},
		},
	})
	require.NoError(t, err)

	doc, exists = handler.documents.Get(uri)
	assert.True(t, exists)
	assert.Equal(t, "updated content", doc.Text)
	assert.Equal(t, int32(2), doc.Version)

	// 3. Save document.
	err = handler.TextDocumentDidSave(glspContext, &protocol.DidSaveTextDocumentParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
	})
	require.NoError(t, err)

	// Document should still exist after save.
	_, exists = handler.documents.Get(uri)
	assert.True(t, exists)

	// 4. Close document.
	err = handler.TextDocumentDidClose(glspContext, &protocol.DidCloseTextDocumentParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
	})
	require.NoError(t, err)

	// Document should be removed after close.
	_, exists = handler.documents.Get(uri)
	assert.False(t, exists)
}

func TestTextDocumentConcurrentOperations(t *testing.T) {
	// Test concurrent document operations don't cause race conditions.
	ctx := context.Background()
	server, err := NewServer(ctx, nil)
	require.NoError(t, err)

	handler := server.GetHandler()
	glspContext := &glsp.Context{}

	// Open initial document.
	uri := protocol.DocumentUri("file:///test.yaml")
	err = handler.TextDocumentDidOpen(glspContext, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:        uri,
			LanguageID: "yaml",
			Version:    1,
			Text:       "initial",
		},
	})
	require.NoError(t, err)

	// Concurrent changes.
	var wg sync.WaitGroup
	wg.Add(100)

	for i := 0; i < 100; i++ {
		go func(version int) {
			defer wg.Done()
			_ = handler.TextDocumentDidChange(glspContext, &protocol.DidChangeTextDocumentParams{
				TextDocument: protocol.VersionedTextDocumentIdentifier{
					TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: uri},
					Version:                int32(version),
				},
				ContentChanges: []interface{}{
					protocol.TextDocumentContentChangeEventWhole{Text: "updated"},
				},
			})
		}(i + 2)
	}

	wg.Wait()

	// Document should still exist and be valid.
	doc, exists := handler.documents.Get(uri)
	assert.True(t, exists)
	assert.NotNil(t, doc)
}
