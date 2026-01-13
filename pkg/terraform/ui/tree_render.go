package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

// RenderTree renders the tree as a string with box-drawing characters.
// Uses a two-column layout: action symbol (fixed width) | tree structure.
func (t *DependencyTree) RenderTree() string {
	return t.RenderTreeWithConfig(nil)
}

// RenderTreeWithConfig renders the tree with custom rendering configuration.
func (t *DependencyTree) RenderTreeWithConfig(config *RenderConfig) string {
	defer perf.Track(nil, "terraform.ui.DependencyTree.RenderTreeWithConfig")()

	var b strings.Builder

	// Styles.
	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorCyan)).Bold(true)
	treeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorGray)) // Dark gray for branches.

	// Render stack/component header (cyan, bold) - aligned with tree.
	b.WriteString(fmt.Sprintf("     %s\n", headerStyle.Render(t.Stack+"/"+t.Component)))

	// Render resource tree.
	renderChildren(&b, t.Root.Children, "", treeStyle, config)
	return b.String()
}

func renderChildren(b *strings.Builder, nodes []*TreeNode, prefix string, treeStyle lipgloss.Style, config *RenderConfig) {
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
		treeLine := treeStyle.Render(prefix+connector) + node.Address

		b.WriteString(fmt.Sprintf("  %s  %s\n", symbol, treeLine))

		// Render attribute changes below the resource.
		if len(node.Changes) > 0 {
			renderAttributeChanges(b, node.Changes, childPrefix, len(node.Children) > 0 || !isLastChild, treeStyle, config)
		}

		// Render children.
		if len(node.Children) > 0 {
			renderChildren(b, node.Children, childPrefix, treeStyle, config)
		}

		// Add blank line after resource block if not compact mode.
		if config != nil && !config.Compact && !isLastChild {
			b.WriteString("\n")
		}
	}
}

// RenderConfig holds configuration for tree rendering.
type RenderConfig struct {
	// ShowAttributeBar shows a thick ┃ bar alongside attributes.
	ShowAttributeBar bool
	// Compact removes blank lines between resources.
	Compact bool
	// MaxLines controls collapsing of large JSON values (0 = show all).
	MaxLines int
}

// renderAttributeChanges renders attribute-level changes with clean indentation.
// Uses simple indentation instead of tree continuation characters for cleaner output.
func renderAttributeChanges(b *strings.Builder, changes []*AttributeChange, prefix string, hasMoreContent bool, treeStyle lipgloss.Style, config *RenderConfig) {
	// Styles for keys only (values are not colorized).
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorGray))
	createStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorGreen))
	updateStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorYellow))
	deleteStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorRed))
	barStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorDarkGray))

	// Calculate base indent for attributes (aligned with tree structure).
	// Base indent: 5 spaces (for "  ●  ") + prefix length + 4 (for "├── ").
	baseIndent := strings.Repeat(" ", 5+len(prefix)+4)

	// Build attribute bar if enabled.
	var attrBar string
	if config != nil && config.ShowAttributeBar {
		attrBar = barStyle.Render("┃") + " "
	}

	// Calculate max key width for alignment.
	maxKeyWidth := 0
	for _, change := range changes {
		if len(change.Key) > maxKeyWidth {
			maxKeyWidth = len(change.Key)
		}
	}

	// Pre-compute all formatted values for column width calculation.
	type formattedChange struct {
		change    *AttributeChange
		oldVal    string
		newVal    string
		isMulti   bool
		isComplex bool
		beforeML  bool
		afterML   bool
	}
	formatted := make([]formattedChange, len(changes))

	maxOldValWidth := 0
	for i, change := range changes {
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
			if len(oldVal) > maxOldValWidth {
				maxOldValWidth = len(oldVal)
			}
		}

		formatted[i] = formattedChange{
			change:    change,
			oldVal:    oldVal,
			newVal:    newVal,
			isMulti:   isMulti,
			isComplex: isComplex,
			beforeML:  beforeIsMultiline,
			afterML:   afterIsMultiline,
		}
	}

	for _, fc := range formatted {
		change := fc.change

		// Determine key style based on change type (color indicates change type).
		// - Green: new attribute (before=nil, after!=nil)
		// - Red: deleted attribute (before!=nil, after=nil, NOT unknown)
		// - Yellow: updated attribute (both have values, or unknown computed value)
		var keyStyle lipgloss.Style
		if change.Before == nil && change.After != nil {
			keyStyle = createStyle
		} else if change.Before != nil && change.After == nil && !change.Unknown {
			keyStyle = deleteStyle
		} else {
			keyStyle = updateStyle
		}

		// Pad key for alignment.
		paddedKey := fmt.Sprintf("%-*s", maxKeyWidth, change.Key)

		// Build "# forces replacement" annotation if applicable.
		var forcesReplacementAnnotation string
		if change.ForcesReplacement {
			replaceStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorOrange))
			forcesReplacementAnnotation = " " + replaceStyle.Render("# forces replacement")
		}

		// Handle complex JSON values (maps, arrays).
		if fc.isComplex {
			renderComplexAttributeChange(b, change, baseIndent, attrBar, keyStyle, dimStyle, createStyle, deleteStyle, forcesReplacementAnnotation, config)
			continue
		}

		// Check if we need multi-line rendering.
		if fc.isMulti {
			// Multi-line rendering: show key on first line, then each value line.
			b.WriteString(fmt.Sprintf("%s%s%s%s\n",
				baseIndent,
				attrBar,
				keyStyle.Render(paddedKey),
				forcesReplacementAnnotation,
			))

			beforeStr, _ := getRawStringValue(change.Before, change.Sensitive)
			afterStr, _ := getRawStringValue(change.After, change.Sensitive)
			if change.Unknown {
				afterStr = "(known after apply)"
			}

			hasBeforeContent := beforeStr != "" && beforeStr != "(none)"
			hasAfterContent := afterStr != "" && afterStr != "(none)"

			// Render diff based on what content we have.
			contentIndent := baseIndent + "  "
			if attrBar != "" {
				contentIndent = baseIndent + attrBar + "  "
			}
			if hasBeforeContent && hasAfterContent {
				renderMultilineDiffSimple(b, beforeStr, afterStr, contentIndent, createStyle, deleteStyle, config)
			} else if hasBeforeContent {
				renderMultilineValueSimple(b, beforeStr, contentIndent, "-", deleteStyle, config)
			} else if hasAfterContent {
				renderMultilineValueSimple(b, afterStr, contentIndent, "+", createStyle, config)
			}
		} else {
			// Single-line rendering: old → new on same line with aligned columns.
			paddedOldVal := fmt.Sprintf("%-*s", maxOldValWidth, fc.oldVal)

			b.WriteString(fmt.Sprintf("%s%s%s %s  %s  %s%s\n",
				baseIndent,
				attrBar,
				keyStyle.Render(paddedKey),
				dimStyle.Render(paddedOldVal),
				dimStyle.Render("→"),
				fc.newVal,
				forcesReplacementAnnotation,
			))
		}
	}
}

