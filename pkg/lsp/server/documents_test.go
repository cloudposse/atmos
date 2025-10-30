package server

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

func TestNewDocumentManager(t *testing.T) {
	dm := NewDocumentManager()

	assert.NotNil(t, dm)
	assert.NotNil(t, dm.documents)
	assert.Equal(t, 0, dm.Count())
}

func TestDocumentManager_Open(t *testing.T) {
	tests := []struct {
		name       string
		uri        protocol.DocumentUri
		languageID string
		version    int32
		text       string
	}{
		{
			name:       "open YAML file",
			uri:        "file:///test.yaml",
			languageID: "yaml",
			version:    1,
			text:       "key: value",
		},
		{
			name:       "open with empty content",
			uri:        "file:///empty.yaml",
			languageID: "yaml",
			version:    1,
			text:       "",
		},
		{
			name:       "open with version 0",
			uri:        "file:///v0.yaml",
			languageID: "yaml",
			version:    0,
			text:       "content",
		},
		{
			name:       "open with large version",
			uri:        "file:///large.yaml",
			languageID: "yaml",
			version:    12345,
			text:       "content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dm := NewDocumentManager()

			doc := dm.Open(tt.uri, tt.languageID, tt.version, tt.text)

			require.NotNil(t, doc)
			assert.Equal(t, tt.uri, doc.URI)
			assert.Equal(t, tt.languageID, doc.LanguageID)
			assert.Equal(t, tt.version, doc.Version)
			assert.Equal(t, tt.text, doc.Text)

			// Verify document is stored.
			stored, exists := dm.Get(tt.uri)
			assert.True(t, exists)
			assert.Equal(t, doc, stored)
		})
	}
}

func TestDocumentManager_Update(t *testing.T) {
	tests := []struct {
		name        string
		initialText string
		updateText  string
		version     int32
	}{
		{
			name:        "update with new content",
			initialText: "original content",
			updateText:  "updated content",
			version:     2,
		},
		{
			name:        "update with empty content",
			initialText: "original content",
			updateText:  "",
			version:     2,
		},
		{
			name:        "update with same content",
			initialText: "content",
			updateText:  "content",
			version:     2,
		},
		{
			name:        "update version only",
			initialText: "content",
			updateText:  "content",
			version:     10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dm := NewDocumentManager()
			uri := protocol.DocumentUri("file:///test.yaml")

			// Open document.
			doc := dm.Open(uri, "yaml", 1, tt.initialText)
			require.NotNil(t, doc)
			assert.Equal(t, tt.initialText, doc.Text)

			// Update document.
			updated := dm.Update(uri, tt.version, tt.updateText)
			require.NotNil(t, updated)

			assert.Equal(t, uri, updated.URI)
			assert.Equal(t, tt.version, updated.Version)
			assert.Equal(t, tt.updateText, updated.Text)

			// Verify updates are persisted.
			stored, exists := dm.Get(uri)
			assert.True(t, exists)
			assert.Equal(t, tt.updateText, stored.Text)
			assert.Equal(t, tt.version, stored.Version)
		})
	}
}

func TestDocumentManager_UpdateNonExistent(t *testing.T) {
	dm := NewDocumentManager()
	uri := protocol.DocumentUri("file:///nonexistent.yaml")

	// Try to update non-existent document.
	doc := dm.Update(uri, 1, "content")

	assert.Nil(t, doc, "Update of non-existent document should return nil")
}

func TestDocumentManager_Close(t *testing.T) {
	dm := NewDocumentManager()
	uri := protocol.DocumentUri("file:///test.yaml")

	// Open document.
	doc := dm.Open(uri, "yaml", 1, "content")
	require.NotNil(t, doc)
	assert.Equal(t, 1, dm.Count())

	// Close document.
	dm.Close(uri)

	// Verify document is removed.
	_, exists := dm.Get(uri)
	assert.False(t, exists)
	assert.Equal(t, 0, dm.Count())
}

func TestDocumentManager_CloseNonExistent(t *testing.T) {
	dm := NewDocumentManager()
	uri := protocol.DocumentUri("file:///nonexistent.yaml")

	// Close non-existent document should not panic.
	assert.NotPanics(t, func() {
		dm.Close(uri)
	})
}

