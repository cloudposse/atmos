package utils

import (
	"bytes"

	"github.com/alecthomas/chroma/quick"
)

// HighlightText returns a syntax highlighted string of text
func HighlightText(content, extension, syntaxTheme string) (string, error) {
	buf := new(bytes.Buffer)
	if err := quick.Highlight(buf, content, extension, "terminal256", syntaxTheme); err != nil {
		return "", err
	}

	return buf.String(), nil
}
