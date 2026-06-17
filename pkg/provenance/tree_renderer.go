package provenance

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"

	termUtils "github.com/cloudposse/atmos/internal/tui/templates/term"
	log "github.com/cloudposse/atmos/pkg/logger"
	m "github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const (
	// SymbolDefined indicates value defined at this level.
	SymbolDefined = "●" // U+25CF BLACK CIRCLE
	// SymbolInherited indicates value inherited/imported.
	SymbolInherited = "○" // U+25CB WHITE CIRCLE
	// SymbolComputed indicates a computed/templated value.
	SymbolComputed = "∴"

	// Rendering constants.
	defaultSeparatorWidth = 60   // Width of separator lines
	commentSpaceNeeded    = 60   // Space needed for provenance comments
	maxLineLength         = 10   // Buffer subtracted from comment column
	maxArrayCheckLimit    = 1000 // Maximum array elements to check for provenance

	// String constants used repeatedly.
	pathSeparator = "."
	yamlKeySep    = ":"
	pathSpace     = " "
	newlineChar   = "\n"
	importsKey    = "imports"
)

// YAMLLineInfo contains information about a YAML line for provenance tracking.
type YAMLLineInfo struct {
	Path           string // JSONPath for this line (e.g., "vars.enabled")
	IsKeyLine      bool   // true if this line contains a key
	IsContinuation bool   // true if this is a continuation of a multi-line value
}

// isProvenanceColorEnabled checks whether provenance output should include color.
// Follows the same pattern as HighlightCodeWithConfig: NoColor wins, then ForceColor,
// then fall back to stdout TTY detection (not stderr, since provenance goes to stdout).
func isProvenanceColorEnabled(atmosConfig *schema.AtmosConfiguration) bool {
	if atmosConfig == nil {
		return false
	}
	if atmosConfig.Settings.Terminal.NoColor {
		return false
	}
	if atmosConfig.Settings.Terminal.ForceColor {
		return true
	}
	return termUtils.IsTTYSupportForStdout()
}

// findProvenance looks up provenance for a normalized path.
// It tries the exact path first, then tries with common prefixes.
func findProvenance(ctx *m.MergeContext, normalizedPath string) *m.ProvenanceEntry {
	defer perf.Track(nil, "provenance.findProvenance")()

	if ctx == nil || !ctx.IsProvenanceEnabled() {
		return nil
	}

	// Try to find provenance by checking all stored paths
	for _, storedPath := range ctx.GetProvenancePaths() {
		// Normalize the stored path and compare
		if normalizeProvenancePath(storedPath) == normalizedPath {
			entries := ctx.GetProvenance(storedPath)
			if len(entries) > 0 {
				// Return the first (most recent/winning) entry
				return &entries[0]
			}
		}
	}

	return nil
}

// formatProvenanceCommentWithStackFile creates an inline comment for provenance.
func formatProvenanceCommentWithStackFile(entry *m.ProvenanceEntry, useColor bool) string {
	defer perf.Track(nil, "provenance.formatProvenanceCommentWithStackFile")()

	if entry == nil {
		return ""
	}

	// Determine symbol based on depth and type.
	var symbol string
	switch {
	case entry.Type == m.ProvenanceTypeComputed:
		symbol = SymbolComputed // ∴ (computed/templated)
	case entry.Depth == 1:
		symbol = SymbolDefined // ● (defined in parent stack - depth 1)
	default:
		symbol = SymbolInherited // ○ (inherited/imported - depth 2+)
	}

	file := shortenFilePath(entry.File)

	if !useColor {
		return fmt.Sprintf("# %s [%d] %s:%d", symbol, entry.Depth, file, entry.Line)
	}

	// Color code the depth based on inheritance level.
	// Depth 1-2: green (parent + first import), 3: orange, 4+: red.
	var depthColor lipgloss.Color
	switch entry.Depth {
	case 1, 2:
		depthColor = lipgloss.Color(theme.ColorGreen)
	case 3:
		depthColor = lipgloss.Color(theme.ColorOrange)
	default: // 4+
		depthColor = lipgloss.Color(theme.ColorRed)
	}

	// Format comment parts.
	grayStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorDarkGray))
	depthStyle := lipgloss.NewStyle().Foreground(depthColor)

	// Build: "# symbol [depth] file:line" with colored depth.
	comment := fmt.Sprintf(
		"%s %s %s %s:%d",
		grayStyle.Render("#"),
		grayStyle.Render(symbol),
		depthStyle.Render(fmt.Sprintf("[%d]", entry.Depth)),
		grayStyle.Render(file),
		entry.Line,
	)

	return comment
}

