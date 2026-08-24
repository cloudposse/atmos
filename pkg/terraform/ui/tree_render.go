package ui

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

// spaceChar is the literal single-space string. It's extracted into a constant since it's
// reused across icon placeholders, indentation, and join separators.
const spaceChar = " "

// iconPlaceholder is used as a placeholder when no action icon is needed.
const iconPlaceholder = spaceChar

// twoSpaceIndent is the indent added for nested content (JSON lines, multi-line diffs).
const twoSpaceIndent = "  "

// newlineStr is the literal newline string, reused when splitting/joining multi-line output.
const newlineStr = "\n"

// fmtIndentedSymbolLine formats an indented "<symbol> <line>" row (used by diff rendering).
const fmtIndentedSymbolLine = "%s%s %s\n"

// RenderTree renders the tree as a string with box-drawing characters.
// Uses a two-column layout: action symbol (fixed width) | tree structure.
func (t *DependencyTree) RenderTree() string {
	return t.RenderTreeWithConfig(nil)
}

// RenderTreeWithConfig renders the tree with custom rendering configuration.
func (t *DependencyTree) RenderTreeWithConfig(config *RenderConfig) string {
	defer perf.Track(nil, "terraform.ui.DependencyTree.RenderTreeWithConfig")()

	var b strings.Builder

	// Header style.
	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorCyan)).Bold(true)

	// Render stack/component header (cyan, bold) - aligned with tree.
	fmt.Fprintf(&b, "     %s\n", headerStyle.Render(t.Stack+"/"+t.Component))

	// Render resource tree.
	renderChildren(&b, t.Root.Children, "", config)
	return b.String()
}

func renderChildren(b *strings.Builder, nodes []*TreeNode, prefix string, config *RenderConfig) {
	config = resolveRenderConfig(config)

	for i, node := range nodes {
		isLastChild := i == len(nodes)-1

		// Determine box-drawing characters.
		var connector, childPrefix string
		if isLastChild {
			connector = "└── "
			childPrefix = prefix + "    "
		} else {
			connector = "├── "
			childPrefix = prefix + "│   "
		}

		// Colorized action symbol (fixed 2-char width: symbol + space).
		symbol := colorizedActionSymbol(node.Action)

		// Build tree line: "  +  ├── resource_name"
		// Column 1: 2 spaces + symbol + 2 spaces (5 chars total for alignment)
		// Column 2: tree prefix + connector + resource address
		treeLine := config.TreeStyle.Render(prefix+connector) + node.Address

		fmt.Fprintf(b, "  %s  %s\n", symbol, treeLine)

		// Render attribute changes below the resource.
		if len(node.Changes) > 0 {
			renderAttributeChanges(b, node.Changes, childPrefix, config)
		}

		// Render children.
		if len(node.Children) > 0 {
			renderChildren(b, node.Children, childPrefix, config)
		}

		// Add blank line after resource block if not compact mode.
		if !config.Compact && !isLastChild {
			b.WriteString(newlineStr)
		}
	}
}

// RenderConfig holds configuration for tree rendering, including display options and the
// styles used to render create/update/delete attribute changes.
type RenderConfig struct {
	// ShowAttributeBar shows a thick ┃ bar alongside attributes.
	ShowAttributeBar bool
	// Compact removes blank lines between resources.
	Compact bool
	// MaxLines controls collapsing of large JSON values (0 = show all).
	MaxLines int

	// CreateStyle, UpdateStyle, DeleteStyle, DimStyle, TreeStyle, and BarStyle are the
	// styles used when rendering the tree and attribute changes. Populated with defaults
	// by resolveRenderConfig when not explicitly set.
	CreateStyle lipgloss.Style
	UpdateStyle lipgloss.Style
	DeleteStyle lipgloss.Style
	DimStyle    lipgloss.Style
	TreeStyle   lipgloss.Style
	BarStyle    lipgloss.Style
}

