package provenance

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"

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
)

// FileTreeNode represents provenance items grouped by file.
type FileTreeNode struct {
	File  string
	Items []ProvenanceItem
}

// ProvenanceItem represents a single provenance entry for rendering.
type ProvenanceItem struct {
	Symbol string // ● ○ ∴
	Line   int    // Line number in source file
	Path   string // vars.name
}

// YAMLLineInfo contains information about a YAML line for provenance tracking.
type YAMLLineInfo struct {
	Path           string // JSONPath for this line (e.g., "vars.enabled")
	IsKeyLine      bool   // true if this line contains a key
	IsContinuation bool   // true if this is a continuation of a multi-line value
}

// RenderTree renders provenance information as a tree structure.
func RenderTree(ctx *m.MergeContext, atmosConfig *schema.AtmosConfiguration, allowedPaths map[string]bool) string {
	defer perf.Track(atmosConfig, "provenance.RenderTree")()

	if ctx == nil || !ctx.IsProvenanceEnabled() {
		return ""
	}

	var buf strings.Builder

	// Build file tree from provenance data
	fileTree := buildFileTree(ctx, allowedPaths)

	// Header
	buf.WriteString("Provenance\n")
	buf.WriteString(strings.Repeat("─", 60))
	buf.WriteString("\n")

	// Render tree structure
	renderFileTree(&buf, fileTree)

	return buf.String()
}

// buildFileTree groups provenance entries by file.
func buildFileTree(ctx *m.MergeContext, allowedPaths map[string]bool) []FileTreeNode {
	// Group provenance entries by file
	byFile := make(map[string][]ProvenanceItem)

	for _, path := range ctx.GetProvenancePaths() {
		// Skip if we have an allowedPaths filter and this path is not in it
		if allowedPaths != nil && !allowedPaths[path] {
			continue
		}

		entries := ctx.GetProvenance(path)
		if len(entries) == 0 {
			continue
		}

		// Take FIRST entry (most recent/final value)
		entry := entries[0]

		file := entry.File
		if entry.Type == m.ProvenanceTypeComputed {
			file = "<computed>"
		}

		symbol := getSymbolForEntry(entry)

		byFile[file] = append(byFile[file], ProvenanceItem{
			Symbol: symbol,
			Line:   entry.Line,
			Path:   path,
		})
	}

	// Convert to sorted tree
	var tree []FileTreeNode
	for file, items := range byFile {
		tree = append(tree, FileTreeNode{
			File:  file,
			Items: items,
		})
	}

	sort.Slice(tree, func(i, j int) bool {
		return tree[i].File < tree[j].File
	})

	return tree
}

// renderFileTree renders the file tree with box drawing characters.
func renderFileTree(buf *strings.Builder, tree []FileTreeNode) {
	if len(tree) == 0 {
		buf.WriteString("No provenance data available.\n")
		return
	}

	buf.WriteString("stacks/\n")

	for i, node := range tree {
		isLast := i == len(tree)-1
		connector := "├──"
		prefix := "│  "
		if isLast {
			connector = "└──"
			prefix = "   "
		}

		// File path (colored)
		buf.WriteString(connector)
		buf.WriteString(" ")
		buf.WriteString(colorize(node.File, lipgloss.Color(theme.ColorSelectedItem)))
		buf.WriteString("\n")

		// Items
		for j, item := range node.Items {
			itemLast := j == len(node.Items)-1
			itemConn := "├─"
			if itemLast {
				itemConn = "└─"
			}

			buf.WriteString(prefix)
			buf.WriteString(itemConn)
			buf.WriteString(" ")

			// Symbol (colored)
			symbol := colorize(item.Symbol, getSymbolColor(item.Symbol))
			buf.WriteString(symbol)
			buf.WriteString(" ")

			// Line number
			if item.Line > 0 {
				lineNum := fmt.Sprintf(":%d", item.Line)
				buf.WriteString(colorize(lineNum, lipgloss.Color(theme.ColorCyan)))
				buf.WriteString("   ")
			}

			// Path
			buf.WriteString(item.Path)
			buf.WriteString("\n")
		}
	}
}

