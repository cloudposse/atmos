// Package extensions provides custom goldmark extensions for enhanced markdown syntax.
package extensions

import (
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	astext "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// MutedKind is the kind of Muted AST node.
var MutedKind = ast.NewNodeKind("Muted")

// Parser and transformer priorities.
const (
	// MutedParserPriority ensures muted runs before other inline parsers.
	mutedParserPriority = 50
	// MutedTransformerPriority runs the transformer after parsing is complete.
	mutedTransformerPriority = 100
)

// Muted represents muted/subtle text using ((text)) syntax.
type Muted struct {
	ast.BaseInline
}

// Kind returns the kind of this node.
func (n *Muted) Kind() ast.NodeKind {
	return MutedKind
}

// Dump dumps the node for debugging.
func (n *Muted) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, nil, nil)
}

// NewMuted creates a new Muted node.
func NewMuted() *Muted {
	return &Muted{}
}

// mutedParser parses ((muted text)) syntax.
type mutedParser struct{}

// Trigger returns the trigger bytes for this parser.
func (p *mutedParser) Trigger() []byte {
	return []byte{'('}
}

// Parse parses the muted syntax.
func (p *mutedParser) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	line, segment := block.PeekLine()

	// Need at least (( at current position.
	if len(line) < 2 || line[0] != '(' || line[1] != '(' {
		return nil
	}

	// Find closing )).
	start := segment.Start + 2
	for i := 2; i < len(line)-1; i++ {
		// Skip if not a closing delimiter.
		if line[i] != ')' || line[i+1] != ')' {
			continue
		}
		// Found closing )). Reject if empty content.
		if i == 2 {
			return nil
		}
		block.Advance(i + 2)
		node := NewMuted()
		node.AppendChild(node, ast.NewTextSegment(text.NewSegment(start, segment.Start+i)))
		return node
	}

	return nil
}

// mutedTransformer transforms Muted nodes to Strikethrough nodes after parsing.
// This is necessary because glamour's ANSI renderer uses a block stack buffering
// model that doesn't work well with custom node renderers. By transforming to
// Strikethrough (a standard GFM node that glamour knows how to render), we get
// correct text order and integrate with glamour's styling system.
// The strikethrough style is configured to render as muted gray text.
type mutedTransformer struct{}

// Transform implements parser.ASTTransformer.
func (t *mutedTransformer) Transform(node *ast.Document, reader text.Reader, pc parser.Context) {
	// Collect nodes to transform (don't modify during walk).
	var nodesToReplace []*Muted

	_ = ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering || n.Kind() != MutedKind {
			return ast.WalkContinue, nil
		}
		nodesToReplace = append(nodesToReplace, n.(*Muted))
		return ast.WalkSkipChildren, nil
	})

	// Replace each Muted node with Strikethrough.
	for _, muted := range nodesToReplace {
		strikethrough := astext.NewStrikethrough()

		// Move all children from muted to strikethrough.
		for child := muted.FirstChild(); child != nil; {
			next := child.NextSibling()
			strikethrough.AppendChild(strikethrough, child)
			child = next
		}

		// Replace muted with strikethrough in the parent.
		if parent := muted.Parent(); parent != nil {
			parent.ReplaceChild(parent, muted, strikethrough)
		}
	}
}

// mutedExtension is the goldmark extension for muted syntax.
type mutedExtension struct{}

// NewMutedExtension creates a new muted extension.
func NewMutedExtension() goldmark.Extender {
	return &mutedExtension{}
}

// Extend extends the goldmark markdown parser/renderer.
// It adds the muted parser and AST transformer. The renderer is not needed
// because the transformer converts Muted nodes to Strikethrough nodes,
// which glamour's ANSI renderer already knows how to handle.
func (e *mutedExtension) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(
		parser.WithInlineParsers(
			util.Prioritized(&mutedParser{}, mutedParserPriority),
		),
		parser.WithASTTransformers(
			util.Prioritized(&mutedTransformer{}, mutedTransformerPriority),
		),
	)
}
