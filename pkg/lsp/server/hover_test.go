package server

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

func TestTextDocumentHover(t *testing.T) {
	tests := []struct {
		name        string
		docContent  string
		position    protocol.Position
		wantHover   bool
		checkHover  func(t *testing.T, hover *protocol.Hover)
		description string
	}{
		{
			name:       "hover over 'import' keyword",
			docContent: "import:\n  - catalog",
			position:   protocol.Position{Line: 0, Character: 2}, // Middle of "import"
			wantHover:  true,
			checkHover: func(t *testing.T, hover *protocol.Hover) {
				contents, ok := hover.Contents.(protocol.MarkupContent)
				require.True(t, ok, "Contents should be MarkupContent")
				assert.Contains(t, contents.Value, "import")
				assert.Contains(t, contents.Value, "Import other stack")
				assert.Equal(t, protocol.MarkupKindMarkdown, contents.Kind)
			},
			description: "Should show hover for 'import' keyword",
		},
		{
			name:       "hover over 'components' keyword",
			docContent: "components:\n  terraform: {}",
			position:   protocol.Position{Line: 0, Character: 5}, // Middle of "components"
			wantHover:  true,
			checkHover: func(t *testing.T, hover *protocol.Hover) {
				assert.Contains(t, hover.Contents.Value, "components")
				assert.Contains(t, hover.Contents.Value, "Define infrastructure")
			},
			description: "Should show hover for 'components' keyword",
		},
		{
			name:       "hover over 'vars' keyword",
			docContent: "vars:\n  region: us-east-1",
			position:   protocol.Position{Line: 0, Character: 2},
			wantHover:  true,
			checkHover: func(t *testing.T, hover *protocol.Hover) {
				assert.Contains(t, hover.Contents.Value, "vars")
				assert.Contains(t, hover.Contents.Value, "variables")
			},
			description: "Should show hover for 'vars' keyword",
		},
		{
			name:       "hover over 'terraform' keyword",
			docContent: "components:\n  terraform:\n    vpc: {}",
			position:   protocol.Position{Line: 1, Character: 5},
			wantHover:  true,
			checkHover: func(t *testing.T, hover *protocol.Hover) {
				assert.Contains(t, hover.Contents.Value, "terraform")
				assert.Contains(t, hover.Contents.Value, "component type")
			},
			description: "Should show hover for 'terraform' keyword",
		},
		{
			name:       "hover over 'namespace' variable",
			docContent: "vars:\n  namespace: acme",
			position:   protocol.Position{Line: 1, Character: 5},
			wantHover:  true,
			checkHover: func(t *testing.T, hover *protocol.Hover) {
				assert.Contains(t, hover.Contents.Value, "namespace")
			},
			description: "Should show hover for 'namespace' variable",
		},
		{
			name:       "hover over unknown keyword",
			docContent: "unknown_key: value",
			position:   protocol.Position{Line: 0, Character: 5},
			wantHover:  false,
			description: "Should return nil for unknown keywords",
		},
		{
			name:       "hover at empty position",
			docContent: "import:\n  - catalog\n\n",
			position:   protocol.Position{Line: 2, Character: 0},
			wantHover:  false,
			description: "Should return nil for empty line",
		},
		{
			name:       "hover beyond line length",
			docContent: "import:\n  - catalog",
			position:   protocol.Position{Line: 10, Character: 0},
			wantHover:  false,
			description: "Should return nil for position beyond document",
		},
		{
			name:       "document not found",
			docContent: "",
			position:   protocol.Position{Line: 0, Character: 0},
			wantHover:  false,
			description: "Should return nil for non-existent document",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			server, err := NewServer(ctx, nil)
			require.NoError(t, err)

			handler := server.GetHandler()
			glspContext := &glsp.Context{}

			// For the "document not found" test, skip adding document.
			var uri string
			if tt.name != "document not found" {
				uri = "file:///test.yaml"
				handler.documents.Set(uri, &Document{
					URI:  uri,
					Text: tt.docContent,
				})
			} else {
				uri = "file:///nonexistent.yaml"
			}

			params := &protocol.HoverParams{
				TextDocumentPositionParams: protocol.TextDocumentPositionParams{
					TextDocument: protocol.TextDocumentIdentifier{
						URI: uri,
					},
					Position: tt.position,
				},
			}

			hover, err := handler.TextDocumentHover(glspContext, params)
			require.NoError(t, err)

			if !tt.wantHover {
				assert.Nil(t, hover, tt.description)
				return
			}

			require.NotNil(t, hover, tt.description)
			if tt.checkHover != nil {
				tt.checkHover(t, hover)
			}
		})
	}
}

func TestGetHoverContent(t *testing.T) {
	tests := []struct {
		name        string
		docContent  string
		position    protocol.Position
		wantContent bool
		checkContent func(t *testing.T, content string)
	}{
		{
			name:        "valid keyword",
			docContent:  "import:\n  - catalog",
			position:    protocol.Position{Line: 0, Character: 2},
			wantContent: true,
			checkContent: func(t *testing.T, content string) {
				assert.NotEmpty(t, content)
				assert.Contains(t, content, "import")
			},
		},
		{
			name:        "position beyond line",
			docContent:  "key: value",
			position:    protocol.Position{Line: 10, Character: 0},
			wantContent: false,
		},
		{
			name:        "empty word",
			docContent:  "  \n  ",
			position:    protocol.Position{Line: 0, Character: 1},
			wantContent: false,
		},
		{
			name:        "unknown keyword",
			docContent:  "unknown_key: value",
			position:    protocol.Position{Line: 0, Character: 5},
			wantContent: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := &Document{
				URI:  "file:///test.yaml",
				Text: tt.docContent,
			}

			handler := &Handler{}
			content := handler.getHoverContent(doc, tt.position)

			if !tt.wantContent {
				assert.Empty(t, content)
				return
			}

			if tt.checkContent != nil {
				tt.checkContent(t, content)
			}
		})
	}
}