// getSymbolForEntry determines which symbol to use based on provenance type.
func getSymbolForEntry(entry m.ProvenanceEntry) string {
	switch entry.Type {
	case m.ProvenanceTypeComputed:
		return SymbolComputed // ∴
	case m.ProvenanceTypeInline, m.ProvenanceTypeOverride:
		return SymbolDefined // ● (defined at this level)
	case m.ProvenanceTypeImport, m.ProvenanceTypeDefault:
		return SymbolInherited // ○ (inherited/imported)
	default:
		return SymbolInherited // ○
	}
}

// getSymbolColor returns the color for a given symbol.
func getSymbolColor(symbol string) lipgloss.Color {
	switch symbol {
	case SymbolDefined:
		return lipgloss.Color(theme.ColorDarkGray)
	case SymbolInherited:
		return lipgloss.Color(theme.ColorDarkGray)
	case SymbolComputed:
		return lipgloss.Color(theme.ColorDarkGray)
	default:
		return lipgloss.Color(theme.ColorDarkGray)
	}
}

// colorize applies a color to text using lipgloss.
func colorize(text string, color lipgloss.Color) string {
	style := lipgloss.NewStyle().Foreground(color)
	return style.Render(text)
}

// RenderSideBySide renders YAML on the left and provenance tree on the right.
func RenderSideBySide(yamlData any, ctx *m.MergeContext, atmosConfig *schema.AtmosConfiguration, leftWidth int) string {
	defer perf.Track(atmosConfig, "provenance.RenderSideBySide")()

	// Generate left side (YAML)
	yamlBytes, err := u.ConvertToYAML(yamlData)
	if err != nil {
		return fmt.Sprintf("Error rendering YAML: %v\n", err)
	}
	leftYAML := yamlBytes

	// Apply syntax highlighting to YAML
	highlighted, err := u.HighlightCodeWithConfig(atmosConfig, leftYAML, "yaml")
	if err != nil {
		// If highlighting fails, use plain YAML
		highlighted = leftYAML
	}

	// Don't filter provenance - show all available provenance data
	// The user wants to see where values came from, even if some values
	// don't have provenance (computed or from component defaults)
	rightTree := RenderTree(ctx, atmosConfig, nil)

	// Combine side-by-side
	return combineSideBySide(highlighted, rightTree, leftWidth)
}

// normalizeProvenancePath strips component inheritance prefixes from paths.
// Examples:
//   - "components.terraform.vpc-flow-logs-bucket.vars.enabled" → "vars.enabled"
//   - "terraform.vars.tags" → "vars.tags"
//   - "vars.enabled" → "vars.enabled" (already normalized)
func normalizeProvenancePath(path string) string {
	defer perf.Track(nil, "provenance.normalizeProvenancePath")()

	parts := strings.Split(path, ".")

	// Remove "components.terraform.<component>." prefix
	if len(parts) >= 3 && parts[0] == "components" && parts[1] == "terraform" {
		// Skip components.terraform.<name> - return rest
		if len(parts) > 3 {
			return strings.Join(parts[3:], ".")
		}
		return ""
	}

	// Remove "terraform." prefix
	if len(parts) >= 2 && parts[0] == "terraform" {
		return strings.Join(parts[1:], ".")
	}

	return path
}

// isMultilineScalarIndicator checks if a value indicates a multi-line YAML scalar.
func isMultilineScalarIndicator(value string) bool {
	return value == "|" || value == "|-" || value == ">" || value == ">-"
}

// extractYAMLKey extracts the key from a YAML line, handling array items.
func extractYAMLKey(trimmed string) string {
	parts := strings.SplitN(trimmed, ":", 2)
	key := strings.TrimSpace(parts[0])

	// Handle array items like "- key:"
	if strings.HasPrefix(key, "- ") {
		key = strings.TrimPrefix(key, "- ")
		key = strings.TrimSpace(key)
	}

	return key
}