// resolveRenderConfig returns a fully-populated RenderConfig, preserving any caller-provided
// display options (Compact, ShowAttributeBar, MaxLines) while filling in default styles.
func resolveRenderConfig(config *RenderConfig) *RenderConfig {
	resolved := &RenderConfig{
		CreateStyle: lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorGreen)),
		UpdateStyle: lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorYellow)),
		DeleteStyle: lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorRed)),
		DimStyle:    lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorGray)),
		TreeStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorGray)),
		BarStyle:    lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorDarkGray)),
	}
	if config != nil {
		resolved.ShowAttributeBar = config.ShowAttributeBar
		resolved.Compact = config.Compact
		resolved.MaxLines = config.MaxLines

		overrideStyle(&resolved.CreateStyle, &config.CreateStyle)
		overrideStyle(&resolved.UpdateStyle, &config.UpdateStyle)
		overrideStyle(&resolved.DeleteStyle, &config.DeleteStyle)
		overrideStyle(&resolved.DimStyle, &config.DimStyle)
		overrideStyle(&resolved.TreeStyle, &config.TreeStyle)
		overrideStyle(&resolved.BarStyle, &config.BarStyle)
	}
	return resolved
}

// overrideStyle replaces *dst with *caller in place when caller is non-zero.
//
// A lipgloss.Style isn't comparable with == (it embeds a func field).
// Its String() method renders the style's own unset text value, so it always returns "" no matter which properties are set (Foreground, Bold, and so on), which makes it useless for a zero check.
// Comparing the struct via reflect.DeepEqual against the Go zero value works instead.
func overrideStyle(dst, caller *lipgloss.Style) {
	if !reflect.DeepEqual(*caller, lipgloss.Style{}) {
		*dst = *caller
	}
}

// diffStyles bundles the create/delete styles used when rendering a line-by-line diff.
type diffStyles struct {
	Create lipgloss.Style
	Delete lipgloss.Style
}

// diffStylesFromConfig extracts the create/delete styles from a resolved RenderConfig.
func diffStylesFromConfig(config *RenderConfig) *diffStyles {
	return &diffStyles{Create: config.CreateStyle, Delete: config.DeleteStyle}
}

// attrRenderContext bundles the layout (indent/bar) and style configuration shared across
// all attribute-change renderers for a single tree node.
type attrRenderContext struct {
	Indent string
	Bar    string
	Config *RenderConfig
}

// attrStyleInfo bundles the per-change key style and "forces replacement" annotation,
// computed once and shared by every rendering branch for that change.
type attrStyleInfo struct {
	KeyStyle   lipgloss.Style
	Annotation string
}

// attributeWidths holds the column widths used to align rendered attribute rows.
type attributeWidths struct {
	Key    int
	OldVal int
}

// attrRenderMeta bundles positional/formatting metadata for rendering a single complex
// (map/array) attribute change.
type attrRenderMeta struct {
	Indent     string
	Bar        string
	Annotation string
	KeyStyle   lipgloss.Style
	Config     *RenderConfig
}

// attributeKeyStyle returns the style used for an attribute key, based on the change type:
// green for a new attribute, red for a deleted attribute, yellow for an update (including
// unknown/computed values).
func attributeKeyStyle(change *AttributeChange, config *RenderConfig) lipgloss.Style {
	switch {
	case change.Before == nil && change.After != nil:
		return config.CreateStyle
	case change.Before != nil && change.After == nil && !change.Unknown:
		return config.DeleteStyle
	default:
		return config.UpdateStyle
	}
}

// forcesReplacementAnnotation renders the "# forces replacement" annotation for a change,
// or an empty string if the change doesn't force replacement.
func forcesReplacementAnnotation(change *AttributeChange) string {
	if !change.ForcesReplacement {
		return ""
	}
	replaceStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorOrange))
	return spaceChar + replaceStyle.Render("# forces replacement")
}

// formattedAttributeChange precomputes the rendering-relevant details for a single
// attribute change, avoiding repeated formatting work in the main render loop.
type formattedAttributeChange struct {
	change    *AttributeChange
	oldVal    string
	newVal    string
	isMulti   bool
	isComplex bool
}