// shortenFilePath removes the "stacks/" prefix for brevity.
func shortenFilePath(path string) string {
	return strings.TrimPrefix(path, "stacks/")
}

// getCommentColumn determines where provenance comments should start.
// Uses terminal width if TTY is attached, otherwise uses default.
func getCommentColumn() int {
	defer perf.Track(nil, "provenance.getCommentColumn")()

	const defaultColumn = 50
	const minColumn = 40
	const commentSpace = 60 // Space needed for comment (# ● file:line).

	// Check if stdout is a TTY
	if !termUtils.IsTTYSupportForStdout() {
		return defaultColumn
	}

	// Get terminal width
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width == 0 {
		return defaultColumn
	}

	// Calculate comment column: terminal_width - comment_space
	// But ensure we have at least minColumn for YAML
	commentColumn := width - commentSpace
	if commentColumn < minColumn {
		commentColumn = minColumn
	}

	return commentColumn
}

// PrepareYAMLForProvenance prepares YAML data for provenance rendering.
func prepareYAMLForProvenance(yamlData any, ctx *m.MergeContext, atmosConfig *schema.AtmosConfiguration) (string, error) {
	defer perf.Track(atmosConfig, "provenance.prepareYAMLForProvenance")()

	// Rename "imports" → "import"
	yamlData = renameImportsToImport(yamlData, ctx)

	// Filter out empty sections
	filteredData := filterEmptySections(yamlData, ctx)

	// Wrap long strings
	wrappedMaxLength := getCommentColumn() - maxLineLength
	wrappedData := u.WrapLongStrings(filteredData, wrappedMaxLength)

	// Get indent from configuration
	indent := u.DefaultYAMLIndent
	if atmosConfig != nil && atmosConfig.Settings.Terminal.TabWidth > 0 {
		indent = atmosConfig.Settings.Terminal.TabWidth
	}

	// Convert to YAML with configured indent
	yamlBytes, err := u.ConvertToYAML(wrappedData, u.YAMLOptions{Indent: indent})
	if err != nil {
		return "", err
	}

	// Apply syntax highlighting - if it fails, fall back to plain YAML
	highlighted, _ := u.HighlightCodeWithConfig(atmosConfig, yamlBytes, "yaml")
	if highlighted == "" {
		// Highlighting failed, return plain YAML
		return yamlBytes, nil
	}

	return highlighted, nil
}

// addProvenanceToLine adds provenance comment to a YAML line.
func addProvenanceToLine(
	result *strings.Builder,
	line string,
	entry *m.ProvenanceEntry,
	commentColumn int,
	useColor bool,
) {
	plainLine := stripANSI(line)
	lineLen := len(plainLine)

	comment := formatProvenanceCommentWithStackFile(entry, useColor)
	if comment == "" {
		result.WriteString(line)
		result.WriteString(newlineChar)
		return
	}

	// Add provenance comment
	if lineLen < commentColumn {
		// Line is short enough - add padding and comment on same line
		result.WriteString(line)
		padding := commentColumn - lineLen
		result.WriteString(strings.Repeat(pathSpace, padding))
		result.WriteString(comment)
	} else {
		// Line is too long - add comment on next line indented
		result.WriteString(line)
		result.WriteString(newlineChar)
		result.WriteString(strings.Repeat(pathSpace, commentColumn))
		result.WriteString(comment)
	}

	result.WriteString(newlineChar)
}

