// Package extensions provides custom goldmark extensions for enhanced markdown syntax.
package extensions

import (
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

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
		}

		// Also check if domain part looks like a version (e.g., @1.0.0).
		// Valid emails have letter TLDs, not numeric ones.
		if !StrictEmailRegexp.MatchString(url) {
			nodesToReplace = append(nodesToReplace, autoLink)
		}

		return ast.WalkContinue, nil
	})

	// Replace each auto-link with plain text.
	for _, autoLink := range nodesToReplace {
		parent := autoLink.Parent()
		if parent == nil {
			continue
		}

		// Create a text node with the original content.
		// AutoLink stores its text in a single child segment.
		textNode := ast.NewString(autoLink.Label(source))
		parent.ReplaceChild(parent, autoLink, textNode)
	}
}

// strictLinkifyExtension adds a transformer that unlinks package references.
type strictLinkifyExtension struct{}

// NewStrictLinkifyExtension creates an extension that prevents package references
// like foo/bar@1.0.0 from being rendered as mailto: links.
//
// Since glamour uses GFM which includes Linkify with a permissive email regex,
// this extension adds an AST transformer that runs after parsing and converts
// auto-linked package references back to plain text.
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
}
