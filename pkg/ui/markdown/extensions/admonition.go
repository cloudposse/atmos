// Package extensions provides custom goldmark extensions for enhanced markdown syntax.
package extensions

import (
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// AdmonitionKind is the kind of Admonition AST node.
var AdmonitionKind = ast.NewNodeKind("Admonition")

// AdmonitionType represents the type of admonition.
type AdmonitionType string

const (
	AdmonitionNote      AdmonitionType = "NOTE"
	AdmonitionWarning   AdmonitionType = "WARNING"
	AdmonitionTip       AdmonitionType = "TIP"
	AdmonitionImportant AdmonitionType = "IMPORTANT"
	AdmonitionCaution   AdmonitionType = "CAUTION"

	// MinAdmonitionLineLen is the minimum line length for admonition syntax ("> [!X]").
	minAdmonitionLineLen = 7

	// AdmonitionParserPriority is higher than blockquote (700) to intercept before blockquote parser.
	admonitionParserPriority = 50

	// AdmonitionRendererPriority is the renderer priority for admonitions.
	admonitionRendererPriority = 500

	// NewlineChar is the newline character for output.
	newlineChar = "\n"
)

// Admonition represents a GitHub-style alert block using > [!TYPE] syntax.
type Admonition struct {
	ast.BaseBlock
	AdmonitionType    AdmonitionType
	AdmonitionContent string
}

// Kind returns the kind of this node.
func (n *Admonition) Kind() ast.NodeKind {
	return AdmonitionKind
}

// Dump dumps the node for debugging.
func (n *Admonition) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{
		"Type":    string(n.AdmonitionType),
		"Content": n.AdmonitionContent,
	}, nil)
}

// NewAdmonition creates a new Admonition node.
func NewAdmonition(admonitionType AdmonitionType, content string) *Admonition {
	return &Admonition{
		AdmonitionType:    admonitionType,
		AdmonitionContent: content,
	}
}

// admonitionRegex matches > [!TYPE] or > [!TYPE] content.
var admonitionRegex = regexp.MustCompile(`^>\s*\[!(NOTE|WARNING|TIP|IMPORTANT|CAUTION)\](?:\s*(.*))?$`)

// admonitionParser parses > [!TYPE] syntax.
type admonitionParser struct{}

// Trigger returns the trigger bytes for this parser.
func (p *admonitionParser) Trigger() []byte {
	return []byte{'>'}
}

// Open opens a new admonition block and consumes all continuation lines.
// We consume all blockquote continuation lines (lines starting with >) in Open
// rather than using Continue, because goldmark's block parser architecture
// may consume intermediate lines before Continue is called.
func (p *admonitionParser) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	line, segment := reader.PeekLine()
	lineStr := strings.TrimSuffix(string(line), "\n")

	if len(line) < minAdmonitionLineLen { // Minimum: > [!X]
		return nil, parser.NoChildren
	}

	matches := admonitionRegex.FindStringSubmatch(lineStr)
	if matches == nil {
		return nil, parser.NoChildren
	}

	admonitionType := AdmonitionType(matches[1])
	content := matches[2]

	reader.Advance(segment.Len())

	// Consume all following blockquote lines in Open.
	for {
		nextLine, nextSeg := reader.PeekLine()
		nextLineStr := strings.TrimSuffix(string(nextLine), "\n")

		// Stop if line is empty or doesn't start with >.
		if len(nextLine) == 0 || nextLine[0] != '>' {
			break
		}

		// Don't consume if it's another admonition.
		if admonitionRegex.MatchString(nextLineStr) {
			break
		}

		// Strip > and optional space.
		lineContent := strings.TrimPrefix(nextLineStr, ">")
		lineContent = strings.TrimPrefix(lineContent, " ")

		if content != "" && lineContent != "" {
			content += "\n" + lineContent
		} else if lineContent != "" {
			content = lineContent
		}

		reader.Advance(nextSeg.Len())
	}

	return NewAdmonition(admonitionType, content), parser.NoChildren
}

// Continue is a no-op since we consume all lines in Open.
func (p *admonitionParser) Continue(node ast.Node, reader text.Reader, pc parser.Context) parser.State {
	return parser.Close
}

// Close closes the admonition block.
func (p *admonitionParser) Close(node ast.Node, reader text.Reader, pc parser.Context) {
	// No cleanup needed.
}

// CanInterruptParagraph returns true if this parser can interrupt a paragraph.
func (p *admonitionParser) CanInterruptParagraph() bool {
	return true
}

// CanAcceptIndentedLine returns true if this parser can accept indented lines.
func (p *admonitionParser) CanAcceptIndentedLine() bool {
	return false
}

// admonitionHTMLRenderer renders Admonition nodes to ANSI.
type admonitionHTMLRenderer struct{}

// RegisterFuncs registers the render functions.
func (r *admonitionHTMLRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(AdmonitionKind, r.renderAdmonition)
}

// admonitionStyles defines the icon and colors for each admonition type.
var admonitionStyles = map[AdmonitionType]struct {
	icon  string
	label string
	color string // ANSI 256 color
}{
	AdmonitionNote:      {icon: "ℹ", label: "Note", color: "33"},      // Blue
	AdmonitionWarning:   {icon: "⚠", label: "Warning", color: "208"},  // Orange
	AdmonitionTip:       {icon: "💡", label: "Tip", color: "34"},       // Green
	AdmonitionImportant: {icon: "❗", label: "Important", color: "99"}, // Purple
	AdmonitionCaution:   {icon: "🔥", label: "Caution", color: "196"},  // Red
}

// renderAdmonition renders the Admonition node.
func (r *admonitionHTMLRenderer) renderAdmonition(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}

	adm := n.(*Admonition)
	style, ok := admonitionStyles[adm.AdmonitionType]
	if !ok {
		style = admonitionStyles[AdmonitionNote] // Default to note.
	}

	// Render admonition with icon, colored label, and content.
	// Format: icon Label: content
	_, _ = w.WriteString(newlineChar)
	_, _ = w.WriteString(style.icon)
	_, _ = w.WriteString(" \x1b[38;5;")
	_, _ = w.WriteString(style.color)
	_, _ = w.WriteString("m\x1b[1m")
	_, _ = w.WriteString(style.label)
	_, _ = w.WriteString(":\x1b[0m")

	if adm.AdmonitionContent != "" {
		_, _ = w.WriteString(" ")
		_, _ = w.WriteString(adm.AdmonitionContent)
	}
	_, _ = w.WriteString(newlineChar)

	return ast.WalkContinue, nil
}

// admonitionExtension is the goldmark extension for admonition syntax.
type admonitionExtension struct{}

// NewAdmonitionExtension creates a new admonition extension.
func NewAdmonitionExtension() goldmark.Extender {
	return &admonitionExtension{}
}

// Extend extends the goldmark markdown parser/renderer.
func (e *admonitionExtension) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(
		parser.WithBlockParsers(
			util.Prioritized(&admonitionParser{}, admonitionParserPriority),
		),
	)
	m.Renderer().AddOptions(
		renderer.WithNodeRenderers(
			util.Prioritized(&admonitionHTMLRenderer{}, admonitionRendererPriority),
		),
	)
}
