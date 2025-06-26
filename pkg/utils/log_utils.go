package utils

import (
	"fmt"
	"os"

	log "github.com/charmbracelet/log"
	"github.com/fatih/color"

	"github.com/cloudposse/atmos/pkg/ui/theme"
)

const (
	LogLevelTrace   = "Trace"
	LogLevelDebug   = "Debug"
	LogLevelInfo    = "Info"
	LogLevelWarning = "Warning"
)

// OsExit is a variable for testing so we can mock os.Exit.
var OsExit = os.Exit

// PrintMessage prints the message to the console
func PrintMessage(message string) {
	fmt.Println(message)
}

// PrintMessageInColor prints the message to the console using the provided color
func PrintMessageInColor(message string, messageColor *color.Color) {
	_, _ = messageColor.Fprint(os.Stdout, message)
}

// Deprecated: Use `log.Error` instead. This function will be removed in a future release.
func PrintErrorInColor(message string) {
	messageColor := theme.Colors.Error
	_, _ = messageColor.Fprint(os.Stderr, message)
}

// PrintfMessageToTUI prints the message to the stderr.
func PrintfMessageToTUI(message string, args ...any) {
	fmt.Fprintf(os.Stderr, message, args...)
}

// Deprecated: Use `log.Debug` instead. This function will be removed in a future release.
// LogDebug logs the provided debug message
func LogDebug(message string) {
	log.Debug(message)
}

// Deprecated: Use `log.Info` instead. This function will be removed in a future release.
// LogInfo logs the provided info message
func LogInfo(message string) {
	log.Info(message)
}

// Deprecated: Use `log.Warn` instead. This function will be removed in a future release.
// LogWarning logs the provided warning message
func LogWarning(message string) {
	log.Warn(message)
}