// formatOneAttributeChange precomputes the rendering-relevant details for a single change.
func formatOneAttributeChange(change *AttributeChange) formattedAttributeChange {
	_, beforeIsMultiline := getRawStringValue(change.Before, change.Sensitive)
	afterStr, afterIsMultiline := getRawStringValue(change.After, change.Sensitive)
	if change.Unknown {
		afterStr = "(known after apply)"
		afterIsMultiline = false
	}

	isMulti := beforeIsMultiline || afterIsMultiline
	isComplex := isComplexValue(change.Before) || isComplexValue(change.After)

	var oldVal, newVal string
	if !isMulti && !isComplex {
		oldVal = formatSimpleValue(change.Before, change.Sensitive)
		newVal = afterStr
		if newVal == "" {
			newVal = formatSimpleValue(change.After, change.Sensitive)
		}
	}

	return formattedAttributeChange{
		change:    change,
		oldVal:    oldVal,
		newVal:    newVal,
		isMulti:   isMulti,
		isComplex: isComplex,
	}
}

// precomputeAttributeFormatting formats every change once up front, computing the key and
// old-value column widths needed for aligned single-line rendering.
func precomputeAttributeFormatting(changes []*AttributeChange) (formatted []formattedAttributeChange, maxKeyWidth, maxOldValWidth int) {
	formatted = make([]formattedAttributeChange, len(changes))
	for i, change := range changes {
		if len(change.Key) > maxKeyWidth {
			maxKeyWidth = len(change.Key)
		}

		fc := formatOneAttributeChange(change)
		if len(fc.oldVal) > maxOldValWidth {
			maxOldValWidth = len(fc.oldVal)
		}
		formatted[i] = fc
	}

	return formatted, maxKeyWidth, maxOldValWidth
}

// renderAttributeChanges renders attribute-level changes with clean indentation.
// Uses simple indentation instead of tree continuation characters for cleaner output.
func renderAttributeChanges(b *strings.Builder, changes []*AttributeChange, prefix string, config *RenderConfig) {
	config = resolveRenderConfig(config)

	// Calculate base indent for attributes (aligned with tree structure).
	// Base indent: 5 spaces (for "  ●  ") + prefix display width + 4 (for "├── ").
	// Use display width, not byte length: prefix carries multi-byte box-drawing
	// characters (e.g. "│", 3 bytes / 1 cell) that would otherwise overcount.
	baseIndent := strings.Repeat(spaceChar, 5+lipgloss.Width(prefix)+4)

	// Build attribute bar if enabled.
	var attrBar string
	if config.ShowAttributeBar {
		attrBar = config.BarStyle.Render("┃") + spaceChar
	}

	formatted, maxKeyWidth, maxOldValWidth := precomputeAttributeFormatting(changes)
	ctx := attrRenderContext{Indent: baseIndent, Bar: attrBar, Config: config}
	widths := attributeWidths{Key: maxKeyWidth, OldVal: maxOldValWidth}

	for _, fc := range formatted {
		renderOneAttributeChange(b, fc, ctx, widths)
	}
}

// renderOneAttributeChange renders a single (precomputed) attribute change: complex
// (map/array) values as pretty-printed JSON, multi-line values as a line-by-line diff, and
// everything else as a single aligned "key  old → new" row.
func renderOneAttributeChange(b *strings.Builder, fc formattedAttributeChange, ctx attrRenderContext, widths attributeWidths) {
	info := &attrStyleInfo{
		KeyStyle:   attributeKeyStyle(fc.change, ctx.Config),
		Annotation: forcesReplacementAnnotation(fc.change),
	}

	switch {
	case fc.isComplex:
		meta := &attrRenderMeta{Indent: ctx.Indent, Bar: ctx.Bar, Annotation: info.Annotation, KeyStyle: info.KeyStyle, Config: ctx.Config}
		renderComplexAttributeChange(b, fc.change, meta, diffStylesFromConfig(ctx.Config))
	case fc.isMulti:
		renderMultilineAttributeChange(b, fc.change, ctx, info)
	default:
		renderSingleLineAttributeChange(b, fc, ctx, info, widths)
	}
}

