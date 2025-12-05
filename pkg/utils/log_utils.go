package utils

import (
	"fmt"
	"os"

	"github.com/fatih/color"

	log "github.com/cloudposse/atmos/pkg/logger"
)

const (
	LogLevelTrace = "Trace"
	LogLevelDebug = "Debug"
)

// OsExit is a variable for testing, so we can mock os.Exit.
var OsExit = os.Exit

// PrintMessage prints the message to the console.
func PrintMessage(message string) {
	fmt.Println(message)
}

// PrintMessageInColor prints the message to the console using the provided color.
func PrintMessageInColor(message string, messageColor *color.Color) {
	if _, err := messageColor.Fprint(os.Stdout, message); err != nil {
		log.Trace("Failed to print colored message to stdout", "error", err)
	}
}

// PrintfMessageToTUI prints the message to the stderr.
func PrintfMessageToTUI(message string, args ...any) {
	fmt.Fprintf(os.Stderr, message, args...)
}
