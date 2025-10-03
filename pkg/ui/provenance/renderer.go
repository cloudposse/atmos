package provenance

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const (
	defaultTerminalWidth = 120
	newlineChar          = "\n"
)

// ProvenanceRenderer handles rendering component configuration with provenance information.
type ProvenanceRenderer struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewProvenanceRenderer creates a new provenance renderer.
func NewProvenanceRenderer(atmosConfig *schema.AtmosConfiguration) *ProvenanceRenderer {
	defer perf.Track(atmosConfig, "provenance.NewProvenanceRenderer")()

	return &ProvenanceRenderer{
		atmosConfig: atmosConfig,
	}
}

// RenderYAMLWithProvenance renders YAML configuration with provenance annotations.
func (r *ProvenanceRenderer) RenderYAMLWithProvenance(data any, sources schema.ConfigSources) (string, error) {
	defer perf.Track(r.atmosConfig, "provenance.RenderYAMLWithProvenance")()

	// Check if TTY is supported
	ttySupported := term.IsTTYSupportForStdout()

	if ttySupported {
		return r.renderYAMLTwoColumn(data, sources)
	}

	return r.renderYAMLInlineComments(data, sources)
}

// renderYAMLTwoColumn renders YAML in a two-column layout with provenance on the right.
func (r *ProvenanceRenderer) renderYAMLTwoColumn(data any, sources schema.ConfigSources) (string, error) {
	defer perf.Track(r.atmosConfig, "provenance.renderYAMLTwoColumn")()

	// Get highlighted YAML for left column
	yamlContent, err := u.GetHighlightedYAML(r.atmosConfig, data)
	if err != nil {
		return "", err
	}

	// Build provenance annotations for right column
	provenanceContent := r.buildProvenanceAnnotations(data, sources)

	// Split into lines
	yamlLines := strings.Split(yamlContent, "\n")
	provenanceLines := strings.Split(provenanceContent, "\n")

	// Ensure both have same number of lines
	maxLines := len(yamlLines)
	if len(provenanceLines) > maxLines {
		maxLines = len(provenanceLines)
	}

	// Pad shorter slice
	for len(yamlLines) < maxLines {
		yamlLines = append(yamlLines, "")
	}
	for len(provenanceLines) < maxLines {
		provenanceLines = append(provenanceLines, "")
	}

	// Calculate column widths
	termWidth := r.atmosConfig.Settings.Terminal.MaxWidth
	if termWidth <= 0 {
		termWidth = defaultTerminalWidth
	}

	// Reserve 3 chars for separator " │ "
	leftWidth := termWidth * 2 / 3
	rightWidth := termWidth - leftWidth - 3

	// Create styles
	leftStyle := lipgloss.NewStyle().Width(leftWidth).Align(lipgloss.Left)
	rightStyle := theme.Styles.GrayText.Width(rightWidth).Align(lipgloss.Left)
	separatorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorBorder))

	// Render each line
	var result strings.Builder
	for i := 0; i < maxLines; i++ {
		leftLine := leftStyle.Render(yamlLines[i])
		rightLine := rightStyle.Render(provenanceLines[i])
		separator := separatorStyle.Render(" │ ")

		result.WriteString(leftLine)
		result.WriteString(separator)
		result.WriteString(rightLine)
		if i < maxLines-1 {
			result.WriteString("\n")
		}
	}

	return result.String(), nil
}

// renderYAMLInlineComments renders YAML with provenance as inline comments.
func (r *ProvenanceRenderer) renderYAMLInlineComments(data any, sources schema.ConfigSources) (string, error) {
	defer perf.Track(r.atmosConfig, "provenance.renderYAMLInlineComments")()

	// Get YAML content
	yamlContent, err := u.ConvertToYAML(data)
	if err != nil {
		return "", err
	}

	lines := strings.Split(yamlContent, newlineChar)
	provenanceMap := r.buildProvenanceMap(sources)

	var result strings.Builder
	for _, line := range lines {
		// Extract the key from the line (simplified parsing)
		key := extractKeyFromLine(line)
		if key != "" {
			if provInfo, ok := provenanceMap[key]; ok {
				result.WriteString(line)
				result.WriteString("  # ")
				result.WriteString(provInfo)
				result.WriteString(newlineChar)
				continue
			}
		}
		result.WriteString(line)
		result.WriteString(newlineChar)
	}

	return result.String(), nil
}

