// Package extensions provides custom goldmark extensions for enhanced markdown syntax.
package extensions

import (
	"bytes"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// stringNodeRendererPriority matches the priority goldmark's own HTML renderer
// uses for ast.KindString, keeping registration order consistent across renderers.
const stringNodeRendererPriority = 500

// StrictEmailRegexp matches valid email addresses but excludes package references.
// This uses a stricter pattern than goldmark's default that requires:
//   - Local part: letters, digits, and common email special chars (._%+-)
//   - Domain: letters, digits, hyphens, and dots
//   - TLD: at least 2 letters (not numbers)
//
// The TLD requirement is what prevents package references like foo/bar@1.0.0
// from matching: ".0" is numeric and doesn't match [a-zA-Z]{2,}.
//
// Matches: user@example.com, support@company.org, user+tag@mail.co.
// Rejects: foo/bar@1.0.0, replicatedhq/replicated@0.124.1, user@localhost.
var StrictEmailRegexp = regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)

// packageRefTransformerPriority runs early to clean up auto-linked package refs.
const packageRefTransformerPriority = 50

// packageRefTransformer transforms auto-linked package references back to plain text.
// This runs after goldmark's Linkify extension has processed the document and removes
// mailto: links for patterns that look like package references (contain "/" in the URL).
//
// Since glamour uses GFM which includes Linkify with a permissive email regex,
// we can't prevent package refs from being linked. Instead, we unlink them afterward.
type packageRefTransformer struct{}

// Transform implements parser.ASTTransformer.
func (t *packageRefTransformer) Transform(node *ast.Document, reader text.Reader, pc parser.Context) {
	source := reader.Source()

	// Collect auto-link nodes to transform.
	var nodesToReplace []*ast.AutoLink

	_ = ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		autoLink, ok := n.(*ast.AutoLink)
		if !ok {
			return ast.WalkContinue, nil
		}

		// Only process email auto-links (not URL auto-links).
		if autoLink.AutoLinkType != ast.AutoLinkEmail {
			return ast.WalkContinue, nil
		}

		// Get the URL content.
		url := string(autoLink.URL(source))

		// Check if it looks like a package reference (contains "/").
		// Valid emails cannot contain "/" so this is safe.
		if strings.Contains(url, "/") {
			nodesToReplace = append(nodesToReplace, autoLink)
			return ast.WalkContinue, nil
		}

		// Also check if domain part looks like a version (e.g., @1.0.0).
		// Valid emails have letter TLDs, not numeric ones.
		if !StrictEmailRegexp.MatchString(url) {
			nodesToReplace = append(nodesToReplace, autoLink)
		}

		return ast.WalkContinue, nil
	})

	// Replace each auto-link with plain text, walked (and thus replaced) in document
	// order, so searchFrom always advances forward through source and lands on the
	// correct occurrence even when the same label repeats later in the document.
	searchFrom := 0
	for _, autoLink := range nodesToReplace {
		parent := autoLink.Parent()
		if parent == nil {
			continue
		}

		label := autoLink.Label(source)
		parent.ReplaceChild(parent, autoLink, replacementTextNode(source, label, &searchFrom))
	}
}

// replacementTextNode builds the plain-text replacement for an unlinked auto-link.
//
// It prefers an ast.Text node anchored to the label's real position in source
// (found via a forward-only search from *searchFrom, advanced past each match so
// repeated labels resolve to their next occurrence in document order), because
// ast.Text's Pos() reflects that real source offset, which glamour's ANSI
// renderer relies on to place inline content in the correct order. In contrast,
// ast.String nodes always report Pos() -1 ("not associated with a source text"),
// which previously caused glamour to render the replacement text out of order
// relative to its surrounding words.
//
// If the label can't be located (should not normally happen -- AutoLink labels are
// verbatim source bytes), this falls back to an ast.String node so the content is
// still rendered, just without an order guarantee; NewStrictLinkifyExtension also
// registers a renderer for ast.KindString to keep that fallback from silently
// disappearing under glamour's ANSI renderer, which has no built-in handling for it.
func replacementTextNode(source, label []byte, searchFrom *int) ast.Node {
	if idx := bytes.Index(source[*searchFrom:], label); idx >= 0 {
		start := *searchFrom + idx
		stop := start + len(label)
		*searchFrom = stop
		return ast.NewTextSegment(text.NewSegment(start, stop))
	}
	return ast.NewString(label)
}

// stringNodeRenderer renders ast.String nodes (the plain-text replacement
// packageRefTransformer substitutes for unlinked auto-links) as their raw byte
// value. Although goldmark's own HTML renderer registers a handler for
// ast.KindString, glamour's ANSI renderer does not: without this registration,
// glamour logs "Warning: unhandled element String" and silently drops the
// node's content instead of printing it, e.g. rendering
// "atmos toolchain exec terraform@1.5.0 -- version" as
// "atmos toolchain exec -- version" with the tool@version spec gone.
type stringNodeRenderer struct{}

// RegisterFuncs implements renderer.NodeRenderer.
func (r *stringNodeRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindString, r.renderString)
}

// renderString writes the ast.String node's raw value, unescaped -- this output
// goes straight to a terminal, not HTML, so no escaping is needed or wanted.
func (r *stringNodeRenderer) renderString(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	str, ok := n.(*ast.String)
	if !ok {
		return ast.WalkContinue, nil
	}
	_, _ = w.Write(str.Value)
	return ast.WalkContinue, nil
}

// strictLinkifyExtension adds a transformer that unlinks package references.
type strictLinkifyExtension struct{}

// NewStrictLinkifyExtension creates an extension that prevents package references
// like foo/bar@1.0.0 from being rendered as mailto: links.
//
// Since glamour uses GFM which includes Linkify with a permissive email regex,
// this extension adds an AST transformer that runs after parsing and converts
// auto-linked package references back to plain text. It also registers a
// renderer for the resulting ast.String nodes (see stringNodeRenderer) since
// glamour's ANSI renderer has no built-in handling for that node kind.
//
// It identifies package references by:
//   - Presence of "/" in the URL (emails cannot contain slashes)
//   - URL not matching a strict email pattern (TLD must be letters)
func NewStrictLinkifyExtension() goldmark.Extender {
	return &strictLinkifyExtension{}
}

// Extend implements goldmark.Extender.
func (e *strictLinkifyExtension) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(
		parser.WithASTTransformers(
			util.Prioritized(&packageRefTransformer{}, packageRefTransformerPriority),
		),
	)
	m.Renderer().AddOptions(
		renderer.WithNodeRenderers(
			util.Prioritized(&stringNodeRenderer{}, stringNodeRendererPriority),
		),
	)
}
