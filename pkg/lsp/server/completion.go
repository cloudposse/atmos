package server

import (
	"strings"

	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
	"gopkg.in/yaml.v3"
)

// stringPtr returns a pointer to a string.
func stringPtr(s string) *string {
	return &s
}

// kindPtr returns a pointer to a CompletionItemKind.
func kindPtr(k protocol.CompletionItemKind) *protocol.CompletionItemKind {
	return &k
}

// TextDocumentCompletion handles the textDocument/completion request.
func (h *Handler) TextDocumentCompletion(context *glsp.Context, params *protocol.CompletionParams) (any, error) {
	doc, exists := h.documents.Get(params.TextDocument.URI)
	if !exists {
		return nil, nil
	}

	// Get completion items based on context.
	items := h.getCompletionItems(doc, params.Position)

	return protocol.CompletionList{
		IsIncomplete: false,
		Items:        items,
	}, nil
}

// getCompletionItems returns completion items for the given position.
func (h *Handler) getCompletionItems(doc *Document, pos protocol.Position) []protocol.CompletionItem {
	var items []protocol.CompletionItem

	// Parse the document to understand context.
	// We continue even if YAML is invalid since user may be mid-typing.
	var content map[string]interface{}
	_ = yaml.Unmarshal([]byte(doc.Text), &content)

	// Get the line at the cursor position.
	lines := strings.Split(doc.Text, "\n")
	if int(pos.Line) >= len(lines) {
		return items
	}

	currentLine := lines[pos.Line]
	// Handle position beyond line length.
	if int(pos.Character) > len(currentLine) {
		return items
	}
	currentLine = currentLine[:pos.Character]
	trimmedLine := strings.TrimSpace(currentLine)

	// Detect parent scope by looking at previous lines with less indentation.
	parentScope := h.getParentScope(lines, int(pos.Line))

	// Provide context-aware completions.

	// Top-level keys - only when at root level (no parent scope).
	if parentScope == "" && (trimmedLine == "" || !strings.Contains(trimmedLine, ":")) {
		items = append(items, h.getTopLevelCompletions()...)
		return items
	}

	// Component types - when under components: scope.
	if parentScope == "components" || strings.Contains(currentLine, "components:") {
		items = append(items, h.getComponentTypeCompletions()...)
	}

	// Common Atmos variables - when under vars: scope.
	if parentScope == "vars" || strings.Contains(currentLine, "vars:") {
		items = append(items, h.getCommonVarCompletions()...)
	}

	// Settings - when under settings: scope.
	if parentScope == "settings" || strings.Contains(currentLine, "settings:") {
		items = append(items, h.getSettingsCompletions()...)
	}

	return items
}

// getParentScope determines the parent YAML key based on indentation.
func (h *Handler) getParentScope(lines []string, currentLineNum int) string {
	if currentLineNum == 0 {
		return ""
	}

	currentLine := lines[currentLineNum]
	currentIndent := len(currentLine) - len(strings.TrimLeft(currentLine, " \t"))

	// Look backwards to find the parent key with less indentation.
	for i := currentLineNum - 1; i >= 0; i-- {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		lineIndent := len(line) - len(strings.TrimLeft(line, " \t"))
		if lineIndent < currentIndent && strings.HasSuffix(trimmed, ":") {
			// Found a parent key.
			return strings.TrimSuffix(trimmed, ":")
		}
	}

	return ""
}