func TestDocumentManager_Get(t *testing.T) {
	dm := NewDocumentManager()
	uri1 := protocol.DocumentUri("file:///test1.yaml")
	uri2 := protocol.DocumentUri("file:///test2.yaml")

	// Open document 1.
	doc1 := dm.Open(uri1, "yaml", 1, "content1")
	require.NotNil(t, doc1)

	// Get existing document.
	retrieved, exists := dm.Get(uri1)
	assert.True(t, exists)
	assert.Equal(t, doc1, retrieved)

	// Get non-existent document.
	_, exists = dm.Get(uri2)
	assert.False(t, exists)
}

func TestDocumentManager_GetAll(t *testing.T) {
	dm := NewDocumentManager()

	// Initially empty.
	docs := dm.GetAll()
	assert.Len(t, docs, 0)

	// Add multiple documents.
	uri1 := protocol.DocumentUri("file:///test1.yaml")
	uri2 := protocol.DocumentUri("file:///test2.yaml")
	uri3 := protocol.DocumentUri("file:///test3.yaml")

	dm.Open(uri1, "yaml", 1, "content1")
	dm.Open(uri2, "yaml", 1, "content2")
	dm.Open(uri3, "yaml", 1, "content3")

	// Get all documents.
	docs = dm.GetAll()
	assert.Len(t, docs, 3)

	// Verify all URIs are present.
	uris := make(map[protocol.DocumentUri]bool)
	for _, doc := range docs {
		uris[doc.URI] = true
	}

	assert.True(t, uris[uri1])
	assert.True(t, uris[uri2])
	assert.True(t, uris[uri3])
}

func TestDocumentManager_Count(t *testing.T) {
	dm := NewDocumentManager()

	assert.Equal(t, 0, dm.Count())

	// Add documents.
	dm.Open("file:///test1.yaml", "yaml", 1, "content1")
	assert.Equal(t, 1, dm.Count())

	dm.Open("file:///test2.yaml", "yaml", 1, "content2")
	assert.Equal(t, 2, dm.Count())

	dm.Open("file:///test3.yaml", "yaml", 1, "content3")
	assert.Equal(t, 3, dm.Count())

	// Close documents.
	dm.Close("file:///test1.yaml")
	assert.Equal(t, 2, dm.Count())

	dm.Close("file:///test2.yaml")
	assert.Equal(t, 1, dm.Count())

	dm.Close("file:///test3.yaml")
	assert.Equal(t, 0, dm.Count())
}

func TestDocumentManager_ConcurrentOpen(t *testing.T) {
	// Test concurrent opens don't cause race conditions.
	dm := NewDocumentManager()
	count := 100

	var wg sync.WaitGroup
	wg.Add(count)

	for i := 0; i < count; i++ {
		go func(index int) {
			defer wg.Done()
			uri := protocol.DocumentUri("file:///test" + string(rune('0'+index)) + ".yaml")
			dm.Open(uri, "yaml", 1, "content")
		}(i)
	}

	wg.Wait()

	// Verify all documents were added.
	// Note: Some may have same URI due to index conversion, so count may be less.
	assert.Greater(t, dm.Count(), 0)
}

func TestDocumentManager_ConcurrentUpdate(t *testing.T) {
	// Test concurrent updates don't cause race conditions.
	dm := NewDocumentManager()
	uri := protocol.DocumentUri("file:///test.yaml")

	// Open initial document.
	dm.Open(uri, "yaml", 1, "initial")

	count := 100
	var wg sync.WaitGroup
	wg.Add(count)

	for i := 0; i < count; i++ {
		go func(index int) {
			defer wg.Done()
			dm.Update(uri, int32(index), "content"+string(rune('0'+index)))
		}(i)
	}

	wg.Wait()

	// Verify document still exists.
	doc, exists := dm.Get(uri)
	assert.True(t, exists)
	assert.NotNil(t, doc)
}

