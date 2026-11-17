package step

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/cellbuf"
)

// viewportText keeps SGR styling while discarding child terminal controls. Each
// line carries its own style so scrolling and truncation cannot color the footer.
type viewportText struct {
	lines  []string
	line   strings.Builder
	style  cellbuf.Style
	color  bool
	dirty  bool
	styled bool
}

func viewportLines(text string, color bool) []string {
	renderer := viewportText{color: color, dirty: true}
	parser := ansi.GetParser()
	defer func() {
		parser.SetHandler(ansi.Handler{})
		ansi.PutParser(parser)
	}()
	parser.SetHandler(ansi.Handler{
		Print:   renderer.print,
		Execute: renderer.control,
		HandleCsi: func(cmd ansi.Cmd, params ansi.Params) {
			if cmd == 'm' {
				cellbuf.ReadStyle(params, &renderer.style)
				renderer.dirty = true
			}
		},
	})
	parser.Parse([]byte(strings.ReplaceAll(text, "\r\n", "\n")))
	if renderer.line.Len() > 0 || len(renderer.lines) == 0 {
		renderer.finishLine()
	}
	return renderer.lines
}

func (r *viewportText) print(char rune) {
	if r.color && r.dirty {
		if r.styled {
			r.line.WriteString(ansi.ResetStyle)
			r.styled = false
		}
		if !r.style.Empty() {
			r.line.WriteString(r.style.Sequence())
			r.styled = true
		}
	}
	r.dirty = false
	r.line.WriteRune(char)
}

func (r *viewportText) control(char byte) {
	switch char {
	case '\n':
		r.finishLine()
	case '\r':
		// Progress updates replace the current row while retaining the active style.
		r.resetLine()
	case '\t':
		for range 4 {
			r.print(' ')
		}
	}
}

func (r *viewportText) finishLine() {
	if r.styled {
		r.line.WriteString(ansi.ResetStyle)
	}
	r.lines = append(r.lines, r.line.String())
	r.resetLine()
}

func (r *viewportText) resetLine() {
	r.line.Reset()
	r.dirty = true
	r.styled = false
}
