package server

import (
	"path/filepath"
	"strings"

	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
	"gopkg.in/yaml.v3"
)

// severityPtr returns a pointer to a DiagnosticSeverity.
func severityPtr(s protocol.DiagnosticSeverity) *protocol.DiagnosticSeverity {
	return &s
}

// validateDocument validates a document and publishes diagnostics.
func (h *Handler) validateDocument(context *glsp.Context, doc *Document) {
	if doc == nil {
		return
	}

	diagnostics := h.validateAtmosFile(doc)

	// Publish diagnostics.
	context.Notify(protocol.ServerTextDocumentPublishDiagnostics, protocol.PublishDiagnosticsParams{
		URI:         doc.URI,
		Diagnostics: diagnostics,
	})
}

// validateAtmosFile validates an Atmos stack or component file.
func (h *Handler) validateAtmosFile(doc *Document) []protocol.Diagnostic {
	var diagnostics []protocol.Diagnostic

	// Get file path from URI.
	filePath := strings.TrimPrefix(doc.URI, "file://")

	// Determine file type.
	ext := strings.ToLower(filepath.Ext(filePath))

	switch ext {
	case ".yaml", ".yml":
		// Validate YAML syntax first.
		yamlDiags := h.validateYAMLSyntax(doc)
		diagnostics = append(diagnostics, yamlDiags...)

		// If YAML is valid, perform Atmos-specific validation.
		if len(yamlDiags) == 0 {
			atmosDiags := h.validateAtmosStack(doc)
			diagnostics = append(diagnostics, atmosDiags...)
		}

	case ".tf", ".hcl":
		// TODO: Add Terraform/HCL validation support.
		// For now, just validate basic syntax.

	default:
		// Unknown file type, no validation.
		return diagnostics
	}

	return diagnostics
}

// validateYAMLSyntax validates YAML syntax.
func (h *Handler) validateYAMLSyntax(doc *Document) []protocol.Diagnostic {
	var diagnostics []protocol.Diagnostic

	// Try to parse YAML.
	var content interface{}
	err := yaml.Unmarshal([]byte(doc.Text), &content)
	if err != nil {
		// YAML syntax error.
		diag := protocol.Diagnostic{
			Range: protocol.Range{
				Start: protocol.Position{Line: 0, Character: 0},
				End:   protocol.Position{Line: 0, Character: 0},
			},
			Severity: severityPtr(protocol.DiagnosticSeverityError),
			Source:   stringPtr("atmos-lsp"),
			Message:  "YAML syntax error: " + err.Error(),
		}

		// Try to extract line number from error message if possible.
		if yamlErr, ok := err.(*yaml.TypeError); ok {
			if len(yamlErr.Errors) > 0 {
				diag.Message = "YAML syntax error: " + yamlErr.Errors[0]
			}
		}

		diagnostics = append(diagnostics, diag)
	}

	return diagnostics
}

// validateAtmosStack performs Atmos-specific validation.
func (h *Handler) validateAtmosStack(doc *Document) []protocol.Diagnostic {
	var diagnostics []protocol.Diagnostic

	// Parse YAML into a map.
	var stackContent map[string]interface{}
	if err := yaml.Unmarshal([]byte(doc.Text), &stackContent); err != nil {
		return diagnostics
	}

	// Validate different sections of the stack.
	diagnostics = append(diagnostics, h.validateImportSection(stackContent)...)
	diagnostics = append(diagnostics, h.validateComponentsSection(stackContent)...)
	diagnostics = append(diagnostics, h.validateVarsSection(stackContent)...)

	return diagnostics
}

// validateImportSection validates the 'import' section.
func (h *Handler) validateImportSection(stackContent map[string]interface{}) []protocol.Diagnostic {
	var diagnostics []protocol.Diagnostic

	if imports, ok := stackContent["import"]; ok {
		if _, isArray := imports.([]interface{}); !isArray {
			diagnostics = append(diagnostics, h.createDiagnostic("'import' should be an array"))
		}
	}

	return diagnostics
}

// validateComponentsSection validates the 'components' section.
func (h *Handler) validateComponentsSection(stackContent map[string]interface{}) []protocol.Diagnostic {
	var diagnostics []protocol.Diagnostic

	components, ok := stackContent["components"]
	if !ok {
		return diagnostics
	}

	compMap, isMap := components.(map[string]interface{})
	if !isMap {
		diagnostics = append(diagnostics, h.createDiagnostic("'components' should be a map"))
		return diagnostics
	}

	// Validate terraform components.
	diagnostics = append(diagnostics, h.validateTerraformComponents(compMap)...)

	// Validate helmfile components.
	diagnostics = append(diagnostics, h.validateHelmfileComponents(compMap)...)

	return diagnostics
}

// validateTerraformComponents validates terraform components.
func (h *Handler) validateTerraformComponents(compMap map[string]interface{}) []protocol.Diagnostic {
	var diagnostics []protocol.Diagnostic

	tf, ok := compMap["terraform"]
	if !ok {
		return diagnostics
	}

	tfMap, isMap := tf.(map[string]interface{})
	if !isMap {
		diagnostics = append(diagnostics, h.createDiagnostic("'components.terraform' should be a map"))
		return diagnostics
	}

	for compName := range tfMap {
		if compName == "" {
			diagnostics = append(diagnostics, h.createDiagnostic("Component name cannot be empty"))
		}
	}

	return diagnostics
}

// validateHelmfileComponents validates helmfile components.
func (h *Handler) validateHelmfileComponents(compMap map[string]interface{}) []protocol.Diagnostic {
	var diagnostics []protocol.Diagnostic

	helmfile, ok := compMap["helmfile"]
	if !ok {
		return diagnostics
	}

	if _, isMap := helmfile.(map[string]interface{}); !isMap {
		diagnostics = append(diagnostics, h.createDiagnostic("'components.helmfile' should be a map"))
	}

	return diagnostics
}

// validateVarsSection validates the 'vars' section.
func (h *Handler) validateVarsSection(stackContent map[string]interface{}) []protocol.Diagnostic {
	var diagnostics []protocol.Diagnostic

	if vars, ok := stackContent["vars"]; ok {
		if _, isMap := vars.(map[string]interface{}); !isMap {
			diagnostics = append(diagnostics, h.createDiagnostic("'vars' should be a map"))
		}
	}

	return diagnostics
}

// createDiagnostic creates a protocol diagnostic with error severity.
// Line and character positions are set to 0 since we don't have precise error locations yet.
func (h *Handler) createDiagnostic(message string) protocol.Diagnostic {
	return protocol.Diagnostic{
		Range: protocol.Range{
			Start: protocol.Position{
				Line:      0,
				Character: 0,
			},
			End: protocol.Position{
				Line:      0,
				Character: 0,
			},
		},
		Severity: severityPtr(protocol.DiagnosticSeverityError),
		Source:   stringPtr("atmos-lsp"),
		Message:  message,
	}
}