// buildYAMLPath constructs a full YAML path from a stack and new key.
func buildYAMLPath(pathStack []string, key string) string {
	if len(pathStack) > 0 {
		return strings.Join(append(pathStack, key), ".")
	}
	return key
}

// getArrayIndex returns the array index for the current level.
func getArrayIndex(arrayIndexStack []int) (int, []int) {
	var arrayIndex int

	if len(arrayIndexStack) > 0 {
		arrayIndex = arrayIndexStack[len(arrayIndexStack)-1]
		newStack := make([]int, len(arrayIndexStack))
		copy(newStack, arrayIndexStack)
		newStack[len(newStack)-1]++ // Increment for next element
		return arrayIndex, newStack
	}

	arrayIndex = 0
	newStack := []int{1} // Start at 1 for next element
	return arrayIndex, newStack
}

// popStacksForIndent pops the path, indent, and array index stacks when indentation decreases.
func popStacksForIndent(indent int, pathStack []string, indentStack, arrayIndexStack []int) ([]string, []int, []int) {
	for len(indentStack) > 1 && indent <= indentStack[len(indentStack)-1] {
		pathStack = pathStack[:len(pathStack)-1]
		indentStack = indentStack[:len(indentStack)-1]
		if len(arrayIndexStack) > 0 {
			arrayIndexStack = arrayIndexStack[:len(arrayIndexStack)-1]
		}
	}
	return pathStack, indentStack, arrayIndexStack
}

// handleArrayItemLine processes a simple array item and records it.
func handleArrayItemLine(lineNum int, pathStack []string, arrayIndexStack []int, lineInfo map[int]YAMLLineInfo) []int {
	if len(pathStack) == 0 {
		return arrayIndexStack
	}

	parentKey := pathStack[len(pathStack)-1]
	arrayIndex, newStack := getArrayIndex(arrayIndexStack)

	// Build path: parent[index]
	currentPath := fmt.Sprintf("%s[%d]", parentKey, arrayIndex)

	// Record this line as an array element
	lineInfo[lineNum] = YAMLLineInfo{
		Path:           currentPath,
		IsKeyLine:      true,
		IsContinuation: false,
	}

	return newStack
}

// yamlPathState holds the state returned from handleKeyLine.
type yamlPathState struct {
	pathStack       []string
	indentStack     []int
	arrayIndexStack []int
	multilineStart  bool
	multilinePath   string
}

// handleKeyLine processes a key: value line and updates stacks.
func handleKeyLine(
	lineNum int,
	indent int,
	parts []string,
	trimmed string,
	pathStack []string,
	indentStack []int,
	arrayIndexStack []int,
	lineInfo map[int]YAMLLineInfo,
) yamlPathState {
	key := extractYAMLKey(trimmed)
	currentPath := buildYAMLPath(pathStack, key)

	// Determine value type
	value := ""
	if len(parts) > 1 {
		value = strings.TrimSpace(parts[1])
	}

	// Check for multi-line scalar indicators
	isMultilineStart := isMultilineScalarIndicator(value)

	// Record this line as a key line
	lineInfo[lineNum] = YAMLLineInfo{
		Path:           currentPath,
		IsKeyLine:      true,
		IsContinuation: false,
	}

	state := yamlPathState{
		pathStack:       pathStack,
		indentStack:     indentStack,
		arrayIndexStack: arrayIndexStack,
		multilineStart:  isMultilineStart,
		multilinePath:   currentPath,
	}

	// Push to stack if this is a parent key
	if value == "" || value == "{}" || value == "[]" || isMultilineStart {
		state.pathStack = append(state.pathStack, key)
		state.indentStack = append(state.indentStack, indent)
		// Reset array index counter for this new parent
		state.arrayIndexStack = append(state.arrayIndexStack, 0)
	}

	return state
}

