package server

import (
	"strings"

	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

// TextDocumentHover handles the textDocument/hover request.
func (h *Handler) TextDocumentHover(context *glsp.Context, params *protocol.HoverParams) (*protocol.Hover, error) {
	doc, exists := h.documents.Get(params.TextDocument.URI)
	if !exists {
		return nil, nil
	}

	// Get hover information for the position.
	hoverContent := h.getHoverContent(doc, params.Position)
	if hoverContent == "" {
		return nil, nil
	}

	return &protocol.Hover{
		Contents: protocol.MarkupContent{
			Kind:  protocol.MarkupKindMarkdown,
			Value: hoverContent,
		},
	}, nil
}

// getHoverContent returns hover documentation for the given position.
func (h *Handler) getHoverContent(doc *Document, pos protocol.Position) string {
	// Get the word at the cursor position.
	lines := strings.Split(doc.Text, "\n")
	if int(pos.Line) >= len(lines) {
		return ""
	}

	currentLine := lines[pos.Line]
	word := h.getWordAtPosition(currentLine, int(pos.Character))

	if word == "" {
		return ""
	}

	// Provide hover documentation for known Atmos keywords.
	return h.getKeywordDocumentation(word)
}

// getWordAtPosition extracts the word at the given character position.
func (h *Handler) getWordAtPosition(line string, char int) string {
	if char >= len(line) {
		return ""
	}

	// Find word boundaries.
	start := char
	end := char

	// Move start backwards to find word beginning.
	for start > 0 && (isWordChar(line[start-1])) {
		start--
	}

	// Move end forwards to find word end.
	for end < len(line) && isWordChar(line[end]) {
		end++
	}

	return line[start:end]
}

// isWordChar returns true if the character is part of a word.
func isWordChar(c byte) bool {
	return (c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') ||
		c == '_' || c == '-'
}

// getKeywordDocumentation returns documentation for Atmos keywords.
func (h *Handler) getKeywordDocumentation(word string) string {
	docs := map[string]string{
		"import": "**import**\n\nImport other stack configuration files.\n\nExample:\n```yaml\nimport:\n  - catalog/vpc\n  - mixins/region/us-east-1\n```\n\nThe import section allows you to compose stacks from reusable configurations.",

		"components": "**components**\n\nDefine infrastructure components for this stack.\n\nExample:\n```yaml\ncomponents:\n  terraform:\n    vpc:\n      vars:\n        cidr_block: \"10.0.0.0/16\"\n```\n\nComponents can be Terraform modules, Helmfile releases, or other deployment types.",

		"vars": "**vars**\n\nDefine variables for the stack or component.\n\nExample:\n```yaml\nvars:\n  namespace: acme\n  environment: prod\n  region: us-east-1\n```\n\nVariables can be referenced in component configurations and templates.",

		"settings": "**settings**\n\nConfigure stack-specific settings.\n\nExample:\n```yaml\nsettings:\n  spacelift:\n    workspace_enabled: true\n  atlantis:\n    apply_requirements: [\"approved\"]\n```\n\nSettings control integrations like Spacelift, Atlantis, and validation rules.",

		"metadata": "**metadata**\n\nDefine metadata for the stack.\n\nExample:\n```yaml\nmetadata:\n  component: vpc\n  type: real\n  terraform_workspace_pattern: \"{tenant}-{environment}-{stage}\"\n```\n\nMetadata provides additional information about the stack configuration.",

		"terraform": "**terraform**\n\nTerraform component type.\n\nDefine Terraform components within this section.\n\nExample:\n```yaml\ncomponents:\n  terraform:\n    vpc:\n      component: vpc\n      vars:\n        cidr_block: \"10.0.0.0/16\"\n```",

		"helmfile": "**helmfile**\n\nHelmfile component type.\n\nDefine Helmfile releases within this section.\n\nExample:\n```yaml\ncomponents:\n  helmfile:\n    nginx:\n      vars:\n        replicas: 3\n```",

		"namespace": "**namespace**\n\nNamespace for cloud resources.\n\nTypically the organization or company name.\n\nExample: `namespace: acme`",

		"tenant": "**tenant**\n\nTenant identifier for multi-tenancy.\n\nExample: `tenant: platform`",

		"environment": "**environment**\n\nEnvironment name (e.g., dev, staging, prod).\n\nExample: `environment: prod`",

		"stage": "**stage**\n\nStage identifier within an environment.\n\nExample: `stage: blue`",

		"region": "**region**\n\nCloud provider region.\n\nExample: `region: us-east-1`",

		"enabled": "**enabled**\n\nEnable or disable a component.\n\nExample: `enabled: true`",
	}

	if doc, ok := docs[word]; ok {
		return doc
	}

	return ""
}
