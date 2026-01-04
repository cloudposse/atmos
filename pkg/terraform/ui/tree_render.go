package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/cloudposse/atmos/pkg/ui/theme"
)

// RenderTree renders the tree as a string with box-drawing characters.
// Uses a two-column layout: action symbol (fixed width) | tree structure.
func (t *DependencyTree) RenderTree() string {
	var b strings.Builder

	// Styles.
	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorCyan)).Bold(true)
	treeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorGray)) // Dark gray for branches.

	// Render stack/component header (cyan, bold) - aligned with tree.
	b.WriteString(fmt.Sprintf("     %s\n", headerStyle.Render(t.Stack+"/"+t.Component)))

	// Render resource tree.
	renderChildren(&b, t.Root.Children, "", treeStyle)
	return b.String()
}

func renderChildren(b *strings.Builder, nodes []*TreeNode, prefix string, treeStyle lipgloss.Style) {
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
			renderAttributeChanges(b, node.Changes, childPrefix, len(node.Children) > 0 || !isLastChild, treeStyle)
		}

		// Render children.
		if len(node.Children) > 0 {
			renderChildren(b, node.Children, childPrefix, treeStyle)
		}
	}
}

// renderAttributeChanges renders attribute-level changes with two-column layout.
// For multi-line values, shows each line on its own row without arrow separator.
func renderAttributeChanges(b *strings.Builder, changes []*AttributeChange, prefix string, hasMoreContent bool, treeStyle lipgloss.Style) {
	// Styles for keys only (values are not colorized).
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorGray))
	createStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorGreen))
	updateStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorYellow))
	deleteStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorRed))

	// Calculate max key width for alignment.
	maxKeyWidth := 0
	for _, change := range changes {
		if len(change.Key) > maxKeyWidth {
			maxKeyWidth = len(change.Key)
		}
	}

	// Pre-compute all formatted values for column width calculation.
	type formattedChange struct {
		change   *AttributeChange
		oldVal   string
		newVal   string
		isMulti  bool
		beforeML bool
		afterML  bool
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

		var oldVal, newVal string
		if !isMulti {
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
			change:   change,
			oldVal:   oldVal,
			newVal:   newVal,
			isMulti:  isMulti,
			beforeML: beforeIsMultiline,
			afterML:  afterIsMultiline,
		}
	}

	for _, fc := range formatted {
		change := fc.change

		// Tree continuation line.
		// Only show │ if there are more sibling resources below (hasMoreContent).
		// Don't show │ just because there are more attributes - that creates
		// a visual "line to nowhere" under └── resources.
		var treeCont string
		if hasMoreContent {
			treeCont = treeStyle.Render(prefix + "│")
		} else {
			treeCont = treeStyle.Render(prefix + " ")
		}

		// Determine key style based on change type (color indicates change type).
		// No symbol on attribute lines - only color-coded keys.
		// - Green: new attribute (before=nil, after!=nil)
		// - Red: deleted attribute (before!=nil, after=nil, NOT unknown)
		// - Yellow: updated attribute (both have values, or unknown computed value)
		var keyStyle lipgloss.Style
		if change.Before == nil && change.After != nil {
			keyStyle = createStyle
		} else if change.Before != nil && change.After == nil && !change.Unknown {
			// Only show as delete if it's truly being removed (not a computed value).
			keyStyle = deleteStyle
		} else {
			// Updated value (including unknown/computed values).
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

		// Check if we need multi-line rendering.
		if fc.isMulti {
			// Multi-line rendering: show key on first line, then each value line.
			// No arrow separator for multi-line values.
			// No symbol - only color-coded key.
			b.WriteString(fmt.Sprintf("      %s  %s%s\n",
				treeCont,
				keyStyle.Render(paddedKey),
				forcesReplacementAnnotation,
			))

			// Determine if tree line should show for multi-line content:
			// Only show │ if there are more sibling resources below (hasMoreContent).
			beforeStr, _ := getRawStringValue(change.Before, change.Sensitive)
			afterStr, _ := getRawStringValue(change.After, change.Sensitive)
			if change.Unknown {
				afterStr = "(known after apply)"
			}

			hasBeforeContent := beforeStr != "" && beforeStr != "(none)"
			hasAfterContent := afterStr != "" && afterStr != "(none)"

			// Render diff based on what content we have.
			if hasBeforeContent && hasAfterContent {
				// Both have content - do a proper line-by-line diff.
				// This shows only changed lines with -/+ markers.
				renderMultilineDiff(b, beforeStr, afterStr, prefix, hasMoreContent, treeStyle)
			} else if hasBeforeContent {
				// Only old content (deletion) - show all lines with -.
				renderMultilineValue(b, beforeStr, prefix, hasMoreContent, treeStyle, "-")
			} else if hasAfterContent {
				// Only new content (creation) - show all lines with +.
				renderMultilineValue(b, afterStr, prefix, hasMoreContent, treeStyle, "+")
			}
		} else {
			// Single-line rendering: old → new on same line with aligned columns.
			// Values are not colorized, only keys are (color indicates change type).
			// No symbol - only color-coded key.
			// Pad old value for consistent column alignment.
			paddedOldVal := fmt.Sprintf("%-*s", maxOldValWidth, fc.oldVal)

			b.WriteString(fmt.Sprintf("      %s  %s %s  %s  %s%s\n",
				treeCont,
				keyStyle.Render(paddedKey),
				dimStyle.Render(paddedOldVal),
				dimStyle.Render("→"),
				fc.newVal,
				forcesReplacementAnnotation,
			))
		}
	}
}

