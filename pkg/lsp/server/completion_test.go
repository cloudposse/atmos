package server

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

func TestTextDocumentCompletion(t *testing.T) {
	tests := []struct {
		name            string
		docContent      string
		position        protocol.Position
		wantItems       bool
		checkItems      func(t *testing.T, items []protocol.CompletionItem)
		description     string
	}{
		{
			name:       "empty document",
			docContent: "",
			position:   protocol.Position{Line: 0, Character: 0},
			wantItems:  true,
			checkItems: func(t *testing.T, items []protocol.CompletionItem) {
				// Should get top-level completions.
				assert.Greater(t, len(items), 0)
				hasImport := false
				hasComponents := false
				for _, item := range items {
					if item.Label == "import" {
						hasImport = true
					}
					if item.Label == "components" {
						hasComponents = true
					}
				}
				assert.True(t, hasImport, "Should include 'import'")
				assert.True(t, hasComponents, "Should include 'components'")
			},
			description: "Empty document should offer top-level keywords",
		},
		{
			name:       "at start of line",
			docContent: "\n",
			position:   protocol.Position{Line: 1, Character: 0},
			wantItems:  true,
			checkItems: func(t *testing.T, items []protocol.CompletionItem) {
				assert.Greater(t, len(items), 0)
			},
			description: "Start of new line should offer completions",
		},
		{
			name:       "after 'components:' keyword",
			docContent: "components:\n  ",
			position:   protocol.Position{Line: 1, Character: 2},
			wantItems:  true,
			checkItems: func(t *testing.T, items []protocol.CompletionItem) {
				hasTerraform := false
				hasHelmfile := false
				for _, item := range items {
					if item.Label == "terraform" {
						hasTerraform = true
					}
					if item.Label == "helmfile" {
						hasHelmfile = true
					}
				}
				assert.True(t, hasTerraform, "Should include 'terraform'")
				assert.True(t, hasHelmfile, "Should include 'helmfile'")
			},
			description: "After 'components:' should offer component types",
		},
		{
			name:       "after 'vars:' keyword",
			docContent: "vars:\n  ",
			position:   protocol.Position{Line: 1, Character: 2},
			wantItems:  true,
			checkItems: func(t *testing.T, items []protocol.CompletionItem) {
				hasRegion := false
				for _, item := range items {
					if item.Label == "region" {
						hasRegion = true
						break
					}
				}
				assert.True(t, hasRegion, "Should include common vars like 'region'")
			},
			description: "After 'vars:' should offer common variables",
		},
		{
			name:       "after 'settings:' keyword",
			docContent: "settings:\n  ",
			position:   protocol.Position{Line: 1, Character: 2},
			wantItems:  true,
			checkItems: func(t *testing.T, items []protocol.CompletionItem) {
				assert.Greater(t, len(items), 0, "Should offer settings completions")
			},
			description: "After 'settings:' should offer settings options",
		},
		{
			name:       "document not found",
			docContent: "",
			position:   protocol.Position{Line: 0, Character: 0},
			wantItems:  false,
			description: "Non-existent document should return nil",
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
				// Add document.
				handler.documents.Set(uri, &Document{
					URI:  uri,
					Text: tt.docContent,
				})
			} else {
				uri = "file:///nonexistent.yaml"
			}

			params := &protocol.CompletionParams{
				TextDocumentPositionParams: protocol.TextDocumentPositionParams{
					TextDocument: protocol.TextDocumentIdentifier{
						URI: uri,
					},
					Position: tt.position,
				},
			}

			result, err := handler.TextDocumentCompletion(glspContext, params)
			require.NoError(t, err)

			if !tt.wantItems {
				assert.Nil(t, result)
				return
			}

			require.NotNil(t, result)
			completionList, ok := result.(protocol.CompletionList)
			require.True(t, ok)

			if tt.checkItems != nil {
				tt.checkItems(t, completionList.Items)
			}
		})
	}
}

func TestGetCompletionItems(t *testing.T) {
	tests := []struct {
		name        string
		docContent  string
		position    protocol.Position
		checkItems  func(t *testing.T, items []protocol.CompletionItem)
		description string
	}{
		{
			name:       "invalid YAML",
			docContent: "invalid:\n\ttab",
			position:   protocol.Position{Line: 0, Character: 0},
			checkItems: func(t *testing.T, items []protocol.CompletionItem) {
				// Should return empty on invalid YAML.
				assert.Len(t, items, 0)
			},
			description: "Invalid YAML should return no completions",
		},
		{
			name:       "empty line",
			docContent: "import:\n  - catalog\n\n",
			position:   protocol.Position{Line: 2, Character: 0},
			checkItems: func(t *testing.T, items []protocol.CompletionItem) {
				assert.Greater(t, len(items), 0)
			},
			description: "Empty line should offer top-level completions",
		},
		{
			name:       "mid-word position",
			docContent: "compo",
			position:   protocol.Position{Line: 0, Character: 5},
			checkItems: func(t *testing.T, items []protocol.CompletionItem) {
				// Should still offer completions.
				assert.Greater(t, len(items), 0)
			},
			description: "Mid-word should offer completions",
		},
		{
			name:       "position beyond line length",
			docContent: "key: value",
			position:   protocol.Position{Line: 10, Character: 0}, // Beyond document
			checkItems: func(t *testing.T, items []protocol.CompletionItem) {
				assert.Len(t, items, 0)
			},
			description: "Position beyond line count should return empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := &Document{
				URI:  "file:///test.yaml",
				Text: tt.docContent,
			}

			handler := &Handler{}
			items := handler.getCompletionItems(doc, tt.position)

			if tt.checkItems != nil {
				tt.checkItems(t, items)
			}
		})
	}
}