// buildYAMLPathMap creates a mapping from line numbers to YAML line information.
// It parses YAML line-by-line, tracks nesting, and detects multi-line constructs.
func buildYAMLPathMap(yamlLines []string) map[int]YAMLLineInfo {
	defer perf.Track(nil, "provenance.buildYAMLPathMap")()

	lineInfo := make(map[int]YAMLLineInfo)
	pathStack := []string{}    // Track nesting context
	indentStack := []int{-1}   // Track indentation levels
	arrayIndexStack := []int{} // Track array indices at each level

	// Track multi-line values
	inMultilineValue := false
	multilineIndent := 0
	multilinePath := ""

	for lineNum, line := range yamlLines {
		// Count leading spaces (accounting for ANSI codes)
		plainLine := stripANSI(line)
		indent := len(plainLine) - len(strings.TrimLeft(plainLine, " "))
		trimmed := strings.TrimSpace(plainLine)

		// Skip empty lines or comments
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Check if we're exiting a multi-line value
		if inMultilineValue && indent <= multilineIndent {
			inMultilineValue = false
		}

		// Handle continuation lines in multi-line values
		if inMultilineValue {
			lineInfo[lineNum] = YAMLLineInfo{
				Path:           multilinePath,
				IsKeyLine:      false,
				IsContinuation: true,
			}
			continue
		}

		// Pop stack for decreased indentation
		pathStack, indentStack, arrayIndexStack = popStacksForIndent(indent, pathStack, indentStack, arrayIndexStack)

		// Handle simple array items (lines starting with "- " but no colon)
		if strings.HasPrefix(trimmed, "- ") && !strings.Contains(trimmed, ":") {
			arrayIndexStack = handleArrayItemLine(lineNum, pathStack, arrayIndexStack, lineInfo)
			continue
		}

		// Extract key from "key:" or "key: value" or "- item"
		if strings.Contains(trimmed, ":") {
			parts := strings.SplitN(trimmed, ":", 2)
			state := handleKeyLine(
				lineNum, indent, parts, trimmed, pathStack, indentStack, arrayIndexStack, lineInfo,
			)

			pathStack = state.pathStack
			indentStack = state.indentStack
			arrayIndexStack = state.arrayIndexStack
			multilinePath = state.multilinePath

			// Enter multi-line mode if this is a multi-line scalar
			if state.multilineStart {
				inMultilineValue = true
				multilineIndent = indent
			}
		}
	}

	return lineInfo
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

// formatProvenanceComment creates an inline comment for provenance.
func formatProvenanceCommentWithStackFile(entry *m.ProvenanceEntry) string {
	defer perf.Track(nil, "provenance.formatProvenanceCommentWithStackFile")()

	if entry == nil {
		return ""
	}

	// Determine symbol based on depth and type.
	var symbol string
	switch {
	case entry.Type == m.ProvenanceTypeComputed:
		symbol = SymbolComputed // ∴ (computed/templated)
	case entry.Depth == 0:
		symbol = SymbolDefined // ● (defined at this level - depth 0)
	default:
		symbol = SymbolInherited // ○ (inherited/imported - depth 1+)
	}

	file := shortenFilePath(entry.File)

	// Color code the depth based on inheritance level.
	// Depth 0-1: green, 2: yellow, 3: orange, 4+: red.
	var depthColor lipgloss.Color
	switch entry.Depth {
	case 0, 1:
		depthColor = lipgloss.Color(theme.ColorGreen)
	case 2:
		depthColor = lipgloss.Color(theme.ColorYellow)
	case 3:
		depthColor = lipgloss.Color(theme.ColorOrange)
	default: // 4+
		depthColor = lipgloss.Color(theme.ColorRed)
	}

	// Format comment parts.
	grayStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorDarkGray))
	depthStyle := lipgloss.NewStyle().Foreground(depthColor)

	// Build: "# symbol [depth] file:line" with colored depth.
	comment := fmt.Sprintf("%s %s %s %s:%d",
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
	const commentSpace = 60 // Space needed for comment (# ● file:line)

	// Check if stdout is a TTY
	if !term.IsTerminal(int(os.Stdout.Fd())) {
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

// renameImportsToImport renames the "imports" key to "import" and updates provenance paths.
// This makes the output match the stack manifest schema (which uses "import:" not "imports:").
func renameImportsToImport(data any, ctx *m.MergeContext) any {
	defer perf.Track(nil, "provenance.renameImportsToImport")()

	dataMap, ok := data.(map[string]any)
	if !ok || ctx == nil {
		return data
	}

	// Check if "imports" key exists.
	_, hasImports := dataMap["imports"]
	if !hasImports {
		return data
	}

	// Create new map with "import" instead of "imports".
	newMap := make(map[string]any, len(dataMap))
	for k, v := range dataMap {
		if k == "imports" {
			newMap["import"] = v
		} else {
			newMap[k] = v
		}
	}

	// Update provenance paths from "imports" → "import" and "imports[N]" → "import[N]".
	if ctx.HasProvenance("imports") {
		if entries := ctx.GetProvenance("imports"); entries != nil && len(entries) > 0 {
			// Use the latest entry (last in the slice).
			ctx.RecordProvenance("import", entries[len(entries)-1])
		}
	}

	// Update array element paths.
	i := 0
	for {
		oldPath := fmt.Sprintf("imports[%d]", i)
		if !ctx.HasProvenance(oldPath) {
			break
		}
		if entries := ctx.GetProvenance(oldPath); len(entries) > 0 {
			newPath := fmt.Sprintf("import[%d]", i)
			// Use the latest entry (last in the slice).
			ctx.RecordProvenance(newPath, entries[len(entries)-1])
		}
		i++
	}

	return newMap
}

// filterEmptySections removes top-level sections that have no provenance.
// This prevents displaying sections like "backend: {}" or "overrides: {}" when they
// weren't explicitly defined in any file and are just generated placeholders.
func filterEmptySections(data any, ctx *m.MergeContext) any {
	defer perf.Track(nil, "provenance.filterEmptySections")()

	dataMap, ok := data.(map[string]any)
	if !ok {
		return data
	}

	// Create a new map to hold filtered results
	filtered := make(map[string]any)

	for key, value := range dataMap {
		// Check if this key or any of its array elements have provenance.
		hasProvenance := false
		if ctx != nil {
			hasProvenance = ctx.HasProvenance(key)

			// If no direct provenance, check for array element provenance.
			if !hasProvenance {
				// Check up to 1000 array elements (reasonable limit).
				for i := 0; i < 1000; i++ {
					arrayPath := fmt.Sprintf("%s[%d]", key, i)
					if ctx.HasProvenance(arrayPath) {
						hasProvenance = true
						break
					}
				}
			}
		}

		// Keep if has provenance.
		if hasProvenance {
			filtered[key] = value
		}
	}

	return filtered
}

// RenderInlineProvenance renders YAML with provenance as inline comments.
// Deprecated: Use RenderInlineProvenanceWithStackFile instead.
func RenderInlineProvenance(yamlData any, ctx *m.MergeContext, atmosConfig *schema.AtmosConfiguration) string {
	return RenderInlineProvenanceWithStackFile(yamlData, ctx, atmosConfig, "")
}

// RenderInlineProvenanceWithStackFile renders YAML with provenance as inline comments.
// The stackFile parameter is the stack manifest file being described (e.g., "orgs/acme/plat/dev/us-east-2.yaml").
// Values from this file will be marked with ● (defined), while values from other files show ○ (inherited).
func RenderInlineProvenanceWithStackFile(yamlData any, ctx *m.MergeContext, atmosConfig *schema.AtmosConfiguration, stackFile string) string {
	defer perf.Track(atmosConfig, "provenance.RenderInlineProvenanceWithStackFile")()

	var result strings.Builder

	// Add legend at top
	legendStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorDarkGray))
	legend := "# Provenance Legend:\n" +
		"#   ● [0] Defined in parent stack\n" +
		"#   ○ [N] Inherited/imported (N levels deep)\n" +
		"#   ∴ Computed/templated\n"
	result.WriteString(legendStyle.Render(legend))
	result.WriteString("\n")

	// Rename "imports" → "import" to match stack manifest schema when provenance is displayed.
	// Also update provenance paths from "imports[N]" → "import[N]".
	yamlData = renameImportsToImport(yamlData, ctx)

	// Filter out empty sections without provenance
	filteredData := filterEmptySections(yamlData, ctx)

	// Wrap long strings to avoid horizontal scrolling.
	// Use comment column minus buffer as the threshold.
	maxLineLength := getCommentColumn() - 10
	wrappedData := u.WrapLongStrings(filteredData, maxLineLength)

	// Convert yamlData to YAML string
	yamlBytes, err := u.ConvertToYAML(wrappedData)
	if err != nil {
		return fmt.Sprintf("Error rendering YAML: %v\n", err)
	}

	// Apply syntax highlighting
	highlighted, err := u.HighlightCodeWithConfig(atmosConfig, yamlBytes, "yaml")
	if err != nil {
		// If highlighting fails, use plain YAML
		highlighted = yamlBytes
	}

	// Split into lines
	lines := strings.Split(highlighted, "\n")

	// Build path mapping
	pathMap := buildYAMLPathMap(lines)

	// Add provenance comments to each line
	commentColumn := getCommentColumn() // Dynamic based on terminal width

	for i, line := range lines {
		info, exists := pathMap[i]

		// Skip provenance for continuation lines
		if exists && info.IsContinuation {
			result.WriteString(line)
			result.WriteString("\n")
			continue
		}

		// Only add provenance if this is a key line
		if !exists || !info.IsKeyLine {
			result.WriteString(line)
			result.WriteString("\n")
			continue
		}

		// This is a key line - look up provenance
		plainLine := stripANSI(line)
		lineLen := len(plainLine)

		entry := findProvenance(ctx, info.Path)
		if entry == nil {
			// No provenance - just write the line
			result.WriteString(line)
			result.WriteString("\n")
			continue
		}

		comment := formatProvenanceCommentWithStackFile(entry)
		if comment == "" {
			result.WriteString(line)
			result.WriteString("\n")
			continue
		}

		// Add provenance comment
		if lineLen < commentColumn {
			// Line is short enough - add padding and comment on same line
			result.WriteString(line)
			padding := commentColumn - lineLen
			result.WriteString(strings.Repeat(" ", padding))
			result.WriteString(comment)
		} else {
			// Line is too long - add comment on next line indented
			result.WriteString(line)
			result.WriteString("\n")
			result.WriteString(strings.Repeat(" ", commentColumn))
			result.WriteString(comment)
		}

		result.WriteString("\n")
	}

	return result.String()
}

