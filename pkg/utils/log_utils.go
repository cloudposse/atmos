package utils

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	log "github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	"github.com/fatih/color"
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

// Deprecated: Use `log.Fatal` instead. This function will be removed in a future release.
// LogErrorAndExit logs errors to std.Error and exits with an error code.
func LogErrorAndExit(err error) {
	log.Error(err)

	// Find the executed command's exit code from the error
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		OsExit(exitError.ExitCode())
	}

	OsExit(1)
}

// Deprecated: Use `log.Error` instead. This function will be removed in a future release.
// LogError logs errors to std.Error
func LogError(err error) {
	if err != nil {
		log.Error(err)
	}
}

// Deprecated: Use `log.Debug` instead. This function will be removed in a future release.
// LogTrace logs the provided trace message
func LogTrace(message string) {
	LogDebug(message)
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
