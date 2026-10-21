package markdown

import (
	"net/url"

	"github.com/charmbracelet/glamour"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

const baseURLTransformerPriority = 100

// ApplyBaseURL resolves Markdown links and images against a fetched document's
// URL. Glamour's WithBaseURL strips leading slashes before resolving links,
// incorrectly treating site-root links as document-relative on nested pages.
// Transform destinations in the AST instead, preserving code and link labels.
func ApplyBaseURL(renderer *glamour.TermRenderer, sourceURL string) {
	if sourceURL == "" {
		return
	}
	base, err := url.Parse(sourceURL)
	if err != nil || !base.IsAbs() {
		return
	}
	md := getGlamourGoldmark(renderer)
	if md == nil {
		return
	}
	md.Parser().AddOptions(parser.WithASTTransformers(util.Prioritized(&baseURLTransformer{base: base}, baseURLTransformerPriority)))
}

type baseURLTransformer struct {
	base *url.URL
}

func (t *baseURLTransformer) Transform(document *ast.Document, _ text.Reader, _ parser.Context) {
	_ = ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			switch n := node.(type) {
			case *ast.Link:
				n.Destination = t.resolve(n.Destination)
			case *ast.Image:
				n.Destination = t.resolve(n.Destination)
			}
		}
		return ast.WalkContinue, nil
	})
}

func (t *baseURLTransformer) resolve(destination []byte) []byte {
	reference, err := url.Parse(string(destination))
	if err != nil || reference.IsAbs() {
		return destination
	}
	return []byte(t.base.ResolveReference(reference).String())
}