// renderSingleLineAttributeChange renders "key  old → new" on a single aligned row.
func renderSingleLineAttributeChange(b *strings.Builder, fc formattedAttributeChange, ctx attrRenderContext, info *attrStyleInfo, widths attributeWidths) {
	paddedKey := fmt.Sprintf("%-*s", widths.Key, fc.change.Key)
	paddedOldVal := fmt.Sprintf("%-*s", widths.OldVal, fc.oldVal)

	fmt.Fprintf(
		b, "%s%s%s %s  %s  %s%s\n",
		ctx.Indent,
		ctx.Bar,
		info.KeyStyle.Render(paddedKey),
		ctx.Config.DimStyle.Render(paddedOldVal),
		ctx.Config.DimStyle.Render("→"),
		fc.newVal,
		info.Annotation,
	)
}

// renderMultilineAttributeChange renders a multi-line string value: the key on its own
// line, followed by a line-by-line diff (or a pure addition/deletion) of the content.
func renderMultilineAttributeChange(b *strings.Builder, change *AttributeChange, ctx attrRenderContext, info *attrStyleInfo) {
	fmt.Fprintf(
		b, "%s%s%s%s\n",
		ctx.Indent,
		ctx.Bar,
		info.KeyStyle.Render(change.Key),
		info.Annotation,
	)

	beforeStr, _ := getRawStringValue(change.Before, change.Sensitive)
	afterStr, _ := getRawStringValue(change.After, change.Sensitive)
	if change.Unknown {
		afterStr = "(known after apply)"
	}

	hasBeforeContent := beforeStr != "" && beforeStr != "(none)"
	hasAfterContent := afterStr != "" && afterStr != "(none)"

	// Render diff based on what content we have.
	contentIndent := ctx.Indent + twoSpaceIndent
	if ctx.Bar != "" {
		contentIndent = ctx.Indent + ctx.Bar + twoSpaceIndent
	}

	styles := diffStylesFromConfig(ctx.Config)
	switch {
	case hasBeforeContent && hasAfterContent:
		renderMultilineDiffSimple(b, beforeStr, afterStr, contentIndent, styles)
	case hasBeforeContent:
		renderMultilineValueSimple(b, beforeStr, contentIndent, "-", &styles.Delete)
	case hasAfterContent:
		renderMultilineValueSimple(b, afterStr, contentIndent, "+", &styles.Create)
	}
}

// renderComplexAttributeChange renders a complex attribute (map/array) with pretty-printed JSON.
func renderComplexAttributeChange(b *strings.Builder, change *AttributeChange, meta *attrRenderMeta, styles *diffStyles) {
	// Write key line.
	fmt.Fprintf(
		b, "%s%s%s%s\n",
		meta.Indent,
		meta.Bar,
		meta.KeyStyle.Render(change.Key),
		meta.Annotation,
	)

	// Format values as JSON lines.
	beforeLines := formatComplexValue(change.Before, nil)
	afterLines := formatComplexValue(change.After, nil)

	// Apply collapsing if configured.
	maxLines := 0
	if meta.Config != nil {
		maxLines = meta.Config.MaxLines
	}
	beforeLines = collapseIfNeeded(beforeLines, maxLines)
	afterLines = collapseIfNeeded(afterLines, maxLines)

	// Content indent for JSON lines.
	contentIndent := meta.Indent + twoSpaceIndent
	if meta.Bar != "" {
		contentIndent = meta.Indent + meta.Bar + twoSpaceIndent
	}

	// Render based on what content we have.
	switch {
	case len(beforeLines) > 0 && len(afterLines) > 0:
		// Both have values - show diff.
		renderJSONDiff(b, beforeLines, afterLines, contentIndent, styles)
	case len(beforeLines) > 0:
		// Only before (deletion).
		for _, line := range beforeLines {
			fmt.Fprintf(b, fmtIndentedSymbolLine, contentIndent, styles.Delete.Render("-"), line)
		}
	case len(afterLines) > 0:
		// Only after (creation).
		for _, line := range afterLines {
			fmt.Fprintf(b, fmtIndentedSymbolLine, contentIndent, styles.Create.Render("+"), line)
		}
	}
}

