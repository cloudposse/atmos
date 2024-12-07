package utils

import (
	"bytes"
	"fmt"
	"os"
	"sync"

	"github.com/alecthomas/chroma/quick"
	"github.com/arsham/figurine/figurine"
	"github.com/jwalton/go-supportscolor"
)

// HighlightCode returns a syntax highlighted code for the specified language
func HighlightCode(code string, language string, syntaxTheme string) (string, error) {
	buf := new(bytes.Buffer)
	if err := quick.Highlight(buf, code, language, "terminal256", syntaxTheme); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// logoOnce ensures the ATMOS logo is printed exactly once
var logoOnce sync.Once

// PrintStyledText prints a styled text to the terminal
func PrintStyledText(text string) error {
	// Check if the terminal supports colors
	if supportscolor.Stdout().SupportsColor {
		return figurine.Write(os.Stdout, text, "ANSI Regular.flf")
	}
	return nil
}

// logoOnce ensures thread-safe single execution of logo display
func PrintAtmosLogo() error {
	var err error
	logoOnce.Do(func() {
		fmt.Println()
		err = PrintStyledText("ATMOS")
	})
	return err
}