// buildProvenanceAnnotations builds provenance annotations matching the YAML structure.
func (r *ProvenanceRenderer) buildProvenanceAnnotations(_ any, sources schema.ConfigSources) string {
	defer perf.Track(r.atmosConfig, "provenance.buildProvenanceAnnotations")()

	var result strings.Builder

	// Process each section in sources
	for section, items := range sources {
		result.WriteString(fmt.Sprintf("%s:\n", section))

		for key, item := range items {
			sourcesText := r.formatStackDependencies(item.StackDependencies)
			result.WriteString(fmt.Sprintf("  %s: %s\n", key, sourcesText))
		}
	}

	return result.String()
}

// buildProvenanceMap builds a map of keys to provenance information.
func (r *ProvenanceRenderer) buildProvenanceMap(sources schema.ConfigSources) map[string]string {
	defer perf.Track(r.atmosConfig, "provenance.buildProvenanceMap")()

	provenanceMap := make(map[string]string)

	for section, items := range sources {
		for key, item := range items {
			fullKey := fmt.Sprintf("%s.%s", section, key)
			provenanceMap[fullKey] = r.formatStackDependencies(item.StackDependencies)
		}
	}

	return provenanceMap
}

// formatStackDependencies formats stack dependencies into a readable string.
func (r *ProvenanceRenderer) formatStackDependencies(deps schema.ConfigSourcesStackDependencies) string {
	defer perf.Track(r.atmosConfig, "provenance.formatStackDependencies")()

	if len(deps) == 0 {
		return ""
	}

	var parts []string
	for _, dep := range deps {
		parts = append(parts, fmt.Sprintf("%s (%s)", dep.StackFile, dep.DependencyType))
	}

	return strings.Join(parts, ", ")
}

// extractKeyFromLine extracts the configuration key from a YAML line.
func extractKeyFromLine(line string) string {
	// Simplified key extraction - finds "key:" pattern
	trimmed := strings.TrimSpace(line)
	if idx := strings.Index(trimmed, ":"); idx > 0 {
		return trimmed[:idx]
	}
	return ""
}

// RenderJSONWithProvenance renders JSON configuration with embedded provenance metadata.
func (r *ProvenanceRenderer) RenderJSONWithProvenance(data any, sources schema.ConfigSources) (string, error) {
	defer perf.Track(r.atmosConfig, "provenance.RenderJSONWithProvenance")()

	// Build enhanced data structure with provenance
	enhanced := r.embedProvenanceInData(data, sources)

	// Convert to JSON with highlighting
	return u.GetHighlightedJSON(r.atmosConfig, enhanced)
}

// embedProvenanceInData embeds provenance information into the data structure.
func (r *ProvenanceRenderer) embedProvenanceInData(data any, sources schema.ConfigSources) map[string]any {
	defer perf.Track(r.atmosConfig, "provenance.embedProvenanceInData")()

	dataMap, ok := data.(map[string]any)
	if !ok {
		return make(map[string]any)
	}

	result := make(map[string]any)
	for k, v := range dataMap {
		if k == "sources" {
			continue
		}

		result[k] = r.embedProvenanceForKey(k, v, sources)
	}

	return result
}

func (r *ProvenanceRenderer) embedProvenanceForKey(key string, value any, sources schema.ConfigSources) any {
	sourceSection, ok := sources[key]
	if !ok {
		return value
	}

	enhanced := make(map[string]any)
	enhanced["value"] = value

	provenanceItems := r.buildProvenanceItems(sourceSection)
	if len(provenanceItems) > 0 {
		enhanced["__provenance"] = provenanceItems
	}

	return enhanced
}

func (r *ProvenanceRenderer) buildProvenanceItems(sourceSection map[string]schema.ConfigSourcesItem) map[string]any {
	provenanceItems := make(map[string]any)
	for key, item := range sourceSection {
		provenanceItems[key] = map[string]any{
			"sources": r.convertDependenciesToJSON(item.StackDependencies),
		}
	}
	return provenanceItems
}

// convertDependenciesToJSON converts stack dependencies to JSON-friendly format.
func (r *ProvenanceRenderer) convertDependenciesToJSON(deps schema.ConfigSourcesStackDependencies) []map[string]any {
	defer perf.Track(r.atmosConfig, "provenance.convertDependenciesToJSON")()

	var result []map[string]any
	for _, dep := range deps {
		result = append(result, map[string]any{
			"file": dep.StackFile,
			"type": dep.DependencyType,
		})
	}
	return result
}
