package markdown

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"

	"github.com/cloudposse/atmos/pkg/perf"
)

// bulletPrefixPattern matches the literal bullet glamour renders for unordered
// list items. Atmos always configures Item.BlockPrefix as "• " -- both the
// builtin default style (styles.go getBuiltinDefaultStyle) and every
// theme-derived style (theme/converter.go createGlamourStyleFromTheme) use
// this exact literal, so matching it directly (rather than reading it back
// out of a StyleConfig) is safe and avoids threading style state through
// every render call site.
var bulletPrefixPattern = regexp.MustCompile(`^• `)

// orderedPrefixPattern matches the "<number>. " prefix glamour renders for
// ordered list items (the item's own number followed by the literal
// Enumeration.BlockPrefix ". ", see the same two style sources above).
var orderedPrefixPattern = regexp.MustCompile(`^\d+\. `)

// FixListHangingIndent re-indents wrapped continuation lines of bullet and
// ordered markdown list items so they align under the item's own text
// instead of falling flush with the bullet/number.
//
// Why this exists (glamour v1.0.0, ansi/blockelement.go BlockElement.Finish):
// a whole list -- every item's bullet/number and text -- is rendered into one
// shared buffer, then word-wrapped as a single blob and given one uniform
// block-level left margin. Glamour has no concept of a hanging indent, so
// every wrapped line, including continuation lines, gets the same left
// margin as the bullet/number line itself. The words placed on each line are
// already correct; only the per-line left padding is wrong, which is why
// this is a line-based post-process rather than a re-wrap.
//
// Overriding glamour's list rendering via a higher-priority goldmark
// NodeRenderer (the pattern extensions/linkify.go uses for ast.KindString)
// was evaluated and rejected for this bug. Glamour funnels an entire list's
// content (bullets, numbers, and every inline node inside each item) through
// a single unexported blockStack owned by the ansi.ANSIRenderer it builds
// internally, and that blockStack is never exposed to goldmark's
// NodeRendererFunc signature. Overriding ast.KindList/ast.KindListItem
// without also reimplementing every inline node kind (text, emphasis, links,
// code spans, and so on) would write list content to the wrong buffer
// entirely. Doing it correctly would require a second, deeper layer of
// unsafe reflection on top of getGlamourGoldmark's (into glamour's
// ANSIRenderer, then into its private RenderContext.blockStack) for what is
// ultimately a missing-padding bug, not a wrong-content bug. Transforming
// glamour's own, otherwise-correct, rendered output is the narrower and more
// robust fix.
//
// This is applied to every markdown render path in pkg/ui (both
// CustomRenderer.Render and formatter.renderMarkdown's raw glamour path),
// since both can render lists.
func FixListHangingIndent(rendered string) string {
	defer perf.Track(nil, "markdown.FixListHangingIndent")()

	if rendered == "" || !strings.Contains(rendered, "\n") {
		return rendered
	}

	lines := strings.Split(rendered, "\n")

	// hangIndent is the number of extra spaces continuation lines of the
	// current item need; -1 means "not currently inside a list item".
	hangIndent := -1
	// baseIndent is the visible indent width of the item's own bullet/number
	// line; continuation lines of that item share this same indent.
	baseIndent := -1

	for i, line := range lines {
		plain := ansi.Strip(line)
		trimmed := strings.TrimLeft(plain, " ")
		indent := len(plain) - len(trimmed)

		if trimmed == "" {
			hangIndent = -1
			continue
		}

		if prefix := listItemPrefix(trimmed); prefix != "" {
			baseIndent = indent
			// Rune count, not byte length: the bullet "•" (U+2022) is a
			// single terminal column but three UTF-8 bytes.
			hangIndent = utf8.RuneCountInString(prefix)
			continue
		}

		if hangIndent >= 0 && indent == baseIndent {
			// Plain leading spaces are visually identical to any styled
			// spaces already on the line -- a space glyph has no visible
			// foreground color -- so prepending them unstyled is safe and
			// avoids needing to parse past embedded ANSI codes to find an
			// "insertion point".
			lines[i] = strings.Repeat(" ", hangIndent) + line
			continue
		}

		hangIndent = -1
	}

	return strings.Join(lines, "\n")
}

// listItemPrefix returns the bullet/number prefix (e.g. "• " or "12. ") at
// the start of trimmed, or "" if trimmed doesn't start with one.
func listItemPrefix(trimmed string) string {
	if m := bulletPrefixPattern.FindString(trimmed); m != "" {
		return m
	}
	return orderedPrefixPattern.FindString(trimmed)
}
