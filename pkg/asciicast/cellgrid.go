package asciicast

import (
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/cellbuf"

	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	tabStopInterval = 8
	oscHyperlinkCmd = 8 // OSC 8 is the hyperlink escape sequence command.
)

// BuildGrid replays a cast file's output events into a styled terminal cell grid.
// The grid preserves the full scrollback (it grows past the recorded terminal
// height) so renderers can emit complete command output, matching how the
// legacy aha-based screengrab pipeline converted entire streams.
func BuildGrid(path string) (*cellbuf.Buffer, error) {
	defer perf.Track(nil, "asciicast.BuildGrid")()

	header, events, err := ReadEvents(path)
	if err != nil {
		return nil, err
	}
	var sb strings.Builder
	for _, event := range events {
		if event.Stream != "o" && event.Stream != "e" {
			continue
		}
		sb.WriteString(event.Data)
	}
	width := header.Width
	if header.Term != nil && header.Term.Cols > 0 {
		width = header.Term.Cols
	}
	if width <= 0 {
		width = DefaultWidth
	}
	return contentGrid(sb.String(), width), nil
}

// contentGrid lays out pre-recorded terminal output into a cell buffer sized to
// fit the whole content: at least termWidth columns wide, growing wider when a
// line overflows (docs artifacts should never lose content to truncation).
func contentGrid(content string, termWidth int) *cellbuf.Buffer {
	content = sanitizeStream(content)
	width, height := measureContent(content)
	if width < termWidth {
		width = termWidth
	}
	if height < 1 {
		height = 1
	}
	buf := cellbuf.NewBuffer(width, height)
	cellbuf.SetContent(buf, content)
	return buf
}

// sanitizeStream removes escape sequences that carry no printable content and
// would otherwise leak literal bytes into cells: OSC sequences (title changes,
// clipboard), cursor-position replies, and other stray CSI responses. SGR
// styling and OSC-8 hyperlinks are kept for cellbuf to interpret. Tabs are
// expanded to spaces because the cell writer has no tab-stop handling.
func sanitizeStream(content string) string {
	var out strings.Builder
	out.Grow(len(content))
	parser := ansi.GetParser()
	defer ansi.PutParser(parser)

	var state byte
	column := 0
	for len(content) > 0 {
		seq, width, n, newState := ansi.DecodeSequence(content, state, parser)
		if n <= 0 {
			_, size := utf8.DecodeRuneInString(content)
			if size <= 0 {
				size = 1
			}
			content = content[size:]
			state = newState
			continue
		}
		switch {
		case width > 0:
			out.WriteString(seq)
			column += width
		case seq == "\n":
			out.WriteString(seq)
			column = 0
		case seq == "\r":
			out.WriteString(seq)
			column = 0
		case seq == "\t":
			spaces := tabStopInterval - column%tabStopInterval
			out.WriteString(strings.Repeat(" ", spaces))
			column += spaces
		case keepSequence(seq, parser):
			out.WriteString(seq)
		}
		state = newState
		content = content[n:]
	}
	return out.String()
}

// keepSequence reports whether a zero-width escape sequence should survive
// sanitization: SGR styling and OSC-8 hyperlinks are meaningful to the cell
// writer; everything else (OSC titles, cursor movement/replies, mode changes)
// is dropped.
func keepSequence(seq string, parser *ansi.Parser) bool {
	if ansi.HasCsiPrefix(seq) && parser.Command() == 'm' {
		return true
	}
	if ansi.HasOscPrefix(seq) && parser.Command() == oscHyperlinkCmd {
		return true
	}
	return false
}

// measureContent computes the visual width and line count of sanitized
// terminal output. Carriage-return segments overwrite in place, so the widest
// segment bounds the line width.
func measureContent(content string) (width, height int) {
	content = strings.TrimSuffix(content, "\n")
	if content == "" {
		return 0, 0
	}
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		for _, segment := range strings.Split(line, "\r") {
			if w := ansi.StringWidth(segment); w > width {
				width = w
			}
		}
	}
	return width, len(lines)
}