// renderProvenanceLegend renders the provenance legend and stack file header.
func renderProvenanceLegend(result *strings.Builder, stackFile string, useColor bool) {
	legend := "# Provenance Legend:" + newlineChar +
		"#   ● [1] Defined in parent stack" + newlineChar +
		"#   ○ [N] Inherited/imported (N=2+ levels deep)" + newlineChar +
		"#   ∴ Computed/templated" + newlineChar

	if useColor {
		legendStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorDarkGray))
		result.WriteString(legendStyle.Render(legend))
	} else {
		result.WriteString(legend)
	}
	result.WriteString(newlineChar)

	// Add stack file comment to show which file is being described.
	if stackFile != "" {
		stackComment := fmt.Sprintf("# Stack: %s%s", stackFile, newlineChar)
		if useColor {
			legendStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorDarkGray))
			result.WriteString(legendStyle.Render(stackComment))
		} else {
			result.WriteString(stackComment)
		}
		result.WriteString(newlineChar)
	}
}

// lineProvenanceContext holds context for processing a line with provenance.
type lineProvenanceContext struct {
	pathMap       map[int]YAMLLineInfo
	ctx           *m.MergeContext
	commentColumn int
	useColor      bool
}

// processYAMLLineWithProvenance processes a single line and adds provenance comment if applicable.
func processYAMLLineWithProvenance(result *strings.Builder, line string, lineNum int, lpCtx *lineProvenanceContext) {
	info, exists := lpCtx.pathMap[lineNum]

	// Skip provenance for continuation lines.
	if exists && info.IsContinuation {
		result.WriteString(line)
		result.WriteString(newlineChar)
		return
	}

	// Only add provenance if this is a key line.
	if !exists || !info.IsKeyLine {
		result.WriteString(line)
		result.WriteString(newlineChar)
		return
	}

	// Look up and add provenance.
	entry := findProvenance(lpCtx.ctx, info.Path)
	if entry == nil {
		result.WriteString(line)
		result.WriteString(newlineChar)
		return
	}

	addProvenanceToLine(result, line, entry, lpCtx.commentColumn, lpCtx.useColor)
}

// RenderInlineProvenanceWithStackFile renders YAML with provenance as inline comments.
// The stackFile parameter is the stack manifest file being described (e.g., "orgs/acme/plat/dev/us-east-2.yaml").
// Values from this file will be marked with ● (defined), while values from other files show ○ (inherited).
func RenderInlineProvenanceWithStackFile(yamlData any, ctx *m.MergeContext, atmosConfig *schema.AtmosConfiguration, stackFile string) (output string) {
	defer perf.Track(atmosConfig, "provenance.RenderInlineProvenanceWithStackFile")()

	// Recover from panics in YAML marshalling (e.g., channels, funcs).
	defer func() {
		if r := recover(); r != nil {
			log.Debug("Panic during YAML marshalling", "error", r)
			output = fmt.Sprintf("Error rendering YAML: %v\n", r)
		}
	}()

	useColor := isProvenanceColorEnabled(atmosConfig)

	var result strings.Builder

	// Add legend at top only if provenance is enabled.
	if ctx != nil && ctx.IsProvenanceEnabled() {
		renderProvenanceLegend(&result, stackFile, useColor)
	}

	// Prepare YAML with provenance.
	highlighted, err := prepareYAMLForProvenance(yamlData, ctx, atmosConfig)
	if err != nil {
		return fmt.Sprintf("Error rendering YAML: %v\n", err)
	}

	// Split into lines and build path mapping.
	lines := strings.Split(highlighted, newlineChar)
	pathMap := buildYAMLPathMap(lines)
	commentColumn := getCommentColumn()

	// Create context for line processing.
	lpCtx := &lineProvenanceContext{
		pathMap:       pathMap,
		ctx:           ctx,
		commentColumn: commentColumn,
		useColor:      useColor,
	}

	// Process each line with provenance.
	for i, line := range lines {
		processYAMLLineWithProvenance(&result, line, i, lpCtx)
	}

	return result.String()
}
