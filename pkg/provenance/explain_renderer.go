package provenance

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

	m "github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

const (
	explainSetByLabel      = "SET by:   "
	explainOverrideLabel   = "OVERRIDE: "
	explainIndent          = "  "
	explainValueHadPrefix  = " had "
	explainValueNone       = "no value"
	explainBulletWinner    = "●"
	explainBulletOverride  = "○"
	explainSeparatorRepeat = 60
)

// ExplainTraceEntry holds a single entry in the per-key merge trace.
type ExplainTraceEntry struct {
	// Path is the dotted key path (e.g., "vars.cidr_block").
	Path string
	// FinalValue is the winning value after all merges.
	FinalValue any
	// Winner is the provenance entry that "won" (final state).
	Winner *m.ProvenanceEntry
	// Overridden lists earlier entries whose values were superseded.
	Overridden []m.ProvenanceEntry
}

// RenderExplainTrace renders a per-key merge trace showing which file "won" for
// each configuration value and which earlier files were overridden.
// The output format matches the problem statement:
//
//	vars.cidr_block = "10.0.0.0/16"
//	  ● SET by:   stacks/orgs/acme/prod/us-east-2.yaml (line 24)
//	  ○ OVERRIDE: catalog/vpc/defaults.yaml had "172.16.0.0/16"
func RenderExplainTrace(
	data map[string]any,
	ctx *m.MergeContext,
	atmosConfig *schema.AtmosConfiguration,
	stackFile string,
) string {
	defer perf.Track(atmosConfig, "provenance.RenderExplainTrace")()

	if ctx == nil || !ctx.IsProvenanceEnabled() {
		return "No provenance data available. Run with --explain to enable merge tracing.\n"
	}

	var buf strings.Builder

	// Header.
	renderExplainHeader(&buf, stackFile)

	// Build and sort trace entries.
	entries := buildExplainEntries(data, ctx)
	if len(entries) == 0 {
		buf.WriteString("No provenance data available.\n")
		return buf.String()
	}

	// Render each entry.
	for _, entry := range entries {
		renderExplainEntry(&buf, entry)
	}

	return buf.String()
}

// renderExplainHeader writes the legend and stack file header.
func renderExplainHeader(buf *strings.Builder, stackFile string) {
	grayStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorDarkGray))

	legend := fmt.Sprintf(
		"# Merge Trace Legend:\n"+
			"#   %s SET by:   value defined here (this file wins)\n"+
			"#   %s OVERRIDE: value was here but overridden by a later file\n",
		colorize(explainBulletWinner, lipgloss.Color(theme.ColorGreen)),
		colorize(explainBulletOverride, lipgloss.Color(theme.ColorCyan)),
	)
	buf.WriteString(legend)
	buf.WriteString(newlineChar)

	if stackFile != "" {
		stackLine := fmt.Sprintf("# Stack: %s\n", stackFile)
		buf.WriteString(grayStyle.Render(stackLine))
		buf.WriteString(newlineChar)
	}

	buf.WriteString(strings.Repeat("─", explainSeparatorRepeat))
	buf.WriteString(newlineChar)
}

// buildExplainEntries walks the merged data and provenance to build a flat list
// of ExplainTraceEntry values sorted by path.
func buildExplainEntries(data map[string]any, ctx *m.MergeContext) []ExplainTraceEntry {
	paths := ctx.GetProvenancePaths()
	if len(paths) == 0 {
		return nil
	}

	// Filter out internal metadata paths (e.g., __import__:..., __import_meta__:...).
	filtered := filterExplainPaths(paths)
	sort.Strings(filtered)

	entries := make([]ExplainTraceEntry, 0, len(filtered))
	for _, path := range filtered {
		chain := ctx.GetProvenance(path)
		if len(chain) == 0 {
			continue
		}

		entry := buildExplainEntry(path, data, chain)
		entries = append(entries, entry)
	}

	return entries
}

// filterExplainPaths removes internal tracking paths from the explain output.
func filterExplainPaths(paths []string) []string {
	result := make([]string, 0, len(paths))
	for _, p := range paths {
		if strings.HasPrefix(p, "__import__:") || strings.HasPrefix(p, "__import_meta__:") {
			continue
		}
		result = append(result, p)
	}
	return result
}

// buildExplainEntry constructs a single ExplainTraceEntry for a path.
// The winner is the entry with the lowest depth (highest priority).
// Earlier entries (higher depth = lower priority) are listed as overridden.
func buildExplainEntry(path string, data map[string]any, chain []m.ProvenanceEntry) ExplainTraceEntry {
	// The last entry in the chain is the "most recent" one recorded.
	// Chain order: base (index 0) → override (index last).
	// The winner is the last-recorded entry (depth nearest to root).
	winnerIdx := findWinnerIndex(chain)

	winner := &chain[winnerIdx]
	overridden := make([]m.ProvenanceEntry, 0, len(chain)-1)
	for i, e := range chain {
		if i != winnerIdx {
			overridden = append(overridden, e)
		}
	}

	return ExplainTraceEntry{
		Path:       path,
		FinalValue: lookupValue(data, path),
		Winner:     winner,
		Overridden: overridden,
	}
}

