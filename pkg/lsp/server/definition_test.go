package server

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

func TestTextDocumentDefinition(t *testing.T) {
	tests := []struct {
		name         string
		docContent   string
		position     protocol.Position
		wantResult   bool
		checkResult  func(t *testing.T, result any)
		description  string
	}{
		{
			name:       "stub returns nil for any position",
			docContent: "import:\n  - catalog/vpc",
			position:   protocol.Position{Line: 1, Character: 5},
			wantResult: false,
			description: "Definition lookup is not yet implemented (stub)",
		},
		{
			name:       "non-existent document returns nil",
			docContent: "",
			position:   protocol.Position{Line: 0, Character: 0},
			wantResult: false,
			description: "Non-existent document should return nil",
		},
		{
			name:       "valid stack file returns nil (stub)",
			docContent: "components:\n  terraform:\n    vpc: {}",
			position:   protocol.Position{Line: 2, Character: 5},
			wantResult: false,
			description: "Even valid files return nil in stub implementation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			server, err := NewServer(ctx, nil)
			require.NoError(t, err)

			handler := server.GetHandler()
			glspContext := &glsp.Context{}

			// For non-existent document test, skip adding document.
			var uri string
			if tt.name != "non-existent document returns nil" {
				uri = "file:///test.yaml"
				handler.documents.Set(uri, &Document{
					URI:  uri,
					Text: tt.docContent,
				})
			} else {
				uri = "file:///nonexistent.yaml"
			}

			params := &protocol.DefinitionParams{
				TextDocumentPositionParams: protocol.TextDocumentPositionParams{
					TextDocument: protocol.TextDocumentIdentifier{
						URI: uri,
					},
					Position: tt.position,
				},
			}

			result, err := handler.TextDocumentDefinition(glspContext, params)
			require.NoError(t, err)

			if !tt.wantResult {
				assert.Nil(t, result, tt.description)
				return
			}

			if tt.checkResult != nil {
				tt.checkResult(t, result)
			}
		})
	}
}

func TestGetDefinitionLocations(t *testing.T) {
	tests := []struct {
		name         string
		docContent   string
		position     protocol.Position
		wantLocations int
	}{
		{
			name:          "stub returns empty array",
			docContent:    "import:\n  - catalog/vpc",
			position:      protocol.Position{Line: 1, Character: 5},
			wantLocations: 0,
		},
		{
			name:          "any position returns empty",
			docContent:    "components:\n  terraform:\n    vpc: {}",
			position:      protocol.Position{Line: 0, Character: 0},
			wantLocations: 0,
		},
		{
			name:          "empty document returns empty",
			docContent:    "",
			position:      protocol.Position{Line: 0, Character: 0},
			wantLocations: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := &Document{
				URI:  "file:///test.yaml",
				Text: tt.docContent,
			}

			handler := &Handler{}
			locations := handler.getDefinitionLocations(doc, tt.position)

			assert.Len(t, locations, tt.wantLocations)
		})
	}
}

func TestDefinitionStubBehavior(t *testing.T) {
	// Verify stub implementation consistently returns nil/empty.
	ctx := context.Background()
	server, err := NewServer(ctx, nil)
	require.NoError(t, err)

	handler := server.GetHandler()
	glspContext := &glsp.Context{}

	// Test multiple different scenarios.
	testCases := []struct {
		uri     string
		content string
		line    uint32
		char    uint32
	}{
		{"file:///test1.yaml", "import:\n  - catalog/vpc", 1, 5},
		{"file:///test2.yaml", "components:\n  terraform: {}", 0, 5},
		{"file:///test3.yaml", "vars:\n  region: us-east-1", 1, 3},
		{"file:///test4.yaml", "", 0, 0},
	}

	for _, tc := range testCases {
		handler.documents.Set(tc.uri, &Document{
			URI:  tc.uri,
			Text: tc.content,
		})

		params := &protocol.DefinitionParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: tc.uri},
				Position:     protocol.Position{Line: tc.line, Character: tc.char},
			},
		}

		result, err := handler.TextDocumentDefinition(glspContext, params)
		require.NoError(t, err)
		assert.Nil(t, result, "Stub should consistently return nil for URI: %s", tc.uri)
	}
}

func TestDefinitionHelperMethod(t *testing.T) {
	// Helper method for tests to set documents without using Open.
	dm := NewDocumentManager()

	uri := protocol.DocumentUri("file:///test.yaml")
	doc := &Document{
		URI:        uri,
		LanguageID: "yaml",
		Version:    1,
		Text:       "test content",
	}

	// Manually set document.
	dm.documents[uri] = doc

	retrieved, exists := dm.Get(uri)
	assert.True(t, exists)
	assert.Equal(t, doc, retrieved)
}

// TODO: When definition feature is implemented, add tests for:
// - Jump to import file location
// - Jump to component definition
// - Jump to variable definition
// - Multiple definition locations
// - Invalid/unresolvable references
// - Cross-file references
// - Relative vs absolute paths
