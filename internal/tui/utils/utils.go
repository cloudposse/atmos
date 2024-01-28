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

// PrintAtmosLogo prints a styled Atmos logo to the terminal
func PrintAtmosLogo() error {
	return figurine.Write(os.Stdout, "ATMOS", "ANSI Regular.flf")
}
