package server

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/cloudposse/atmos/pkg/config"
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

// importPathPattern matches a YAML list item with a bare import path (e.g., "  - catalog/vpc").
var importPathPattern = regexp.MustCompile(`^\s*-\s+(.+)$`)

// importObjectPathPattern matches import objects with a path key (e.g., '  - path: "shared/network.yaml"').
var importObjectPathPattern = regexp.MustCompile(`^\s*(?:-\s+)?path:\s*["']?([^"'\s]+)["']?`)

// getDefinitionLocations returns definition locations for the given position.
func (h *Handler) getDefinitionLocations(doc *Document, pos protocol.Position) []protocol.Location {
	lines := strings.Split(doc.Text, "\n")
	if int(pos.Line) >= len(lines) {
		return nil
	}

	currentLine := lines[pos.Line]

	// Check if the cursor is within an import section.
	if !h.isInImportSection(lines, int(pos.Line)) {
		return nil
	}

	// Extract the import path from the current line.
	importPath := h.extractImportPath(currentLine)
	if importPath == "" {
		return nil
	}

	// Resolve the import path to actual file(s).
	filePaths := h.resolveImportPath(importPath)
	if len(filePaths) == 0 {
		return nil
	}

	// Convert file paths to LSP locations.
	locations := make([]protocol.Location, 0, len(filePaths))
	for _, fp := range filePaths {
		absPath, err := filepath.Abs(fp)
		if err != nil {
			continue
		}
		uri := "file://" + absPath
		locations = append(locations, protocol.Location{
			URI: uri,
			Range: protocol.Range{
				Start: protocol.Position{Line: 0, Character: 0},
				End:   protocol.Position{Line: 0, Character: 0},
			},
		})
	}

	return locations
}

// isInImportSection checks whether the given line is inside an import: block.
func (h *Handler) isInImportSection(lines []string, lineNum int) bool {
	currentLine := lines[lineNum]
	currentIndent := countLeadingSpaces(currentLine)

	// Walk backwards to find a parent key.
	for i := lineNum - 1; i >= 0; i-- {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// Skip empty lines and comments.
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		lineIndent := countLeadingSpaces(line)

		// Found a line with less indentation - this is a potential parent.
		if lineIndent < currentIndent {
			return strings.HasPrefix(trimmed, "import:")
		}
	}

	// Check if current line itself starts the import section.
	trimmed := strings.TrimSpace(currentLine)
	return strings.HasPrefix(trimmed, "import:")
}

// extractImportPath extracts the import path string from a YAML line.
func (h *Handler) extractImportPath(line string) string {
	// Try object form first: "path: ..." or "- path: ...".
	if matches := importObjectPathPattern.FindStringSubmatch(line); len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}

	// Try simple list item: "- catalog/vpc".
	if matches := importPathPattern.FindStringSubmatch(line); len(matches) > 1 {
		value := strings.TrimSpace(matches[1])
		// Skip if this is a YAML key (contains ":") - it's an object, not a path.
		if strings.Contains(value, ":") {
			return ""
		}
		return value
	}

	return ""
}

// resolveImportPath resolves an import path to actual file path(s) on disk.
func (h *Handler) resolveImportPath(importPath string) []string {
	basePath := h.getStacksBasePath()
	if basePath == "" {
		return nil
	}

	// Build the full path by joining base path with import path.
	fullPath := filepath.Join(basePath, importPath)

	// Use SearchAtmosConfig which handles .yaml/.yml extension resolution and glob patterns.
	paths, err := config.SearchAtmosConfig(fullPath)
	if err != nil || len(paths) == 0 {
		return nil
	}

	return paths
}

// getStacksBasePath returns the stacks base directory from atmos config.
func (h *Handler) getStacksBasePath() string {
	if h.server == nil || h.server.atmosConfig == nil {
		return ""
	}

	// Prefer the absolute path if available.
	if h.server.atmosConfig.StacksBaseAbsolutePath != "" {
		return h.server.atmosConfig.StacksBaseAbsolutePath
	}

	// Fall back to constructing from base path + stacks base path.
	if h.server.atmosConfig.Stacks.BasePath != "" {
		if h.server.atmosConfig.BasePath != "" {
			return filepath.Join(h.server.atmosConfig.BasePath, h.server.atmosConfig.Stacks.BasePath)
		}
		return h.server.atmosConfig.Stacks.BasePath
	}

	// Try to derive from the document URI.
	return ""
}

// countLeadingSpaces counts the number of leading spaces in a string.
func countLeadingSpaces(s string) int {
	count := 0
	for _, ch := range s {
		if ch == ' ' {
			count++
		} else if ch == '\t' {
			count += 2
		} else {
			break
		}
	}
	return count
}
