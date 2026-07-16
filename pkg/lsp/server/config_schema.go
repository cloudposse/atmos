package server

import (
	"path/filepath"
	"regexp"
	"strings"

	protocol "github.com/tliron/glsp/protocol_3_16"
	"github.com/xeipuuv/gojsonschema"
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/cloudposse/atmos/pkg/validator"
)

// configSchemaSource is the embedded atmos.yaml JSON Schema, generated from the
// Atmos configuration structs (see pkg/config/schema) — the same document
// `atmos config schema` prints and `atmos validate schema` uses by default.
const configSchemaSource = "atmos://schema/atmos/config/1.0"

// schemaFieldIndexPattern rewrites gojsonschema field paths (commands.0.env)
// into the JSONPath form the position extractor records (commands[0].env).
var schemaFieldIndexPattern = regexp.MustCompile(`\.(\d+)`)

// isAtmosConfigDocument reports whether a file is an Atmos CLI configuration
// document: atmos.yaml (including hidden variants), an atmos.d fragment, or a
// project-local profile fragment.
func isAtmosConfigDocument(filePath string) bool {
	base := filepath.Base(filePath)
	switch base {
	case "atmos.yaml", "atmos.yml", ".atmos.yaml", ".atmos.yml":
		return true
	}
	slashed := filepath.ToSlash(filePath)
	for _, dir := range []string{"/atmos.d/", "/.atmos.d/", "/.atmos/profiles/"} {
		if strings.Contains(slashed, dir) {
			return true
		}
	}
	return false
}

// validateConfigSchema validates an atmos.yaml document (or fragment) against
// the embedded generated schema and maps violations to positioned diagnostics.
func (h *Handler) validateConfigSchema(doc *Document) []protocol.Diagnostic {
	var diagnostics []protocol.Diagnostic

	yamlValidator := validator.NewYAMLSchemaValidator(h.serverAtmosConfig())
	validationErrors, err := yamlValidator.ValidateYAMLContent(configSchemaSource, []byte(doc.Text))
	if err != nil {
		// Syntax errors are reported by validateYAMLSyntax; schema-fetch or
		// conversion failures produce no schema diagnostics.
		return diagnostics
	}

	positions := documentPositions(doc)
	for _, validationError := range filterCascadeErrors(validationErrors) {
		field := validationError.Field()
		position := u.GetYAMLPosition(positions, schemaFieldToJSONPath(field))

		// LSP positions are 0-based; the YAML parser reports 1-based.
		line := uint32(max(0, position.Line-1))     //nolint:gosec // Bounded by max(0, ...).
		column := uint32(max(0, position.Column-1)) //nolint:gosec // Bounded by max(0, ...).

		diagnostics = append(diagnostics, protocol.Diagnostic{
			Range: protocol.Range{
				Start: protocol.Position{Line: line, Character: column},
				End:   protocol.Position{Line: line, Character: column},
			},
			Severity: severityPtr(protocol.DiagnosticSeverityError),
			Source:   stringPtr("atmos-lsp"),
			Message:  field + ": " + validationError.Description(),
		})
	}

	return diagnostics
}

// anyOfErrorType is gojsonschema's error type for a failed anyOf keyword.
const anyOfErrorType = "number_any_of"

// filterCascadeErrors drops generic anyOf failures that have a more specific
// nested error, so diagnostics point at the offending leaf value instead of
// every ancestor (the generated schema wraps most values in anyOf to admit
// YAML functions and null).
func filterCascadeErrors(validationErrors []gojsonschema.ResultError) []gojsonschema.ResultError {
	filtered := make([]gojsonschema.ResultError, 0, len(validationErrors))
	for _, validationError := range validationErrors {
		if validationError.Type() == anyOfErrorType && hasMoreSpecificError(validationErrors, validationError.Field()) {
			continue
		}
		filtered = append(filtered, validationError)
	}
	return filtered
}

// hasMoreSpecificError reports whether a non-anyOf error exists at the field or
// deeper within it.
func hasMoreSpecificError(validationErrors []gojsonschema.ResultError, field string) bool {
	for _, other := range validationErrors {
		if other.Type() == anyOfErrorType {
			continue
		}
		if other.Field() == field || strings.HasPrefix(other.Field(), field+".") {
			return true
		}
	}
	return false
}

// serverAtmosConfig returns the server's Atmos configuration, tolerating a
// handler constructed without a server (tests).
func (h *Handler) serverAtmosConfig() *schema.AtmosConfiguration {
	if h == nil || h.server == nil {
		return nil
	}
	return h.server.atmosConfig
}

// documentPositions parses the document and extracts JSONPath -> position
// mappings so schema violations can point at the offending line.
func documentPositions(doc *Document) u.PositionMap {
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(doc.Text), &node); err != nil {
		return u.PositionMap{}
	}
	return u.ExtractYAMLPositions(&node, true)
}

// schemaFieldToJSONPath converts a gojsonschema field path (commands.0.env)
// into the JSONPath form used by the position extractor (commands[0].env).
func schemaFieldToJSONPath(field string) string {
	if field == "(root)" {
		return ""
	}
	return schemaFieldIndexPattern.ReplaceAllString(field, "[$1]")
}