// wrapLine wraps a line to fit within maxWidth, preserving ANSI codes.
func wrapLine(line string, maxWidth int) []string {
	if maxWidth <= 0 {
		return []string{line}
	}

	// Strip ANSI to measure actual width
	plainText := stripANSI(line)
	if len(plainText) <= maxWidth {
		return []string{line}
	}

	// Split line into ANSI segments and text
	var wrapped []string
	var currentLine strings.Builder
	var currentPlain strings.Builder
	currentWidth := 0
	inEscape := false

	runes := []rune(line)
	for i := 0; i < len(runes); i++ {
		r := runes[i]

		// Handle ANSI escape sequences
		if r == '\x1b' {
			inEscape = true
			currentLine.WriteRune(r)
			continue
		}

		if inEscape {
			currentLine.WriteRune(r)
			if r == 'm' {
				inEscape = false
			}
			continue
		}

		// Regular character - check if we need to wrap
		if currentWidth >= maxWidth && (r == ' ' || r == '\t') {
			// Wrap at whitespace
			wrapped = append(wrapped, currentLine.String())
			currentLine.Reset()
			currentPlain.Reset()
			currentWidth = 0
			continue
		}

		currentLine.WriteRune(r)
		currentPlain.WriteRune(r)
		currentWidth++
	}

	// Add remaining content
	if currentLine.Len() > 0 {
		wrapped = append(wrapped, currentLine.String())
	}

	// If we couldn't wrap nicely, just hard-wrap at maxWidth
	if len(wrapped) == 0 && len(plainText) > maxWidth {
		wrapped = append(wrapped, line[:maxWidth])
		if len(line) > maxWidth {
			wrapped = append(wrapped, wrapLine(line[maxWidth:], maxWidth)...)
		}
	}

	return wrapped
}