// renderComplexAttributeChange renders a complex attribute (map/array) with pretty-printed JSON.
func renderComplexAttributeChange(b *strings.Builder, change *AttributeChange, baseIndent, attrBar string,
	keyStyle, dimStyle, createStyle, deleteStyle lipgloss.Style, annotation string, config *RenderConfig,
) {
	// Write key line.
	b.WriteString(fmt.Sprintf("%s%s%s%s\n",
		baseIndent,
		attrBar,
		keyStyle.Render(change.Key),
		annotation,
	))

	// Format values as JSON lines.
	beforeLines := formatComplexValue(change.Before, nil)
	afterLines := formatComplexValue(change.After, nil)

	// Apply collapsing if configured.
	maxLines := 0
	if config != nil {
		maxLines = config.MaxLines
	}
	beforeLines = collapseIfNeeded(beforeLines, maxLines)
	afterLines = collapseIfNeeded(afterLines, maxLines)

	// Content indent for JSON lines.
	contentIndent := baseIndent + "  "
	if attrBar != "" {
		contentIndent = baseIndent + attrBar + "  "
	}

	// Render based on what content we have.
	if len(beforeLines) > 0 && len(afterLines) > 0 {
		// Both have values - show diff.
		renderJSONDiff(b, beforeLines, afterLines, contentIndent, createStyle, deleteStyle, config)
	} else if len(beforeLines) > 0 {
		// Only before (deletion).
		for _, line := range beforeLines {
			b.WriteString(fmt.Sprintf("%s%s %s\n", contentIndent, deleteStyle.Render("-"), line))
		}
	} else if len(afterLines) > 0 {
		// Only after (creation).
		for _, line := range afterLines {
			b.WriteString(fmt.Sprintf("%s%s %s\n", contentIndent, createStyle.Render("+"), line))
		}
	}
}

// renderJSONDiff renders a line-by-line diff of JSON content.
func renderJSONDiff(b *strings.Builder, beforeLines, afterLines []string, indent string,
	createStyle, deleteStyle lipgloss.Style, config *RenderConfig,
) {
	i, j := 0, 0
	for i < len(beforeLines) || j < len(afterLines) {
		if i < len(beforeLines) && j < len(afterLines) && beforeLines[i] == afterLines[j] {
			// Unchanged line.
			b.WriteString(fmt.Sprintf("%s  %s\n", indent, beforeLines[i]))
			i++
			j++
		} else {
			// Changed lines - collect and group.
			var deleted, added []string
			for i < len(beforeLines) || j < len(afterLines) {
				if i < len(beforeLines) && j < len(afterLines) && beforeLines[i] == afterLines[j] {
					break
				}
				if i < len(beforeLines) {
					deleted = append(deleted, beforeLines[i])
					i++
				}
				if j < len(afterLines) {
					added = append(added, afterLines[j])
					j++
				}
			}

			// Output deleted first, then added.
			for _, line := range deleted {
				b.WriteString(fmt.Sprintf("%s%s %s\n", indent, deleteStyle.Render("-"), line))
			}
			for _, line := range added {
				b.WriteString(fmt.Sprintf("%s%s %s\n", indent, createStyle.Render("+"), line))
			}
		}
	}
}

