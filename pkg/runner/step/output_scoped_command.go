package step

import (
	"bytes"
	"io"

	iolib "github.com/cloudposse/atmos/pkg/io"
)

// executeScoped captures raw values while streaming masked output to the caller.
// Embedded scripts own their task labels and output synchronization, so terminal
// viewports and process-global step decorations do not wrap these streams.
func (w *OutputModeWriter) executeScoped(runner func(io.Writer, io.Writer) error) (string, string, error) {
	var stdout, stderr bytes.Buffer
	out, diagnostic := w.writers.Stdout, w.writers.Stderr
	if out == nil || w.mode == OutputModeNone {
		out = io.Discard
	}
	if diagnostic == nil || w.mode == OutputModeNone {
		diagnostic = io.Discard
	}
	maskedOut, maskedErr := iolib.NewStreamingMaskWriter(out), iolib.NewStreamingMaskWriter(diagnostic)
	err := runner(io.MultiWriter(&stdout, maskedOut), io.MultiWriter(&stderr, maskedErr))
	flushDisplay(maskedOut, maskedErr)
	return stdout.String(), stderr.String(), err
}
