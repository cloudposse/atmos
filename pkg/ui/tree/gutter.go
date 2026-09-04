// Package tree computes the box-drawing gutters ("│", "├── ", "└── ") that connect the
// rows of a rendered tree, and checks that a rendered tree is fully connected.
//
// Renderers tend to compute these gutters ad hoc at each place they emit a row (the
// node's own row, rows of content that belong to the node, spacer rows) and drift apart:
// the classic failure is a node whose content rows sit between its connector and its
// children's connectors with no rail through them, leaving the children floating. This
// package owns that geometry in one place, as pure functions of a node's position, so a
// renderer only has to describe where a row sits and the invariant can be tested directly
// against generated trees (see Violations).
package tree

import (
	"fmt"
	"strings"
)

const (
	// ConnectorMore is the connector of a node that has later siblings.
	ConnectorMore = "├── "
	// ConnectorLast is the connector of a node that is the last of its siblings.
	ConnectorLast = "└── "
	// Rail is one level of gutter with rows still to come at that level.
	Rail = "│   "
	// Blank is one level of gutter with nothing further at that level; it has the same
	// width as Rail so deeper columns stay aligned.
	Blank = "    "

	// Width is the display width of every gutter segment above.
	Width = 4

	// The narrowest connector arm Violations recognizes. Lipgloss/tree's default
	// enumerator ("├──", no trailing space) puts a child's connector three columns right
	// of its parent's, while this package's segments put it four (Width).
	minArmWidth = 3
)

// Path is a node's position in a tree, one entry per level from the root's children down
// to the node itself: Path[i] reports whether the node's ancestor at depth i (the node
// itself for the final entry) is the last among its siblings.
type Path []bool

// Connector returns the gutter for the node's own row: a rail or blank for each ancestor
// level, then the node's connector.
func Connector(p Path) string {
	if len(p) == 0 {
		return ""
	}
	var b strings.Builder
	writeLevels(&b, p[:len(p)-1])
	if p[len(p)-1] {
		b.WriteString(ConnectorLast)
	} else {
		b.WriteString(ConnectorMore)
	}
	return b.String()
}

// ContentGutter returns the gutter for rows of content that belong to the node and sit
// between the node's row and its children's rows (attribute diffs, descriptions, ...):
// a rail or blank for every level including the node's own, then one more level that
// carries a rail down to the children when the node has any, so the first child's
// connector is never left floating below the content.
func ContentGutter(p Path, hasChildren bool) string {
	var b strings.Builder
	writeLevels(&b, p)
	if hasChildren {
		b.WriteString(Rail)
	} else {
		b.WriteString(Blank)
	}
	return b.String()
}

// SpacerGutter returns the gutter for an otherwise empty spacer row emitted after the
// node's whole block (its content and children), before its next sibling: the rails that
// still have rows to come, with trailing padding trimmed. A node that is the last of its
// siblings has no rail of its own to carry.
func SpacerGutter(p Path) string {
	var b strings.Builder
	writeLevels(&b, p)
	return strings.TrimRight(b.String(), " ")
}

// writeLevels writes one Rail or Blank segment per level.
func writeLevels(b *strings.Builder, levels []bool) {
	for _, last := range levels {
		if last {
			b.WriteString(Blank)
		} else {
			b.WriteString(Rail)
		}
	}
}

// SpacerFromConnectorRow derives the spacer row that belongs in place of a rendered tree row
// whose connector introduces a spacer placeholder (a blank line between sibling blocks).
//
// The row is the rendered row with ANSI styling stripped. Everything before the connector -- the
// ancestor rails -- is kept as is; the connector itself becomes a rail when it is "├" (rows
// still follow at that level) and nothing when it is "└"; the connector's arm and the
// placeholder text are dropped. This is SpacerGutter for renderers that emit spacers as
// tree nodes (lipgloss/tree) rather than tracking a Path themselves. A row without a
// connector has no tree position and yields an empty spacer.
func SpacerFromConnectorRow(row string) string {
	runes := []rune(row)
	for c, ch := range runes {
		if !isConnector(ch) {
			continue
		}
		gutter := string(runes[:c])
		if ch == '├' {
			gutter += "│"
		}
		return strings.TrimRight(gutter, " ")
	}
	return ""
}

// Violations checks that a rendered tree is connected and returns one message per break.
//
// The rows are the rendered lines with any ANSI styling already stripped; any fixed leading
// column (an action symbol, indentation) is fine as long as it is the same width on every
// row. The invariant: every box-drawing character must have, on the row directly above
// it, either a box-drawing character in the same column (the rail it continues) or a
// connector one arm's width to the left (the parent it hangs from; both this package's
// four-column segments and lipgloss/tree's three-column enumerator are recognized). The one
// exception is a root-level connector -- one in the tree's leftmost box-drawing column --
// which may sit under a row with no tree characters at all, such as a header. Nothing
// deeper may: a nested connector or a rail under a bare row is exactly a broken rail.
func Violations(rows []string) []string {
	root := rootColumn(rows)
	var out []string
	var prev []rune
	for r, row := range rows {
		cur := []rune(row)
		for c, ch := range cur {
			if r == 0 || !isBox(ch) {
				continue
			}
			above := at(prev, c)
			switch {
			case isBox(above), hangsFromParent(prev, c):
				// Continues a rail, or hangs from its parent's connector.
			case isBare(prev) && isConnector(ch) && c == root:
				// A root-level connector under a header row.
			default:
				out = append(out, fmt.Sprintf("row %d col %d: %q has nothing above it: %q", r, c, string(ch), strings.TrimRight(string(prev), " ")))
			}
		}
		prev = cur
	}
	return out
}

// hangsFromParent reports whether column c on the row above holds a connector one arm's
// width to the left, i.e. the parent this row's box-drawing character hangs from.
func hangsFromParent(prev []rune, c int) bool {
	for w := minArmWidth; w <= Width; w++ {
		if isConnector(at(prev, c-w)) {
			return true
		}
	}
	return false
}

// rootColumn returns the leftmost column holding a box-drawing character anywhere in
// rows: the column of root-level connectors. It is -1 when there are none.
func rootColumn(rows []string) int {
	root := -1
	for _, row := range rows {
		for c, ch := range []rune(row) {
			if isBox(ch) && (root == -1 || c < root) {
				root = c
			}
			if isBox(ch) {
				break
			}
		}
	}
	return root
}

// isBare reports whether a row has no tree characters at all (a header or label row).
func isBare(row []rune) bool {
	for _, ch := range row {
		if isBox(ch) {
			return false
		}
	}
	return true
}

func at(row []rune, i int) rune {
	if i < 0 || i >= len(row) {
		return 0
	}
	return row[i]
}

func isBox(ch rune) bool { return ch == '│' || isConnector(ch) }

func isConnector(ch rune) bool { return ch == '├' || ch == '└' }
