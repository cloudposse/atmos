package ui

import (
	"encoding/json"
	"errors"
	"io"
	"strings"

	yaml "gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/utils"
)

// attributeValue keeps uncolored logical lines separate from their display language.
// Diff comparisons operate on these lines, before highlighting or visual wrapping.
type attributeValue struct {
	lines  []string
	format string
}

func (v attributeValue) isBlock() bool {
	return v.format != "" || len(v.lines) > 1
}

func plainAttributeValue(text string) attributeValue {
	return attributeValue{lines: strings.Split(text, newlineStr)}
}

// formatAttributeValue detects documents without interpreting scalar strings as YAML.
// Callers must substitute sensitive and unknown placeholders before invoking it.
func formatAttributeValue(value any, config *RenderConfig) attributeValue {
	if value == nil {
		return attributeValue{}
	}
	if isComplexValue(value) {
		return attributeValue{lines: formatComplexValue(value, nil), format: "json"}
	}
	text, ok := value.(string)
	if !ok {
		return plainAttributeValue(formatSimpleValue(value, false))
	}
	if formatted, ok := formatJSONDocument(text); ok {
		return attributeValue{lines: strings.Split(formatted, newlineStr), format: "json"}
	}
	indent := utils.DefaultYAMLIndent
	if config != nil && config.AtmosConfig != nil && config.AtmosConfig.Settings.Terminal.TabWidth > 0 {
		indent = config.AtmosConfig.Settings.Terminal.TabWidth
	}
	if formatted, ok := formatYAMLDocument(text, indent); ok {
		return attributeValue{lines: strings.Split(formatted, newlineStr), format: "yaml"}
	}
	return plainAttributeValue(text)
}

// formatJSONDocument preserves numeric precision and requires a complete object or array.
func formatJSONDocument(text string) (string, bool) {
	if !json.Valid([]byte(text)) {
		return "", false
	}
	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.UseNumber()
	var value any
	if decoder.Decode(&value) != nil || !isComplexValue(value) {
		return "", false
	}
	formatted, err := utils.ConvertToJSON(value)
	return formatted, err == nil
}

// formatYAMLDocument retains comments, tags, and scalar spelling via the syntax tree.
// It uses the YAML parser directly, never Atmos's function-evaluating config loader.
func formatYAMLDocument(text string, indent int) (string, bool) {
	decoder := yaml.NewDecoder(strings.NewReader(text))
	var document, extra yaml.Node
	if decoder.Decode(&document) != nil || len(document.Content) != 1 {
		return "", false
	}
	root := document.Content[0]
	if root.Kind != yaml.MappingNode && root.Kind != yaml.SequenceNode {
		return "", false
	}
	if !errors.Is(decoder.Decode(&extra), io.EOF) {
		return "", false
	}
	// Validate duplicate keys and invalid aliases without changing the syntax tree.
	var value any
	if document.Decode(&value) != nil {
		return "", false
	}
	expandYAMLCollections(root)
	formatted, err := utils.ConvertToYAML(&document, utils.YAMLOptions{Indent: indent})
	return strings.TrimSuffix(formatted, newlineStr), err == nil
}

// expandYAMLCollections pretty-prints flow collections while preserving scalar styles.
func expandYAMLCollections(node *yaml.Node) {
	if node.Kind == yaml.MappingNode || node.Kind == yaml.SequenceNode {
		node.Style &^= yaml.FlowStyle
	}
	for _, child := range node.Content {
		expandYAMLCollections(child)
	}
}

// displayLines highlights the entire document so multiline lexer state is preserved.
// The existing highlighter mutates terminal settings, so pass a configuration copy.
func (v attributeValue) displayLines(config *RenderConfig) []string {
	lines := v.lines
	if v.format != "" && attributeHighlightingEnabled(config.AtmosConfig) {
		atmosConfig := *config.AtmosConfig
		text := strings.Join(lines, newlineStr) + newlineStr
		if highlighted, err := utils.HighlightCodeWithConfig(&atmosConfig, text, v.format); err == nil {
			colored := strings.Split(strings.TrimSuffix(highlighted, newlineStr), newlineStr)
			if len(colored) == len(lines) {
				lines = colored
			}
		}
	}
	if v.format != "" {
		lines = collapseIfNeeded(lines, config.MaxLines)
	}
	return lines
}

func (v attributeValue) diffLines(config *RenderConfig) []string {
	if v.format != "" {
		return collapseIfNeeded(v.lines, config.MaxLines)
	}
	return v.lines
}

// attributeHighlightingEnabled honors an explicit disabled setting before calling
// the legacy highlighter, which otherwise fills false fields with enabled defaults.
func attributeHighlightingEnabled(config *schema.AtmosConfiguration) bool {
	if config == nil {
		return false
	}
	settings := config.Settings.Terminal.SyntaxHighlighting
	return settings == (schema.SyntaxHighlighting{}) || settings.Enabled
}
