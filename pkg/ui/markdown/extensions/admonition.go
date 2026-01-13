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

// admonitionStyles defines the icon and label for each admonition type.
// Colors are determined dynamically via getAdmonitionStyle() using the theme system.
var admonitionStyles = map[AdmonitionType]struct {
	icon  string
	label string
}{
	AdmonitionNote:      {icon: "ℹ", label: "Note"},
	AdmonitionWarning:   {icon: "⚠", label: "Warning"},
	AdmonitionTip:       {icon: "💡", label: "Tip"},
	AdmonitionImportant: {icon: "❗", label: "Important"},
	AdmonitionCaution:   {icon: "🔥", label: "Caution"},
}

// Semantic colors for admonitions using ANSI color numbers.
// These match the default theme's semantic color mappings to ensure consistency.
// Using ANSI color numbers allows lipgloss to adapt to the terminal's color profile.
var admonitionColors = map[AdmonitionType]lipgloss.Color{
	AdmonitionNote:      lipgloss.Color("12"), // Bright Blue (Info/Link color)
	AdmonitionWarning:   lipgloss.Color("3"),  // Yellow (Warning color)
	AdmonitionTip:       lipgloss.Color("2"),  // Green (Success color)
	AdmonitionImportant: lipgloss.Color("5"),  // Magenta (Notice/Secondary color)
	AdmonitionCaution:   lipgloss.Color("1"),  // Red (Error color)
}

// getAdmonitionStyle returns the lipgloss style for an admonition type.
// Uses semantic ANSI colors that adapt to the terminal's color profile.
func getAdmonitionStyle(admonitionType AdmonitionType) lipgloss.Style {
	color, ok := admonitionColors[admonitionType]
	if !ok {
		color = admonitionColors[AdmonitionNote] // Default to note color.
	}
	return lipgloss.NewStyle().Foreground(color)
}

// renderAdmonition renders the Admonition node.
func (r *admonitionHTMLRenderer) renderAdmonition(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}

	adm := n.(*Admonition)
	styleInfo, ok := admonitionStyles[adm.AdmonitionType]
	if !ok {
		styleInfo = admonitionStyles[AdmonitionNote] // Default to note.
	}

	// Get theme-aware style for this admonition type.
	labelStyle := getAdmonitionStyle(adm.AdmonitionType).Bold(true)

	// Render admonition with icon, colored label, and content.
	// Format: icon Label: content
	_, _ = w.WriteString(newlineChar)
	_, _ = w.WriteString(styleInfo.icon)
	_, _ = w.WriteString(" ")
	_, _ = w.WriteString(labelStyle.Render(styleInfo.label + ":"))

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
