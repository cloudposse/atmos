package utils

import (
	"bytes"
	"os"

	"github.com/alecthomas/chroma/quick"
	"github.com/arsham/figurine/figurine"
)

// HighlightCode returns a syntax highlighted code for the specified language
func HighlightCode(code string, language string, syntaxTheme string) (string, error) {
	buf := new(bytes.Buffer)
	if err := quick.Highlight(buf, code, language, "terminal256", syntaxTheme); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// PrintDecoratedText prints a decorated text to the terminal
func PrintDecoratedText(text string) error {
	return figurine.Write(os.Stdout, text, "ANSI Regular.flf")
}
