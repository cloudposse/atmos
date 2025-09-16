package output

import (
	"fmt"
	"io"
	"os"

	"github.com/cloudposse/atmos/tools/gotcha/pkg/config"
	"github.com/spf13/cobra"
)

// Writer manages output streams for the application.
// It provides a clean abstraction for handling stdout/stderr,
// with special handling for CI environments to prevent stream interleaving.
type Writer struct {
	// Data is for machine-readable output/results that can be piped
	Data io.Writer

	// UI is for human-readable UI messages, progress, and status
	UI io.Writer

	// forceUnified indicates all output should go to one stream
	forceUnified bool
}

// New creates a Writer based on the current environment.
// In GitHub Actions, it unifies streams to prevent interleaving unless
// GOTCHA_SPLIT_STREAMS is set.
func New() *Writer {
	// In GitHub Actions, unify streams to prevent interleaving
	// Unless user explicitly wants split streams
	if config.IsGitHubActions() && os.Getenv("GOTCHA_SPLIT_STREAMS") == "" {
		return &Writer{
			Data:         os.Stdout,
			UI:           os.Stdout,
			forceUnified: true,
		}
	}

	// Standard terminal mode: split streams
	// UI goes to stderr (can be hidden with 2>/dev/null)
	// Data goes to stdout (can be piped)
	return &Writer{
		Data:         os.Stdout,
		UI:           os.Stderr,
		forceUnified: false,
	}
}

// NewCustom creates a Writer with custom streams (primarily for testing).
func NewCustom(data, ui io.Writer) *Writer {
	return &Writer{
		Data:         data,
		UI:           ui,
		forceUnified: false,
	}
}

// ConfigureCommand sets up a Cobra command with this writer's streams.
func (w *Writer) ConfigureCommand(cmd *cobra.Command) {
	cmd.SetOut(w.Data)
	cmd.SetErr(w.UI)
}

// PrintUI writes UI output (progress, status messages, errors).
func (w *Writer) PrintUI(format string, args ...interface{}) {
	fmt.Fprintf(w.UI, format, args...)
}

// PrintData writes data output (results that might be piped or processed).
func (w *Writer) PrintData(format string, args ...interface{}) {
	fmt.Fprintf(w.Data, format, args...)
}

// FprintUI writes UI output to the UI stream.
func (w *Writer) FprintUI(format string, args ...interface{}) (n int, err error) {
	return fmt.Fprintf(w.UI, format, args...)
}

// FprintData writes data output to the Data stream.
func (w *Writer) FprintData(format string, args ...interface{}) (n int, err error) {
	return fmt.Fprintf(w.Data, format, args...)
}

// IsUnified returns true if all output goes to one stream.
// This is typically true in CI environments to prevent interleaving.
func (w *Writer) IsUnified() bool {
	return w.forceUnified
}