// linesMatch checks if both indices are valid and lines match.
func linesMatch(before, after []string, i, j int) bool {
	return i < len(before) && j < len(after) && before[i] == after[j]
}

// diffCursor tracks the current read position in the before/after line slices while
// collecting a run of changed lines.
type diffCursor struct {
	I, J int
}

// collectChanges collects deleted and added lines until the next matching line.
func collectChanges(before, after []string, start diffCursor) (deleted, added []string, next diffCursor) {
	i, j := start.I, start.J
	for i < len(before) || j < len(after) {
		if linesMatch(before, after, i, j) {
			break
		}
		if i < len(before) {
			deleted = append(deleted, before[i])
			i++
		}
		if j < len(after) {
			added = append(added, after[j])
			j++
		}
	}
	return deleted, added, diffCursor{I: i, J: j}
}

// transformLines applies transform to every line, returning lines unchanged if transform is nil.
func transformLines(lines []string, transform func(string) string) []string {
	if transform == nil {
		return lines
	}
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = transform(line)
	}
	return out
}

// renderDiffLines outputs deleted and added lines to the builder.
func renderDiffLines(b *strings.Builder, deleted, added []string, indent string, styles *diffStyles) {
	for _, line := range deleted {
		fmt.Fprintf(b, fmtIndentedSymbolLine, indent, styles.Delete.Render("-"), line)
	}
	for _, line := range added {
		fmt.Fprintf(b, fmtIndentedSymbolLine, indent, styles.Create.Render("+"), line)
	}
}

// renderJSONDiff renders a line-by-line diff of JSON content.
func renderJSONDiff(b *strings.Builder, beforeLines, afterLines []string, indent string, styles *diffStyles) {
	cursor := diffCursor{}
	for cursor.I < len(beforeLines) || cursor.J < len(afterLines) {
		if linesMatch(beforeLines, afterLines, cursor.I, cursor.J) {
			fmt.Fprintf(b, "%s  %s\n", indent, beforeLines[cursor.I])
			cursor.I++
			cursor.J++
			continue
		}
		var deleted, added []string
		deleted, added, cursor = collectChanges(beforeLines, afterLines, cursor)
		renderDiffLines(b, deleted, added, indent, styles)
	}
}

// makeTruncator returns a function that truncates lines to maxWidth.
func makeTruncator(maxWidth int) func(string) string {
	return func(line string) string {
		if maxWidth > 3 && len(line) > maxWidth {
			return line[:maxWidth-3] + "..."
		}
		return line
	}
}

// renderMultilineDiffSimple renders a simple line-by-line diff with clean indentation.
func renderMultilineDiffSimple(b *strings.Builder, before, after, indent string, styles *diffStyles) {
	maxWidth := getMaxLineWidth()
	beforeLines := strings.Split(before, newlineStr)
	afterLines := strings.Split(after, newlineStr)
	truncateLine := makeTruncator(maxWidth)

	cursor := diffCursor{}
	for cursor.I < len(beforeLines) || cursor.J < len(afterLines) {
		if linesMatch(beforeLines, afterLines, cursor.I, cursor.J) {
			fmt.Fprintf(b, "%s  %s\n", indent, truncateLine(beforeLines[cursor.I]))
			cursor.I++
			cursor.J++
			continue
		}
		var deleted, added []string
		deleted, added, cursor = collectChanges(beforeLines, afterLines, cursor)
		renderDiffLines(b, transformLines(deleted, truncateLine), transformLines(added, truncateLine), indent, styles)
	}
}

