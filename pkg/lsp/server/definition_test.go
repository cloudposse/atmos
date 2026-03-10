package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestExtractImportPath(t *testing.T) {
	h := &Handler{}

	tests := []struct {
		name     string
		line     string
		expected string
	}{
		{
			name:     "simple import path",
			line:     "  - catalog/vpc",
			expected: "catalog/vpc",
		},
		{
			name:     "simple import path with extra spaces",
			line:     "    -   catalog/rds  ",
			expected: "catalog/rds",
		},
		{
			name:     "object form with path key double quotes",
			line:     `  - path: "shared/network.yaml"`,
			expected: "shared/network.yaml",
		},
		{
			name:     "object form with path key single quotes",
			line:     `  - path: 'shared/network.yaml'`,
			expected: "shared/network.yaml",
		},
		{
			name:     "object form path key without quotes",
			line:     "    path: shared/network.yaml",
			expected: "shared/network.yaml",
		},
		{
			name:     "yaml key line is not an import",
			line:     "  vars:",
			expected: "",
		},
		{
			name:     "empty line",
			line:     "",
			expected: "",
		},
		{
			name:     "list item with colon is not a bare path",
			line:     "  - key: value",
			expected: "",
		},
		{
			name:     "deeply nested simple import",
			line:     "      - mixins/region/us-east-1",
			expected: "mixins/region/us-east-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := h.extractImportPath(tt.line)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsInImportSection(t *testing.T) {
	h := &Handler{}

	tests := []struct {
		name     string
		doc      string
		lineNum  int
		expected bool
	}{
		{
			name:     "first item under import section",
			doc:      "import:\n  - catalog/vpc\n  - catalog/rds",
			lineNum:  1,
			expected: true,
		},
		{
			name:     "second item under import section",
			doc:      "import:\n  - catalog/vpc\n  - catalog/rds",
			lineNum:  2,
			expected: true,
		},
		{
			name:     "line under vars section",
			doc:      "vars:\n  namespace: test",
			lineNum:  1,
			expected: false,
		},
		{
			name:     "line under components section",
			doc:      "import:\n  - catalog/vpc\n\ncomponents:\n  terraform:\n    vpc:",
			lineNum:  5,
			expected: false,
		},
		{
			name:     "import key line itself",
			doc:      "import:\n  - catalog/vpc",
			lineNum:  0,
			expected: true,
		},
		{
			name:     "skips comments when walking up",
			doc:      "import:\n  # Load VPC catalog\n  - catalog/vpc",
			lineNum:  2,
			expected: true,
		},
		{
			name:     "skips empty lines when walking up",
			doc:      "import:\n\n  - catalog/vpc",
			lineNum:  2,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := splitTestLines(tt.doc)
			result := h.isInImportSection(lines, tt.lineNum)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCountLeadingSpaces(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"no spaces", "hello", 0},
		{"two spaces", "  hello", 2},
		{"four spaces", "    hello", 4},
		{"tab", "\thello", 2},
		{"empty string", "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, countLeadingSpaces(tt.input))
		})
	}
}

func TestResolveImportPath(t *testing.T) {
	// Create a temporary directory structure simulating stacks.
	tmpDir := t.TempDir()
	catalogDir := filepath.Join(tmpDir, "catalog")
	require.NoError(t, os.MkdirAll(catalogDir, 0o755))

	// Create a mock stack file.
	vpcFile := filepath.Join(catalogDir, "vpc.yaml")
	require.NoError(t, os.WriteFile(vpcFile, []byte("vars:\n  enabled: true\n"), 0o644))

	atmosConfig := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath: tmpDir,
	}
	srv := &Server{atmosConfig: atmosConfig}
	h := &Handler{server: srv, documents: NewDocumentManager()}

	t.Run("resolves existing import path", func(t *testing.T) {
		paths := h.resolveImportPath("catalog/vpc")
		require.Len(t, paths, 1)
		assert.Equal(t, vpcFile, paths[0])
	})

	t.Run("returns nil for nonexistent path", func(t *testing.T) {
		paths := h.resolveImportPath("catalog/nonexistent")
		assert.Nil(t, paths)
	})

	t.Run("returns nil when no base path configured", func(t *testing.T) {
		emptySrv := &Server{atmosConfig: &schema.AtmosConfiguration{}}
		emptyHandler := &Handler{server: emptySrv, documents: NewDocumentManager()}
		paths := emptyHandler.resolveImportPath("catalog/vpc")
		assert.Nil(t, paths)
	})

	t.Run("returns nil with nil server", func(t *testing.T) {
		nilHandler := &Handler{server: nil, documents: NewDocumentManager()}
		paths := nilHandler.resolveImportPath("catalog/vpc")
		assert.Nil(t, paths)
	})
}

func TestGetDefinitionLocations(t *testing.T) {
	// Create a temporary directory structure.
	tmpDir := t.TempDir()
	catalogDir := filepath.Join(tmpDir, "catalog")
	require.NoError(t, os.MkdirAll(catalogDir, 0o755))

	vpcFile := filepath.Join(catalogDir, "vpc.yaml")
	require.NoError(t, os.WriteFile(vpcFile, []byte("vars:\n  enabled: true\n"), 0o644))

	atmosConfig := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath: tmpDir,
	}
	srv := &Server{atmosConfig: atmosConfig}
	h := &Handler{server: srv, documents: NewDocumentManager()}

	t.Run("returns location for simple import", func(t *testing.T) {
		doc := &Document{
			URI:  "file:///test/stacks/dev.yaml",
			Text: "import:\n  - catalog/vpc\n",
		}
		locations := h.getDefinitionLocations(doc, protocol.Position{Line: 1, Character: 5})
		require.Len(t, locations, 1)
		assert.Contains(t, locations[0].URI, "catalog/vpc.yaml")
		assert.Equal(t, protocol.Position{Line: 0, Character: 0}, locations[0].Range.Start)
	})

	t.Run("returns nil for non-import line", func(t *testing.T) {
		doc := &Document{
			URI:  "file:///test/stacks/dev.yaml",
			Text: "vars:\n  namespace: test\n",
		}
		locations := h.getDefinitionLocations(doc, protocol.Position{Line: 1, Character: 5})
		assert.Nil(t, locations)
	})

	t.Run("returns nil for invalid import path", func(t *testing.T) {
		doc := &Document{
			URI:  "file:///test/stacks/dev.yaml",
			Text: "import:\n  - nonexistent/path\n",
		}
		locations := h.getDefinitionLocations(doc, protocol.Position{Line: 1, Character: 5})
		assert.Nil(t, locations)
	})

	t.Run("returns nil for line beyond document", func(t *testing.T) {
		doc := &Document{
			URI:  "file:///test/stacks/dev.yaml",
			Text: "import:\n  - catalog/vpc\n",
		}
		locations := h.getDefinitionLocations(doc, protocol.Position{Line: 10, Character: 0})
		assert.Nil(t, locations)
	})

	t.Run("handles object import form", func(t *testing.T) {
		doc := &Document{
			URI:  "file:///test/stacks/dev.yaml",
			Text: "import:\n  - path: catalog/vpc\n",
		}
		locations := h.getDefinitionLocations(doc, protocol.Position{Line: 1, Character: 10})
		require.Len(t, locations, 1)
		assert.Contains(t, locations[0].URI, "catalog/vpc.yaml")
	})
}