func TestGetWordAtPosition(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		char     int
		wantWord string
	}{
		{
			name:     "word at start",
			line:     "import: value",
			char:     2,
			wantWord: "import",
		},
		{
			name:     "word in middle",
			line:     "key: components: value",
			char:     7,
			wantWord: "components",
		},
		{
			name:     "word with underscore",
			line:     "my_variable: value",
			char:     5,
			wantWord: "my_variable",
		},
		{
			name:     "word with hyphen",
			line:     "my-variable: value",
			char:     5,
			wantWord: "my-variable",
		},
		{
			name:     "word with numbers",
			line:     "var123: value",
			char:     3,
			wantWord: "var123",
		},
		{
			name:     "at word boundary (start)",
			line:     "import: value",
			char:     0,
			wantWord: "import",
		},
		{
			name:     "at word boundary (end)",
			line:     "import: value",
			char:     5, // After 't'
			wantWord: "import",
		},
		{
			name:     "position beyond line",
			line:     "key: value",
			char:     100,
			wantWord: "",
		},
		{
			name:     "on whitespace",
			line:     "key:   value",
			char:     5, // On space
			wantWord: "",
		},
		{
			name:     "empty line",
			line:     "",
			char:     0,
			wantWord: "",
		},
		{
			name:     "on special character",
			line:     "key: value",
			char:     3, // On ':'
			wantWord: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &Handler{}
			word := handler.getWordAtPosition(tt.line, tt.char)
			assert.Equal(t, tt.wantWord, word)
		})
	}
}

func TestIsWordChar(t *testing.T) {
	tests := []struct {
		char byte
		want bool
	}{
		// Valid word characters.
		{'a', true},
		{'z', true},
		{'A', true},
		{'Z', true},
		{'0', true},
		{'9', true},
		{'_', true},
		{'-', true},

		// Invalid word characters.
		{' ', false},
		{':', false},
		{'.', false},
		{',', false},
		{'/', false},
		{'\\', false},
		{'!', false},
		{'@', false},
		{'#', false},
		{'(', false},
		{')', false},
	}

	for _, tt := range tests {
		t.Run(string(tt.char), func(t *testing.T) {
			result := isWordChar(tt.char)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestGetKeywordDocumentation(t *testing.T) {
	handler := &Handler{}

	// Test all documented keywords.
	keywords := []string{
		"import", "components", "vars", "settings", "metadata",
		"terraform", "helmfile",
		"namespace", "tenant", "environment", "stage", "region", "enabled",
	}

	for _, keyword := range keywords {
		t.Run(keyword, func(t *testing.T) {
			doc := handler.getKeywordDocumentation(keyword)

			assert.NotEmpty(t, doc, "Should have documentation for: %s", keyword)
			assert.Contains(t, doc, keyword, "Documentation should mention the keyword")
			assert.Contains(t, doc, "**"+keyword+"**", "Documentation should have markdown header")
		})
	}

	// Test unknown keyword.
	t.Run("unknown keyword", func(t *testing.T) {
		doc := handler.getKeywordDocumentation("unknown_key")
		assert.Empty(t, doc)
	})
}

func TestHoverDocumentationFormat(t *testing.T) {
	// Verify all hover documentation follows markdown format.
	handler := &Handler{}

	keywords := []string{
		"import", "components", "vars", "settings", "metadata",
		"terraform", "helmfile",
		"namespace", "tenant", "environment", "stage", "region", "enabled",
	}

	for _, keyword := range keywords {
		t.Run(keyword, func(t *testing.T) {
			doc := handler.getKeywordDocumentation(keyword)

			// Should start with bold keyword.
			assert.True(t, strings.HasPrefix(doc, "**"+keyword+"**"),
				"Documentation should start with bold keyword: %s", keyword)

			// Should contain example if it's a major keyword.
			if keyword == "import" || keyword == "components" || keyword == "vars" || keyword == "settings" {
				assert.Contains(t, doc, "Example:", "Major keyword should have example: %s", keyword)
				assert.Contains(t, doc, "```", "Example should be in code block: %s", keyword)
			}
		})
	}
}

func TestHoverAllKeywordsCovered(t *testing.T) {
	// Ensure all keywords from completion also have hover documentation.
	handler := &Handler{}

	// Get all completion keywords.
	topLevel := handler.getTopLevelCompletions()
	componentTypes := handler.getComponentTypeCompletions()
	commonVars := handler.getCommonVarCompletions()

	// Check top-level keywords have documentation.
	for _, item := range topLevel {
		t.Run("top-level/"+item.Label, func(t *testing.T) {
			doc := handler.getKeywordDocumentation(item.Label)
			assert.NotEmpty(t, doc, "Top-level keyword should have hover documentation: %s", item.Label)
		})
	}

	// Check component types have documentation.
	for _, item := range componentTypes {
		t.Run("component-type/"+item.Label, func(t *testing.T) {
			doc := handler.getKeywordDocumentation(item.Label)
			assert.NotEmpty(t, doc, "Component type should have hover documentation: %s", item.Label)
		})
	}

	// Check common vars have documentation.
	for _, item := range commonVars {
		t.Run("var/"+item.Label, func(t *testing.T) {
			doc := handler.getKeywordDocumentation(item.Label)
			assert.NotEmpty(t, doc, "Common var should have hover documentation: %s", item.Label)
		})
	}
}