// renderMultilineValueSimple renders multi-line content with clean indentation.
func renderMultilineValueSimple(b *strings.Builder, content, indent, symbol string, symbolStyle *lipgloss.Style) {
	maxWidth := getMaxLineWidth()
	lines := strings.Split(content, newlineStr)

	for _, line := range lines {
		if maxWidth > 3 && len(line) > maxWidth {
			line = line[:maxWidth-3] + "..."
		}
		fmt.Fprintf(b, fmtIndentedSymbolLine, indent, symbolStyle.Render(symbol), line)
	}
}

func colorizedActionSymbol(action string) string {
	createStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorGreen))
	updateStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorYellow))
	deleteStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorRed))
	readStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorCyan))
	replaceStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorOrange)) // Orange for replace (delete+create).

	// Use colored dots (●) for all actions with different colors:
	// - Green: create
	// - Yellow: update/change in place
	// - Red: delete
	// - Orange: replace/recreate
	// - Cyan: read/refresh
	switch action {
	case "create":
		return createStyle.Render(theme.IconActive)
	case "update":
		return updateStyle.Render(theme.IconActive)
	case "delete":
		return deleteStyle.Render(theme.IconActive)
	case "replace":
		return replaceStyle.Render(theme.IconActive)
	case "read":
		return readStyle.Render(theme.IconActive)
	case "no-op":
		return iconPlaceholder
	default:
		return iconPlaceholder
	}
}

// GetChangeSummary returns a summary of changes from the tree.
func (t *DependencyTree) GetChangeSummary() (add, change, remove int) {
	defer perf.Track(nil, "terraform.ui.DependencyTree.GetChangeSummary")()

	countActions(t.Root, &add, &change, &remove)
	return add, change, remove
}

func countActions(node *TreeNode, add, change, remove *int) {
	if node == nil {
		return
	}

	switch node.Action {
	case "create":
		*add++
	case "update":
		*change++
	case "delete":
		*remove++
	case "replace":
		// Replace counts as both add and remove since the resource is destroyed and recreated.
		*add++
		*remove++
	}

	for _, child := range node.Children {
		countActions(child, add, change, remove)
	}
}

// noChangesBadge renders the "NO CHANGES" badge shown when a plan has no changes.
func noChangesBadge() string {
	return lipgloss.NewStyle().
		Background(lipgloss.Color(theme.ColorDarkGray)).
		Foreground(lipgloss.Color(theme.ColorWhite)).
		Bold(true).
		Padding(0, 1).
		Render("NO CHANGES")
}

// changeBadge renders a single "<count> <LABEL>" badge with a background color and a
// contrasting, accessible text color.
func changeBadge(bgColor, text string) string {
	return lipgloss.NewStyle().
		Background(lipgloss.Color(bgColor)).
		Foreground(lipgloss.Color(getContrastTextColor(bgColor))).
		Bold(true).
		Padding(0, 1).
		Render(text)
}

// buildChangeBadges renders one badge per non-zero change count.
func buildChangeBadges(add, change, remove int) []string {
	var badges []string
	if add > 0 {
		badges = append(badges, changeBadge(theme.ColorGreen, fmt.Sprintf("%d ADD", add)))
	}
	if change > 0 {
		badges = append(badges, changeBadge(theme.ColorYellow, fmt.Sprintf("%d CHANGE", change)))
	}
	if remove > 0 {
		badges = append(badges, changeBadge(theme.ColorRed, fmt.Sprintf("%d DELETE", remove)))
	}
	return badges
}

// RenderChangeSummaryBadges renders a badge-style change summary.
// Shows "NO CHANGES" badge if all counts are zero.
// Format: "  1 ADD 2 CHANGE 1 DELETE" with colored badges (green/yellow/red backgrounds).
func RenderChangeSummaryBadges(add, change, remove int) string {
	defer perf.Track(nil, "terraform.ui.RenderChangeSummaryBadges")()

	badges := []string{noChangesBadge()}
	if add > 0 || change > 0 || remove > 0 {
		badges = buildChangeBadges(add, change, remove)
	}

	// Join badges with a space, add blank line above and below, and indent 2 spaces.
	return "\n  " + strings.Join(badges, spaceChar) + "\n\n"
}