// getTopLevelCompletions returns top-level Atmos stack keys.
func (h *Handler) getTopLevelCompletions() []protocol.CompletionItem {
	return []protocol.CompletionItem{
		{
			Label:         "import",
			Kind:          kindPtr(protocol.CompletionItemKindKeyword),
			Detail:        stringPtr("Import other stack files"),
			Documentation: "Import one or more stack configuration files",
			InsertText:    stringPtr("import:\n  - "),
		},
		{
			Label:         "components",
			Kind:          kindPtr(protocol.CompletionItemKindKeyword),
			Detail:        stringPtr("Define components"),
			Documentation: "Define Terraform, Helmfile, or other components",
			InsertText:    stringPtr("components:\n  terraform:\n    "),
		},
		{
			Label:         "vars",
			Kind:          kindPtr(protocol.CompletionItemKindKeyword),
			Detail:        stringPtr("Define variables"),
			Documentation: "Define stack-level variables",
			InsertText:    stringPtr("vars:\n  "),
		},
		{
			Label:         "settings",
			Kind:          kindPtr(protocol.CompletionItemKindKeyword),
			Detail:        stringPtr("Stack settings"),
			Documentation: "Configure stack-specific settings",
			InsertText:    stringPtr("settings:\n  "),
		},
		{
			Label:         "metadata",
			Kind:          kindPtr(protocol.CompletionItemKindKeyword),
			Detail:        stringPtr("Metadata"),
			Documentation: "Define metadata for the stack",
			InsertText:    stringPtr("metadata:\n  "),
		},
	}
}

// getComponentTypeCompletions returns component type completions.
func (h *Handler) getComponentTypeCompletions() []protocol.CompletionItem {
	return []protocol.CompletionItem{
		{
			Label:         "terraform",
			Kind:          kindPtr(protocol.CompletionItemKindModule),
			Detail:        stringPtr("Terraform components"),
			Documentation: "Define Terraform components",
		},
		{
			Label:         "helmfile",
			Kind:          kindPtr(protocol.CompletionItemKindModule),
			Detail:        stringPtr("Helmfile components"),
			Documentation: "Define Helmfile components",
		},
	}
}

// getCommonVarCompletions returns common Atmos variable completions.
func (h *Handler) getCommonVarCompletions() []protocol.CompletionItem {
	return []protocol.CompletionItem{
		{
			Label:         "namespace",
			Kind:          kindPtr(protocol.CompletionItemKindVariable),
			Detail:        stringPtr("Namespace"),
			Documentation: "Namespace for the stack",
		},
		{
			Label:         "tenant",
			Kind:          kindPtr(protocol.CompletionItemKindVariable),
			Detail:        stringPtr("Tenant"),
			Documentation: "Tenant identifier",
		},
		{
			Label:         "environment",
			Kind:          kindPtr(protocol.CompletionItemKindVariable),
			Detail:        stringPtr("Environment"),
			Documentation: "Environment (e.g., dev, staging, prod)",
		},
		{
			Label:         "stage",
			Kind:          kindPtr(protocol.CompletionItemKindVariable),
			Detail:        stringPtr("Stage"),
			Documentation: "Stage identifier",
		},
		{
			Label:         "region",
			Kind:          kindPtr(protocol.CompletionItemKindVariable),
			Detail:        stringPtr("Region"),
			Documentation: "Cloud region",
		},
		{
			Label:         "enabled",
			Kind:          kindPtr(protocol.CompletionItemKindVariable),
			Detail:        stringPtr("Enabled flag"),
			Documentation: "Enable or disable component",
		},
		{
			Label:         "tags",
			Kind:          kindPtr(protocol.CompletionItemKindVariable),
			Detail:        stringPtr("Resource tags"),
			Documentation: "Tags to apply to resources",
		},
	}
}

// getSettingsCompletions returns settings completions.
func (h *Handler) getSettingsCompletions() []protocol.CompletionItem {
	return []protocol.CompletionItem{
		{
			Label:         "spacelift",
			Kind:          kindPtr(protocol.CompletionItemKindProperty),
			Detail:        stringPtr("Spacelift settings"),
			Documentation: "Configure Spacelift integration",
		},
		{
			Label:         "atlantis",
			Kind:          kindPtr(protocol.CompletionItemKindProperty),
			Detail:        stringPtr("Atlantis settings"),
			Documentation: "Configure Atlantis integration",
		},
		{
			Label:         "validation",
			Kind:          kindPtr(protocol.CompletionItemKindProperty),
			Detail:        stringPtr("Validation settings"),
			Documentation: "Configure validation rules",
		},
	}
}