func TestGetTopLevelCompletions(t *testing.T) {
	handler := &Handler{}
	items := handler.getTopLevelCompletions()

	assert.Greater(t, len(items), 0, "Should return at least one completion")

	// Verify expected top-level keys.
	expectedLabels := []string{"import", "components", "vars", "settings", "metadata"}
	foundLabels := make(map[string]bool)

	for _, item := range items {
		foundLabels[item.Label] = true

		// Verify item structure.
		assert.NotNil(t, item.Kind, "Item should have kind: %s", item.Label)
		assert.NotNil(t, item.Detail, "Item should have detail: %s", item.Label)
		assert.NotEmpty(t, item.Documentation, "Item should have documentation: %s", item.Label)
	}

	for _, label := range expectedLabels {
		assert.True(t, foundLabels[label], "Should include label: %s", label)
	}
}

func TestGetComponentTypeCompletions(t *testing.T) {
	handler := &Handler{}
	items := handler.getComponentTypeCompletions()

	assert.Greater(t, len(items), 0, "Should return component type completions")

	// Verify expected component types.
	expectedTypes := []string{"terraform", "helmfile"}
	foundTypes := make(map[string]bool)

	for _, item := range items {
		foundTypes[item.Label] = true

		// Verify item structure.
		assert.NotNil(t, item.Kind)
		assert.NotNil(t, item.Detail)
	}

	for _, typ := range expectedTypes {
		assert.True(t, foundTypes[typ], "Should include component type: %s", typ)
	}
}

func TestGetCommonVarCompletions(t *testing.T) {
	handler := &Handler{}
	items := handler.getCommonVarCompletions()

	assert.Greater(t, len(items), 0, "Should return variable completions")

	// Verify expected common variables.
	expectedVars := []string{"namespace", "tenant", "environment", "stage", "region"}
	foundVars := make(map[string]bool)

	for _, item := range items {
		foundVars[item.Label] = true

		// Verify item structure.
		assert.NotNil(t, item.Kind)
	}

	for _, varName := range expectedVars {
		assert.True(t, foundVars[varName], "Should include variable: %s", varName)
	}
}

func TestGetSettingsCompletions(t *testing.T) {
	handler := &Handler{}
	items := handler.getSettingsCompletions()

	assert.Greater(t, len(items), 0, "Should return settings completions")

	// Verify all items have proper structure.
	for _, item := range items {
		assert.NotEmpty(t, item.Label)
		assert.NotNil(t, item.Kind)
	}
}

func TestCompletionItemStructure(t *testing.T) {
	// Test that completion items have consistent structure.
	handler := &Handler{}
	allItems := [][]protocol.CompletionItem{
		handler.getTopLevelCompletions(),
		handler.getComponentTypeCompletions(),
		handler.getCommonVarCompletions(),
		handler.getSettingsCompletions(),
	}

	for _, items := range allItems {
		for _, item := range items {
			// Every item must have a label.
			assert.NotEmpty(t, item.Label, "Item must have label")

			// Every item must have a kind.
			assert.NotNil(t, item.Kind, "Item must have kind: %s", item.Label)

			// If item has detail, it should be non-empty.
			if item.Detail != nil {
				assert.NotEmpty(t, *item.Detail, "Detail should be non-empty: %s", item.Label)
			}

			// If item has insert text, it should be non-empty.
			if item.InsertText != nil {
				assert.NotEmpty(t, *item.InsertText, "InsertText should be non-empty: %s", item.Label)
			}
		}
	}
}

func TestCompletionContextAwareness(t *testing.T) {
	// Test that completions change based on context.
	handler := &Handler{}

	// Test 1: Empty line should give top-level.
	doc1 := &Document{URI: "file:///test.yaml", Text: ""}
	items1 := handler.getCompletionItems(doc1, protocol.Position{Line: 0, Character: 0})
	hasTopLevel1 := false
	for _, item := range items1 {
		if item.Label == "import" || item.Label == "components" {
			hasTopLevel1 = true
			break
		}
	}
	assert.True(t, hasTopLevel1, "Empty context should give top-level completions")

	// Test 2: After 'components:' should give component types.
	doc2 := &Document{URI: "file:///test.yaml", Text: "components:\n  "}
	items2 := handler.getCompletionItems(doc2, protocol.Position{Line: 1, Character: 2})
	hasComponentType := false
	for _, item := range items2 {
		if item.Label == "terraform" || item.Label == "helmfile" {
			hasComponentType = true
			break
		}
	}
	assert.True(t, hasComponentType, "After 'components:' should give component types")
}