// combineSideBySide combines left and right text into side-by-side layout.
func combineSideBySide(left, right string, leftWidth int) string {
	// Wrap left lines to fit within leftWidth
	var wrappedLeftLines []string
	for _, line := range strings.Split(left, "\n") {
		wrapped := wrapLine(line, leftWidth-2) // Reserve 2 chars for padding
		wrappedLeftLines = append(wrappedLeftLines, wrapped...)
	}

	rightLines := strings.Split(right, "\n")

	// Balance the lines by inserting blanks where needed
	balancedLeft, balancedRight := balanceColumns(wrappedLeftLines, rightLines)

	var buf strings.Builder

	// Header
	buf.WriteString("Configuration")
	buf.WriteString(strings.Repeat(" ", leftWidth-13))
	buf.WriteString(" │  Provenance\n")
	buf.WriteString(strings.Repeat("─", leftWidth))
	buf.WriteString("┼")
	buf.WriteString(strings.Repeat("─", 60))
	buf.WriteString("\n")

	// Combine lines
	maxLines := max(len(balancedLeft), len(balancedRight))
	for i := 0; i < maxLines; i++ {
		// Left side
		leftLine := ""
		if i < len(balancedLeft) {
			leftLine = balancedLeft[i]
		}
		buf.WriteString(leftLine)

		// Pad to left width (accounting for ANSI color codes)
		padding := leftWidth - len(stripANSI(leftLine))
		if padding > 0 {
			buf.WriteString(strings.Repeat(" ", padding))
		}

		// Separator
		buf.WriteString(" │  ")

		// Right side
		if i < len(balancedRight) {
			buf.WriteString(balancedRight[i])
		}

		buf.WriteString("\n")
	}

	return buf.String()
}

