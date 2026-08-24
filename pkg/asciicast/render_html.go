package asciicast

import (
	"fmt"
	"html"
	"image/color"
	"os"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/cellbuf"

	"github.com/cloudposse/atmos/pkg/perf"
)

// atmosBlue is the Atmos brand blue applied to ANSI blue output in docs
// artifacts. The legacy aha pipeline rewrote `color:blue` and bat's GitHub
// blue (#183691) to this value with sed; the native renderer maps them
// directly.
const atmosBlue = "#005f87"

// htmlColorOverrides remaps specific resolved colors to docs-friendly values.
var htmlColorOverrides = map[string]string{
	"#183691": atmosBlue,
}

// RenderHTML writes the final terminal content of a cast file as an HTML
// fragment of inline-styled spans, one line per text line. The fragment is
// consumed by the website Screengrab component via dangerouslySetInnerHTML
// inside a <pre>, so no wrapper element and no background colors are emitted
// (backgrounds broke light/dark themes with the legacy aha pipeline).
func RenderHTML(input, output string) error {
	defer perf.Track(nil, "asciicast.RenderHTML")()

	grid, err := BuildGrid(input)
	if err != nil {
		return err
	}
	return os.WriteFile(output, []byte(gridHTML(grid)), castFilePerm)
}

func gridHTML(grid *cellbuf.Buffer) string {
	var sb strings.Builder
	height := grid.Height()
	for y := 0; y < height; y++ {
		sb.WriteString(gridLineHTML(grid, y))
		sb.WriteByte('\n')
	}
	return strings.TrimRight(sb.String(), "\n") + "\n"
}

// htmlRun is a horizontal run of cells sharing one style and hyperlink.
type htmlRun struct {
	style cellbuf.Style
	link  cellbuf.Link
	text  strings.Builder
}

func gridLineHTML(grid *cellbuf.Buffer, y int) string {
	var sb strings.Builder
	var run *htmlRun
	flush := func() {
		if run != nil {
			sb.WriteString(run.render())
			run = nil
		}
	}
	for x := 0; x < grid.Width(); x++ {
		cell := grid.Cell(x, y)
		if cell == nil || cell.Width == 0 {
			continue
		}
		if run == nil || !run.style.Equal(&cell.Style) || run.link != cell.Link {
			flush()
			run = &htmlRun{style: cell.Style, link: cell.Link}
		}
		run.text.WriteString(cell.String())
	}
	flush()
	return strings.TrimRight(sb.String(), " ")
}

func (r *htmlRun) render() string {
	text := r.text.String()
	// A run of unstyled trailing blanks renders as plain spaces so TrimRight
	// can drop them; styled blanks keep their span (e.g. underlined padding).
	escaped := html.EscapeString(text)
	css := styleCSS(&r.style)
	body := escaped
	if css != "" {
		body = fmt.Sprintf("<span style=%q>%s</span>", css, escaped)
	}
	if r.link.URL != "" {
		return fmt.Sprintf("<a href=%q>%s</a>", r.link.URL, body)
	}
	return body
}

// styleCSS converts a cell style to inline CSS. Background colors are
// intentionally never emitted: docs screengrabs must adapt to the website's
// light and dark terminal themes.
func styleCSS(s *cellbuf.Style) string {
	if s == nil || s.Empty() {
		return ""
	}
	var parts []string
	if hex := cssColor(s.Fg); hex != "" {
		parts = append(parts, "color:"+hex)
	}
	if s.Attrs.Contains(cellbuf.BoldAttr) {
		parts = append(parts, "font-weight:bold")
	}
	if s.Attrs.Contains(cellbuf.ItalicAttr) {
		parts = append(parts, "font-style:italic")
	}
	if s.Attrs.Contains(cellbuf.FaintAttr) {
		parts = append(parts, "opacity:0.7")
	}
	decorations := textDecorations(s)
	if len(decorations) > 0 {
		parts = append(parts, "text-decoration:"+strings.Join(decorations, " "))
	}
	return strings.Join(parts, ";")
}

func textDecorations(s *cellbuf.Style) []string {
	var decorations []string
	if s.UlStyle != cellbuf.NoUnderline {
		decorations = append(decorations, "underline")
	}
	if s.Attrs.Contains(cellbuf.StrikethroughAttr) {
		decorations = append(decorations, "line-through")
	}
	return decorations
}

// cssColor resolves an ANSI color to a CSS hex value, applying Atmos docs
// overrides (ANSI blue and bat's GitHub blue become the Atmos brand blue).
func cssColor(c ansi.Color) string {
	if c == nil {
		return ""
	}
	if basic, ok := c.(ansi.BasicColor); ok && (basic == ansi.Blue || basic == ansi.BrightBlue) {
		return atmosBlue
	}
	hex := colorHex(c)
	if override, ok := htmlColorOverrides[hex]; ok {
		return override
	}
	return hex
}

func colorHex(c color.Color) string {
	r, g, b, _ := c.RGBA()
	const eightBitShift = 8
	const byteMask = 0xff
	return fmt.Sprintf("#%02x%02x%02x", uint8(r>>eightBitShift&byteMask), uint8(g>>eightBitShift&byteMask), uint8(b>>eightBitShift&byteMask))
}