func TestTextDocumentDefinition(t *testing.T) {
	// Create temp stacks directory.
	tmpDir := t.TempDir()
	catalogDir := filepath.Join(tmpDir, "catalog")
	require.NoError(t, os.MkdirAll(catalogDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(catalogDir, "vpc.yaml"),
		[]byte("vars:\n  enabled: true\n"), 0o644,
	))

	atmosConfig := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath: tmpDir,
	}

	t.Run("returns location for valid import", func(t *testing.T) {
		ctx := context.Background()
		srv, err := NewServer(ctx, atmosConfig)
		require.NoError(t, err)

		handler := srv.GetHandler()
		uri := "file:///test/stacks/dev.yaml"
		handler.documents.Set(uri, &Document{
			URI:  uri,
			Text: "import:\n  - catalog/vpc\n",
		})

		params := &protocol.DefinitionParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri},
				Position:     protocol.Position{Line: 1, Character: 5},
			},
		}

		result, err := handler.TextDocumentDefinition(&glsp.Context{}, params)
		require.NoError(t, err)
		require.NotNil(t, result)

		// Single location is returned directly (not wrapped in array).
		loc, ok := result.(protocol.Location)
		require.True(t, ok)
		assert.Contains(t, loc.URI, "catalog/vpc.yaml")
	})

	t.Run("returns nil for non-existent document", func(t *testing.T) {
		ctx := context.Background()
		srv, err := NewServer(ctx, atmosConfig)
		require.NoError(t, err)

		handler := srv.GetHandler()
		params := &protocol.DefinitionParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: "file:///nonexistent.yaml"},
				Position:     protocol.Position{Line: 0, Character: 0},
			},
		}

		result, err := handler.TextDocumentDefinition(&glsp.Context{}, params)
		require.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("returns nil for non-import content", func(t *testing.T) {
		ctx := context.Background()
		srv, err := NewServer(ctx, atmosConfig)
		require.NoError(t, err)

		handler := srv.GetHandler()
		uri := "file:///test/stacks/dev.yaml"
		handler.documents.Set(uri, &Document{
			URI:  uri,
			Text: "vars:\n  region: us-east-1\n",
		})

		params := &protocol.DefinitionParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri},
				Position:     protocol.Position{Line: 1, Character: 3},
			},
		}

		result, err := handler.TextDocumentDefinition(&glsp.Context{}, params)
		require.NoError(t, err)
		assert.Nil(t, result)
	})
}

// splitTestLines splits a document string into lines.
func splitTestLines(s string) []string {
	lines := []string{}
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start <= len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
