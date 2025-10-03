package provenance

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/merge"
)

// RenderInline renders YAML with inline provenance comments.
// Format: `key: value  # from: file.yaml:42`
//
// This is useful for headless environments and piping, where the provenance
// information is embedded directly in the YAML output.
func RenderInline(data any, ctx *merge.MergeContext) (string, error) {
	if ctx == nil || ctx.Provenance == nil {
		// No provenance available - return regular YAML.
		bytes, err := yaml.Marshal(data)
		if err != nil {
			return "", fmt.Errorf("failed to marshal YAML: %w", err)
		}
		return string(bytes), nil
	}

	// Marshal to YAML first.
	bytes, err := yaml.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("failed to marshal YAML: %w", err)
	}

	yamlStr := string(bytes)
	lines := strings.Split(yamlStr, "\n")

	// Build a simple map of line content to provenance.
	// This is a simplified approach - a full implementation would parse
	// the YAML structure and match it with provenance paths.
	result := &strings.Builder{}

	for _, line := range lines {
		result.WriteString(line)

		// For this minimal implementation, we just output the YAML as-is.
		// A full implementation would:
		// 1. Parse the YAML line to extract the key/path
		// 2. Look up provenance for that path
		// 3. Append the provenance comment
		//
		// For now, this serves as a placeholder that returns valid YAML.

		result.WriteString("\n")
	}

	return result.String(), nil
}

// FormatProvenanceComment formats a provenance entry as an inline comment.
func FormatProvenanceComment(entry merge.ProvenanceEntry) string {
	if entry.Column > 0 {
		return fmt.Sprintf("# from: %s:%d:%d", entry.File, entry.Line, entry.Column)
	}
	return fmt.Sprintf("# from: %s:%d", entry.File, entry.Line)
}
