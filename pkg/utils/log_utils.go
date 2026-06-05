package utils

import (
	"fmt"
	"os"

	"github.com/fatih/color"

	ioLayer "github.com/cloudposse/atmos/pkg/io"
	log "github.com/cloudposse/atmos/pkg/logger"
)

const (
	LogLevelTrace = "Trace"
	LogLevelDebug = "Debug"
)

// OsExit is a variable for testing, so we can mock os.Exit.
var OsExit = os.Exit

// PrintMessage prints the message to the console with automatic secret masking.
func PrintMessage(message string) {
	fmt.Fprintln(ioLayer.MaskWriter(os.Stdout), message)
}

// PrintMessageInColor prints the message to the console using the provided color with automatic secret masking.
func PrintMessageInColor(message string, messageColor *color.Color) {
	if _, err := messageColor.Fprint(ioLayer.MaskWriter(os.Stdout), message); err != nil {
		log.Trace("Failed to print colored message to stdout", "error", err)
	}
}

// PrintfMessageToTUI prints the message to the stderr with automatic secret masking.
func PrintfMessageToTUI(message string, args ...any) {
	fmt.Fprintf(ioLayer.MaskWriter(os.Stderr), message, args...)
}
