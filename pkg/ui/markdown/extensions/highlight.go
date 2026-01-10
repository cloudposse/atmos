// Package extensions provides custom goldmark extensions for enhanced markdown syntax.
package extensions

import (
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// HighlightKind is the kind of Highlight AST node.
var HighlightKind = ast.NewNodeKind("Highlight")

const (
	// HighlightParserPriority is the parser priority for highlight syntax.
	highlightParserPriority = 500

	// HighlightRendererPriority is the renderer priority for highlight nodes.
	highlightRendererPriority = 500
)

// Highlight represents highlighted text using ==text== syntax.
type Highlight struct {
	ast.BaseInline
}

// Kind returns the kind of this node.
func (n *Highlight) Kind() ast.NodeKind {
	return HighlightKind
}

// Dump dumps the node for debugging.
func (n *Highlight) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, nil, nil)
}

// NewHighlight creates a new Highlight node.
func NewHighlight() *Highlight {
	return &Highlight{}
}

// highlightDelimiterProcessor processes == delimiters for highlight syntax.
type highlightDelimiterProcessor struct{}

// IsDelimiter returns true if the byte is a delimiter.
func (p *highlightDelimiterProcessor) IsDelimiter(b byte) bool {
	return b == '='
}

// CanOpenCloser returns true if the delimiter can open/close.
func (p *highlightDelimiterProcessor) CanOpenCloser(opener, closer *parser.Delimiter) bool {
	return opener.Char == closer.Char && opener.Length >= 2 && closer.Length >= 2
}

// OnMatch is called when a delimiter pair is matched.
func (p *highlightDelimiterProcessor) OnMatch(consumes int) ast.Node {
	return NewHighlight()
}

// highlightParser parses ==highlight== syntax.
type highlightParser struct{}

// Trigger returns the trigger bytes for this parser.
func (p *highlightParser) Trigger() []byte {
	return []byte{'='}
}

// Parse parses the highlight syntax.
func (p *highlightParser) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	line, segment := block.PeekLine()
	if len(line) < 2 || line[0] != '=' || line[1] != '=' {
		return nil
	}

	// Find closing ==.
	start := segment.Start + 2
	for i := 2; i < len(line)-1; i++ {
		if line[i] == '=' && line[i+1] == '=' {
			// Found closing delimiter.
			block.Advance(i + 2)
			node := NewHighlight()
			node.AppendChild(node, ast.NewTextSegment(text.NewSegment(start, segment.Start+i)))
			return node
		}
	}

	return nil
}

// highlightHTMLRenderer renders Highlight nodes to ANSI.
// Note: glamour's ANSI renderer will handle the actual styling based on the node type.
// We output mark tags which glamour will style.
type highlightHTMLRenderer struct{}

// RegisterFuncs registers the render functions.
func (r *highlightHTMLRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(HighlightKind, r.renderHighlight)
}

// renderHighlight renders the Highlight node.
// We render the text content ourselves and skip children to prevent glamour's
// ANSI renderer from overriding our highlight style with its own text styles.
func (r *highlightHTMLRenderer) renderHighlight(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}

	// Start yellow background using ANSI escape codes.
	// ESC[43m = yellow background, ESC[30m = black foreground.
	_, _ = w.WriteString("\x1b[43m\x1b[30m")

	// Render text content ourselves by extracting from children.
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		if child.Kind() == ast.KindText {
			textNode := child.(*ast.Text)
			segment := textNode.Segment
			_, _ = w.Write(segment.Value(source))
		}
	}

	// Reset styling.
	_, _ = w.WriteString("\x1b[0m")

	// Skip children since we rendered them ourselves.
	return ast.WalkSkipChildren, nil
}

// highlightExtension is the goldmark extension for highlight syntax.
type highlightExtension struct{}

// NewHighlightExtension creates a new highlight extension.
func NewHighlightExtension() goldmark.Extender {
	return &highlightExtension{}
}

// Extend extends the goldmark markdown parser/renderer.
func (e *highlightExtension) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(
		parser.WithInlineParsers(
			util.Prioritized(&highlightParser{}, highlightParserPriority),
		),
	)
	m.Renderer().AddOptions(
		renderer.WithNodeRenderers(
			util.Prioritized(&highlightHTMLRenderer{}, highlightRendererPriority),
		),
	)
}
