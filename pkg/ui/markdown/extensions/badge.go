// Package extensions provides custom goldmark extensions for enhanced markdown syntax.
package extensions

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"

	"github.com/cloudposse/atmos/pkg/ui/theme"
)

// BadgeKind is the kind of Badge AST node.
var BadgeKind = ast.NewNodeKind("Badge")

const (
	// MinBadgeLen is the minimum length for badge syntax ("[!BADGE ]").
	minBadgeLen = 8

	// BadgeParserPriority ensures badge runs before link parser (priority 200).
	badgeParserPriority = 50

	// BadgeRendererPriority is the renderer priority for badges.
	badgeRendererPriority = 500
)

// Badge represents a styled badge using [!BADGE text] or [!BADGE:variant text] syntax.
type Badge struct {
	ast.BaseInline
	BadgeVariant string // e.g., "warning", "success", "error", "info", or empty for default
	BadgeText    string // The badge text content
}

// Kind returns the kind of this node.
func (n *Badge) Kind() ast.NodeKind {
	return BadgeKind
}

// Dump dumps the node for debugging.
func (n *Badge) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{
		"Variant": n.BadgeVariant,
		"Text":    n.BadgeText,
	}, nil)
}

// NewBadge creates a new Badge node.
func NewBadge(variant, badgeText string) *Badge {
	return &Badge{
		BadgeVariant: variant,
		BadgeText:    badgeText,
	}
}

// badgeRegex matches [!BADGE text] or [!BADGE:variant text].
var badgeRegex = regexp.MustCompile(`^\[!BADGE(?::(\w+))?\s+([^\]]+)\]`)

// badgeParser parses [!BADGE text] syntax.
type badgeParser struct{}

// Trigger returns the trigger bytes for this parser.
func (p *badgeParser) Trigger() []byte {
	return []byte{'['}
}

// Parse parses the badge syntax.
func (p *badgeParser) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	line, _ := block.PeekLine()
	pos := block.LineOffset()

	// Get remaining text from current position.
	remaining := string(line[pos:])

	if len(remaining) < minBadgeLen { // Minimum: [!BADGE ]
		return nil
	}

	// Check for [!BADGE prefix.
	if !strings.HasPrefix(remaining, "[!BADGE") {
		return nil
	}

	matches := badgeRegex.FindStringSubmatch(remaining)
	if matches == nil {
		return nil
	}

	variant := matches[1] // May be empty.
	badgeText := matches[2]

	block.Advance(len(matches[0]))
	return NewBadge(variant, badgeText)
}

// badgeHTMLRenderer renders Badge nodes to ANSI.
type badgeHTMLRenderer struct{}

// RegisterFuncs registers the render functions.
func (r *badgeHTMLRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(BadgeKind, r.renderBadge)
}

// getBadgeStyle returns the lipgloss style for a badge variant.
// Uses theme-aware colors that adapt to the user's configured theme.
// Badge styles use the semantic color as background with contrasting foreground.
func getBadgeStyle(variant string) lipgloss.Style {
	styles := theme.GetCurrentStyles()
	var baseStyle lipgloss.Style

	switch variant {
	case "warning":
		baseStyle = styles.Warning
	case "success":
		baseStyle = styles.Success
	case "error":
		baseStyle = styles.Error
	case "info":
		baseStyle = styles.Info
	default:
		// Default to Notice (purple) for unmarked badges.
		baseStyle = styles.Notice
	}

	// Extract the foreground color from the theme style and use as background.
	// Use a contrasting dark color for foreground text.
	fg := baseStyle.GetForeground()
	return lipgloss.NewStyle().
		Background(fg).
		Foreground(lipgloss.Color("16")). // Dark/black for contrast.
		Bold(true).
		Padding(0, 1)
}

// renderBadge renders the Badge node.
func (r *badgeHTMLRenderer) renderBadge(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}

	badge := n.(*Badge)
	style := getBadgeStyle(badge.BadgeVariant)
	_, _ = w.WriteString(style.Render(badge.BadgeText))

	return ast.WalkContinue, nil
}

// badgeExtension is the goldmark extension for badge syntax.
type badgeExtension struct{}

// NewBadgeExtension creates a new badge extension.
func NewBadgeExtension() goldmark.Extender {
	return &badgeExtension{}
}

// Extend extends the goldmark markdown parser/renderer.
func (e *badgeExtension) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(
		parser.WithInlineParsers(
			util.Prioritized(&badgeParser{}, badgeParserPriority),
		),
	)
	m.Renderer().AddOptions(
		renderer.WithNodeRenderers(
			util.Prioritized(&badgeHTMLRenderer{}, badgeRendererPriority),
		),
	)
}