// findWinnerIndex finds the index of the winning entry in the chain.
// The winner is the entry with the lowest depth (root = highest priority).
// In case of ties, prefer the last entry (most recently recorded).
func findWinnerIndex(chain []m.ProvenanceEntry) int {
	bestIdx := len(chain) - 1
	bestDepth := chain[bestIdx].Depth
	for i := len(chain) - 2; i >= 0; i-- {
		if chain[i].Depth < bestDepth {
			bestDepth = chain[i].Depth
			bestIdx = i
		}
	}
	return bestIdx
}

// lookupValue retrieves the final value from the merged data for a dotted path.
func lookupValue(data map[string]any, path string) any {
	parts := strings.Split(path, pathSeparator)
	current := any(data)
	for _, part := range parts {
		// Handle array index notation like "imports[0]".
		if idx, key, ok := parseArrayIndex(part); ok {
			if m, isMap := current.(map[string]any); isMap {
				arr, hasKey := m[key]
				if !hasKey {
					return nil
				}
				if slice, isSlice := arr.([]any); isSlice && idx < len(slice) {
					current = slice[idx]
					continue
				}
			}
			return nil
		}
		m, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		val, exists := m[part]
		if !exists {
			return nil
		}
		current = val
	}
	return current
}

// parseArrayIndex parses "key[N]" notation.
// Returns (index, key, true) on success or (0, "", false) if not array notation.
func parseArrayIndex(part string) (int, string, bool) {
	open := strings.Index(part, "[")
	close := strings.Index(part, "]")
	if open < 0 || close < 0 || close < open {
		return 0, "", false
	}
	key := part[:open]
	idxStr := part[open+1 : close]
	var idx int
	if _, err := fmt.Sscanf(idxStr, "%d", &idx); err != nil {
		return 0, "", false
	}
	return idx, key, true
}

// renderExplainEntry writes a single key trace block to the buffer.
func renderExplainEntry(buf *strings.Builder, entry ExplainTraceEntry) {
	// Path = FinalValue line.
	pathStyle := lipgloss.NewStyle().Bold(true)
	valueStr := formatExplainValue(entry.FinalValue)

	buf.WriteString(pathStyle.Render(entry.Path))
	buf.WriteString(" = ")
	buf.WriteString(colorize(valueStr, lipgloss.Color(theme.ColorCyan)))
	buf.WriteString(newlineChar)

	// Winner line.
	if entry.Winner != nil {
		renderWinnerLine(buf, entry.Winner)
	}

	// Overridden lines (from most-recently-overridden to oldest).
	for i := len(entry.Overridden) - 1; i >= 0; i-- {
		renderOverrideLine(buf, &entry.Overridden[i])
	}

	buf.WriteString(newlineChar)
}

// renderWinnerLine renders the "SET by" line.
func renderWinnerLine(buf *strings.Builder, winner *m.ProvenanceEntry) {
	greenStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorGreen))
	grayStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorDarkGray))

	buf.WriteString(explainIndent)
	buf.WriteString(colorize(explainBulletWinner, lipgloss.Color(theme.ColorGreen)))
	buf.WriteString(pathSpace)
	buf.WriteString(greenStyle.Render(explainSetByLabel))
	buf.WriteString(grayStyle.Render(shortenFilePath(winner.File)))
	if winner.Line > 0 {
		buf.WriteString(colorize(fmt.Sprintf(" (line %d)", winner.Line), lipgloss.Color(theme.ColorCyan)))
	}
	buf.WriteString(newlineChar)
}

// renderOverrideLine renders an "OVERRIDE" line showing a value that was superseded.
func renderOverrideLine(buf *strings.Builder, entry *m.ProvenanceEntry) {
	cyanStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorCyan))
	grayStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorDarkGray))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorDarkGray))

	buf.WriteString(explainIndent)
	buf.WriteString(colorize(explainBulletOverride, lipgloss.Color(theme.ColorCyan)))
	buf.WriteString(pathSpace)
	buf.WriteString(cyanStyle.Render(explainOverrideLabel))
	buf.WriteString(grayStyle.Render(shortenFilePath(entry.File)))
	if entry.Line > 0 {
		buf.WriteString(colorize(fmt.Sprintf(" (line %d)", entry.Line), lipgloss.Color(theme.ColorCyan)))
	}

	// Show what value this entry had (if we have it).
	if entry.Value != nil {
		valueStr := formatExplainValue(entry.Value)
		buf.WriteString(dimStyle.Render(explainValueHadPrefix))
		buf.WriteString(dimStyle.Render(valueStr))
	} else if entry.ValueHash == "" {
		buf.WriteString(dimStyle.Render(explainValueHadPrefix))
		buf.WriteString(dimStyle.Render(explainValueNone))
	}
	buf.WriteString(newlineChar)
}

// formatExplainValue formats a value for display in the explain trace.
// Scalars are quoted, maps/arrays show a short summary.
func formatExplainValue(v any) string {
	if v == nil {
		return explainValueNone
	}
	switch val := v.(type) {
	case string:
		return fmt.Sprintf("%q", val)
	case bool:
		return fmt.Sprintf("%t", val)
	case int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		return fmt.Sprintf("%v", val)
	case map[string]any:
		if len(val) == 0 {
			return "{}"
		}
		return fmt.Sprintf("{%d keys}", len(val))
	case []any:
		if len(val) == 0 {
			return "[]"
		}
		return fmt.Sprintf("[%d items]", len(val))
	default:
		// Fall back to JSON for complex types.
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(b)
	}
}