// renderMultilineDiffSimple renders a simple line-by-line diff with clean indentation.
func renderMultilineDiffSimple(b *strings.Builder, before, after, indent string,
	createStyle, deleteStyle lipgloss.Style, config *RenderConfig,
) {
	maxWidth := getMaxLineWidth()
	beforeLines := strings.Split(before, "\n")
	afterLines := strings.Split(after, "\n")

	// Truncate helper.
	truncateLine := func(line string) string {
		if len(line) > maxWidth {
			return line[:maxWidth-3] + "..."
		}
		return line
	}

	i, j := 0, 0
	for i < len(beforeLines) || j < len(afterLines) {
		if i < len(beforeLines) && j < len(afterLines) && beforeLines[i] == afterLines[j] {
			b.WriteString(fmt.Sprintf("%s  %s\n", indent, truncateLine(beforeLines[i])))
			i++
			j++
		} else {
			var deleted, added []string
			for i < len(beforeLines) || j < len(afterLines) {
				if i < len(beforeLines) && j < len(afterLines) && beforeLines[i] == afterLines[j] {
					break
				}
				if i < len(beforeLines) {
					deleted = append(deleted, beforeLines[i])
					i++
				}
				if j < len(afterLines) {
					added = append(added, afterLines[j])
					j++
				}
			}

			for _, line := range deleted {
				b.WriteString(fmt.Sprintf("%s%s %s\n", indent, deleteStyle.Render("-"), truncateLine(line)))
			}
			for _, line := range added {
				b.WriteString(fmt.Sprintf("%s%s %s\n", indent, createStyle.Render("+"), truncateLine(line)))
			}
		}
	}
}

// renderMultilineValueSimple renders multi-line content with clean indentation.
func renderMultilineValueSimple(b *strings.Builder, content, indent, symbol string,
	symbolStyle lipgloss.Style, config *RenderConfig,
) {
	maxWidth := getMaxLineWidth()
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		if len(line) > maxWidth {
			line = line[:maxWidth-3] + "..."
		}
		b.WriteString(fmt.Sprintf("%s%s %s\n", indent, symbolStyle.Render(symbol), line))
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
		return " "
	default:
		return " "
	}
}

// GetChangeSummary returns a summary of changes from the tree.
func (t *DependencyTree) GetChangeSummary() (add, change, remove int) {
	defer perf.Track(nil, "terraform.ui.DependencyTree.GetChangeSummary")()

	countActions(t.Root, &add, &change, &remove)
	return
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

// RenderChangeSummaryBadges renders a badge-style change summary.
// Shows "NO CHANGES" badge if all counts are zero.
// Format: "  1 ADD 2 CHANGE 1 DELETE" with colored badges (green/yellow/red backgrounds).
func RenderChangeSummaryBadges(add, change, remove int) string {
	defer perf.Track(nil, "terraform.ui.RenderChangeSummaryBadges")()

	var badges []string

	// If no changes, show a "NO CHANGES" badge.
	if add == 0 && change == 0 && remove == 0 {
		noChangesBadge := lipgloss.NewStyle().
			Background(lipgloss.Color(theme.ColorDarkGray)).
			Foreground(lipgloss.Color(theme.ColorWhite)).
			Bold(true).
			Padding(0, 1).
			Render("NO CHANGES")
		badges = append(badges, noChangesBadge)
	} else {
		// Badge styles with background colors and contrasting text.
		if add > 0 {
			addBadge := lipgloss.NewStyle().
				Background(lipgloss.Color(theme.ColorGreen)).
				Foreground(lipgloss.Color(getContrastTextColor(theme.ColorGreen))).
				Bold(true).
				Padding(0, 1).
				Render(fmt.Sprintf("%d ADD", add))
			badges = append(badges, addBadge)
		}

		if change > 0 {
			changeBadge := lipgloss.NewStyle().
				Background(lipgloss.Color(theme.ColorYellow)).
				Foreground(lipgloss.Color(getContrastTextColor(theme.ColorYellow))).
				Bold(true).
				Padding(0, 1).
				Render(fmt.Sprintf("%d CHANGE", change))
			badges = append(badges, changeBadge)
		}

		if remove > 0 {
			removeBadge := lipgloss.NewStyle().
				Background(lipgloss.Color(theme.ColorRed)).
				Foreground(lipgloss.Color(getContrastTextColor(theme.ColorRed))).
				Bold(true).
				Padding(0, 1).
				Render(fmt.Sprintf("%d DELETE", remove))
			badges = append(badges, removeBadge)
		}
	}

	// Join badges with a space, add blank line above and below, and indent 2 spaces.
	return "\n  " + strings.Join(badges, " ") + "\n\n"
}

// defaultTreeStyle returns the default tree branch style (for testing).
func defaultTreeStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorGray))
}
