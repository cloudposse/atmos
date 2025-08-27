//go:build !linting
// +build !linting

package main

import (
	"errors"
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
	"github.com/muesli/termenv"
)

// Predefined reusable errors.
var (
	ErrNotFound = NewBaseError("not found")
	ErrUsage    = NewBaseError("incorrect usage")
	ErrExit     = NewBaseError("command exited non-zero")
)

func main() {
	// Initialize structured logger
	baseLogger := log.NewWithOptions(os.Stdout, log.Options{
		Level: TraceLevel, // Change this to control log filtering
	})

	// Wrap base logger in AtmosLogger
	logger := &AtmosLogger{Logger: baseLogger}

	// 🟢 Ensure Lipgloss uses terminal colors
	lipgloss.SetColorProfile(termenv.TrueColor)

	// 🟢 Define styles for different log levels
	styles := log.DefaultStyles()

	// 🟢 Custom style for TRACE level
	styles.Levels[TraceLevel] = lipgloss.NewStyle().
		SetString("TRCE").
		Padding(0, 0).
		Background(lipgloss.Color("242")). // Gray background
		Foreground(lipgloss.Color("15"))   // White text

	// 🟢 Apply styles
	logger.SetStyles(styles)
	logger.SetColorProfile(termenv.TrueColor)

	// 🔹 Log messages with different levels
	logger.Trace("Tracing execution flow...", "function", "main", "step", 1)
	logger.Debug("Debugging an issue...", "module", "auth")
	logger.Info("Starting Atmos CLI...", "version", "1.0.0")
	logger.Warn("Potential misconfiguration detected", "config", "atmos.yaml")
	logger.Error("Fatal error occurred", "reason", "connection timeout")

	// 🛠 Create an enriched error from a base error
	err := ErrUsage.WithContext("Unknown command", "flag", "--foo", "subcommand", "bar").
		WithTips("Use --help to see valid options", "Check the documentation at https://example.com").
		WithExitCode(2)

	// We can check the type of error and handle it accordingly
	// Rather than matching on the error message, we can use errors.Is
	if errors.Is(err, ErrUsage) {
		// ✅ Just log the error like normal, but metadata is included!
		logger.Fatal(err)
	}

	logger.Error(err)
}