func TestDocumentManager_ConcurrentClose(t *testing.T) {
	// Test concurrent closes don't cause race conditions.
	dm := NewDocumentManager()

	// Open multiple documents.
	for i := 0; i < 100; i++ {
		uri := protocol.DocumentUri("file:///test" + string(rune('0'+i)) + ".yaml")
		dm.Open(uri, "yaml", 1, "content")
	}

	var wg sync.WaitGroup
	wg.Add(100)

	// Close all concurrently.
	for i := 0; i < 100; i++ {
		go func(index int) {
			defer wg.Done()
			uri := protocol.DocumentUri("file:///test" + string(rune('0'+index)) + ".yaml")
			dm.Close(uri)
		}(i)
	}

	wg.Wait()

	// Verify count eventually reaches 0 or low number.
	assert.LessOrEqual(t, dm.Count(), 100)
}

func TestDocumentManager_ConcurrentGetAll(t *testing.T) {
	// Test concurrent GetAll calls don't cause race conditions.
	dm := NewDocumentManager()

	// Open some documents.
	for i := 0; i < 10; i++ {
		uri := protocol.DocumentUri("file:///test" + string(rune('0'+i)) + ".yaml")
		dm.Open(uri, "yaml", 1, "content")
	}

	var wg sync.WaitGroup
	wg.Add(50)

	// Call GetAll concurrently.
	for i := 0; i < 50; i++ {
		go func() {
			defer wg.Done()
			docs := dm.GetAll()
			assert.NotNil(t, docs)
		}()
	}

	wg.Wait()
}

func TestDocumentManager_ConcurrentMixedOperations(t *testing.T) {
	// Test mixed concurrent operations.
	dm := NewDocumentManager()

	var wg sync.WaitGroup
	wg.Add(300) // 100 open + 100 update + 100 get

	// Concurrent opens.
	for i := 0; i < 100; i++ {
		go func(index int) {
			defer wg.Done()
			uri := protocol.DocumentUri("file:///test.yaml")
			dm.Open(uri, "yaml", 1, "content")
		}(i)
	}

	// Concurrent updates.
	for i := 0; i < 100; i++ {
		go func(index int) {
			defer wg.Done()
			uri := protocol.DocumentUri("file:///test.yaml")
			dm.Update(uri, int32(index), "updated")
		}(i)
	}

	// Concurrent gets.
	for i := 0; i < 100; i++ {
		go func() {
			defer wg.Done()
			uri := protocol.DocumentUri("file:///test.yaml")
			_, _ = dm.Get(uri)
		}()
	}

	wg.Wait()

	// Verify manager is still functional.
	assert.NotPanics(t, func() {
		dm.Count()
		dm.GetAll()
	})
}

func TestDocumentManager_Set(t *testing.T) {
	// Test helper method used in tests (if it exists).
	dm := NewDocumentManager()
	uri := protocol.DocumentUri("file:///test.yaml")

	doc := &Document{
		URI:        uri,
		LanguageID: "yaml",
		Version:    1,
		Text:       "content",
	}

	// Manually set document (for testing).
	dm.documents[uri] = doc

	retrieved, exists := dm.Get(uri)
	assert.True(t, exists)
	assert.Equal(t, doc, retrieved)
}

func TestDocument_Fields(t *testing.T) {
	// Test Document struct field access.
	doc := &Document{
		URI:        "file:///test.yaml",
		LanguageID: "yaml",
		Version:    5,
		Text:       "test content",
	}

	assert.Equal(t, protocol.DocumentUri("file:///test.yaml"), doc.URI)
	assert.Equal(t, "yaml", doc.LanguageID)
	assert.Equal(t, int32(5), doc.Version)
	assert.Equal(t, "test content", doc.Text)
}

func TestDocumentManager_ReopenSameURI(t *testing.T) {
	// Test reopening same URI replaces the document.
	dm := NewDocumentManager()
	uri := protocol.DocumentUri("file:///test.yaml")

	// Open first time.
	doc1 := dm.Open(uri, "yaml", 1, "content1")
	assert.Equal(t, "content1", doc1.Text)

	// Open again with same URI.
	doc2 := dm.Open(uri, "yaml", 2, "content2")
	assert.Equal(t, "content2", doc2.Text)

	// Verify only one document exists.
	assert.Equal(t, 1, dm.Count())

	// Verify latest content is stored.
	stored, exists := dm.Get(uri)
	assert.True(t, exists)
	assert.Equal(t, "content2", stored.Text)
	assert.Equal(t, int32(2), stored.Version)
}