// renderMultilineDiff renders a line-by-line diff of two multi-line strings.
// Only lines that differ get -/+ markers; unchanged lines have no marker.
// Consecutive changed lines are grouped (all - lines, then all + lines) to match
// Terraform's native diff output style.
func renderMultilineDiff(b *strings.Builder, before, after string, prefix string, showTreeLine bool, treeStyle lipgloss.Style) {
	createStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorGreen))
	deleteStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorRed))

	// Get terminal width for smart truncation.
	maxLineWidth := getMaxLineWidth()

	beforeLines := strings.Split(before, "\n")
	afterLines := strings.Split(after, "\n")

	// Truncate long lines helper.
	truncateLine := func(line string) string {
		if len(line) > maxLineWidth {
			return line[:maxLineWidth-3] + "..."
		}
		return line
	}

	// Render a line with optional symbol.
	renderLine := func(line, symbol string, style lipgloss.Style) {
		var treeCont string
		if showTreeLine {
			treeCont = treeStyle.Render(prefix + "│")
		} else {
			treeCont = treeStyle.Render(prefix + " ")
		}
		if symbol == "" {
			// No marker for unchanged lines.
			b.WriteString(fmt.Sprintf("      %s    %s\n", treeCont, truncateLine(line)))
		} else {
			b.WriteString(fmt.Sprintf("      %s  %s %s\n", treeCont, style.Render(symbol), truncateLine(line)))
		}
	}

	i, j := 0, 0
	for i < len(beforeLines) || j < len(afterLines) {
		// Check if current lines match.
		if i < len(beforeLines) && j < len(afterLines) && beforeLines[i] == afterLines[j] {
			// Lines are identical - no marker.
			renderLine(beforeLines[i], "", lipgloss.Style{})
			i++
			j++
		} else {
			// Lines differ - find the extent of consecutive differences.
			// Collect all consecutive differing lines, then output grouped.
			var deletedLines []string
			var addedLines []string

			// Scan ahead to find how many consecutive lines differ.
			// A line "differs" if it doesn't match or we're past one array's end.
			for i < len(beforeLines) || j < len(afterLines) {
				// Check if we're back to matching lines.
				if i < len(beforeLines) && j < len(afterLines) && beforeLines[i] == afterLines[j] {
					break // Found matching lines, stop collecting.
				}

				// Collect differing lines from both sides.
				if i < len(beforeLines) {
					deletedLines = append(deletedLines, beforeLines[i])
					i++
				}
				if j < len(afterLines) {
					addedLines = append(addedLines, afterLines[j])
					j++
				}
			}

			// Output all deleted lines first (grouped).
			for _, line := range deletedLines {
				renderLine(line, "-", deleteStyle)
			}
			// Then output all added lines (grouped).
			for _, line := range addedLines {
				renderLine(line, "+", createStyle)
			}
		}
	}
}

// renderMultilineValue renders each line of a multi-line string value with a symbol.
// Used when there's only before OR after content (not both for diffing).
func renderMultilineValue(b *strings.Builder, content, prefix string, showTreeLine bool, treeStyle lipgloss.Style, symbol string) {
	createStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorGreen))
	deleteStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorRed))

	// Choose symbol style based on +/-.
	var symbolStyle lipgloss.Style
	if symbol == "+" {
		symbolStyle = createStyle
	} else {
		symbolStyle = deleteStyle
	}

	// Get terminal width for smart truncation.
	maxLineWidth := getMaxLineWidth()

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		var treeCont string
		if showTreeLine {
			treeCont = treeStyle.Render(prefix + "│")
		} else {
			treeCont = treeStyle.Render(prefix + " ")
		}
		// Truncate long lines based on terminal width.
		if len(line) > maxLineWidth {
			line = line[:maxLineWidth-3] + "..."
		}
		// Output: tree continuation + symbol + line content.
		b.WriteString(fmt.Sprintf("      %s  %s %s\n",
			treeCont,
			symbolStyle.Render(symbol),
			line,
		))
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
	var badges []string

	// If no changes, show a "NO CHANGES" badge.
	if add == 0 && change == 0 && remove == 0 {
		noChangesBadge := lipgloss.NewStyle().
			Background(lipgloss.Color(theme.ColorDarkGray)).
			Foreground(lipgloss.Color("#FFFFFF")).
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
