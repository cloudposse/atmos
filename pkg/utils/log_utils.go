package utils

import (
	"fmt"
	"os"

	ioLayer "github.com/cloudposse/atmos/pkg/io"
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

// PrintfMessageToTUI prints the message to the stderr with automatic secret masking.
func PrintfMessageToTUI(message string, args ...any) {
	fmt.Fprintf(ioLayer.MaskWriter(os.Stderr), message, args...)
}