// balanceColumns aligns left and right columns by inserting blank lines.
func balanceColumns(leftLines, rightLines []string) ([]string, []string) {
	// Build aligned output
	var balancedLeft, balancedRight []string
	leftIdx, rightIdx := 0, 0

	for leftIdx < len(leftLines) || rightIdx < len(rightLines) {
		// If both have content, check if they should align
		switch {
		case leftIdx < len(leftLines) && rightIdx < len(rightLines):
			// Both sides have content - add both
			balancedLeft = append(balancedLeft, leftLines[leftIdx])
			balancedRight = append(balancedRight, rightLines[rightIdx])
			leftIdx++
			rightIdx++
		case leftIdx < len(leftLines):
			// Only left has content - add blank to right
			balancedLeft = append(balancedLeft, leftLines[leftIdx])
			balancedRight = append(balancedRight, "")
			leftIdx++
		default:
			// Only right has content - add blank to left
			balancedLeft = append(balancedLeft, "")
			balancedRight = append(balancedRight, rightLines[rightIdx])
			rightIdx++
		}
	}

	return balancedLeft, balancedRight
}

// stripANSI removes ANSI escape codes from a string for length calculation.
func stripANSI(s string) string {
	// Simple ANSI stripping - removes escape sequences
	result := ""
	inEscape := false
	for _, r := range s {
		if r == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if r == 'm' {
				inEscape = false
			}
			continue
		}
		result += string(r)
	}
	return result
}

// max returns the maximum of two integers.
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
