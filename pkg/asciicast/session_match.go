package asciicast

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// waitMatchEnd returns the end of the first matching literal or regular expression.
func waitMatchEnd(text, literal string, expression *regexp.Regexp) (int, bool) {
	if literal != "" {
		if start := strings.Index(text, literal); start >= 0 {
			return start + len(literal), true
		}
	}
	if expression != nil {
		if match := expression.FindStringIndex(text); match != nil {
			return match[1], true
		}
	}
	return 0, false
}

// rawTerminalOffset maps a normalized byte boundary back into the terminal stream.
// The ANSI parser counts printed runes and controls while skipping escape sequences.
func rawTerminalOffset(raw string, plainEnd int) int {
	if plainEnd == 0 {
		return 0
	}
	parser := ansi.GetParser()
	defer func() {
		parser.SetHandler(ansi.Handler{})
		ansi.PutParser(parser)
	}()
	plainBytes := 0
	parser.SetHandler(ansi.Handler{
		Print:   func(r rune) { plainBytes += utf8.RuneLen(r) },
		Execute: func(_ byte) { plainBytes++ },
	})
	for i := range len(raw) {
		parser.Advance(raw[i])
		if plainBytes >= plainEnd {
			return i + 1
		}
	}
	return len(raw)
}
