package utils

import (
	"fmt"
	"os"

	"github.com/fatih/color"
)

const (
	LogLevelTrace   = "Trace"
	LogLevelDebug   = "Debug"
	LogLevelInfo    = "Info"
	LogLevelWarning = "Warning"
)

// OsExit is a variable for testing, so we can mock os.Exit.
var OsExit = os.Exit

// PrintMessage prints the message to the console
func PrintMessage(message string) {
	fmt.Println(message)
}

// PrintMessageInColor prints the message to the console using the provided color
func PrintMessageInColor(message string, messageColor *color.Color) {
	_, _ = messageColor.Fprint(os.Stdout, message)
}

// PrintfMessageToTUI prints the message to the stderr.
func PrintfMessageToTUI(message string, args ...any) {
	fmt.Fprintf(os.Stderr, message, args...)
}
