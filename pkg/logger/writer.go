package logger

import (
	"io"
	"os"
	"reflect"

	charm "github.com/charmbracelet/log"
	"github.com/muesli/termenv"
)

// opaqueWriter forwards Write calls to an underlying writer without exposing
// the underlying writer's file descriptor.
//
// The charm logger builds a lipgloss renderer around its output writer, and the
// renderer (via termenv) eagerly queries the terminal for its foreground and
// background colors (OSC 10 and OSC 11) whenever the writer is an *os.File
// attached to a TTY. When the other end of the PTY never answers, each query
// blocks for termenv's fixed 5 second timeout, which delays every Atmos
// invocation. Hiding the file descriptor makes termenv treat the writer as a
// non-TTY, so no query is ever issued. The color profile is then set explicitly
// from our own detection (see profileFor).
type opaqueWriter struct {
	// w is the destination. When nil, os.Stderr is resolved at write time.
	w io.Writer
}

// Write forwards p to the destination writer.
func (o opaqueWriter) Write(p []byte) (int, error) {
	if o.w == nil {
		return os.Stderr.Write(p)
	}
	return o.w.Write(p)
}

// hasFd reports whether the writer exposes a file descriptor, which is what makes
// termenv consider it a potential TTY.
func hasFd(w io.Writer) bool {
	_, ok := w.(interface{ Fd() uintptr })
	return ok
}

// profileFor returns the color profile for the writer using termenv's environment
// detection (TTY check, NO_COLOR, CLICOLOR, CLICOLOR_FORCE, COLORTERM, TERM).
// This is the same profile the charm logger would have derived on its own, but it
// is computed without the color cache, so no terminal queries are sent.
func profileFor(w io.Writer) termenv.Profile {
	return termenv.NewOutput(w).Profile
}

// newOutputWriter returns the writer to hand to the charm logger and, when the
// destination could be a TTY, the color profile to apply to the logger.
// The returned profile is nil when the writer was already opaque or is not file-like.
func newOutputWriter(w io.Writer) (io.Writer, *termenv.Profile) {
	if w == nil {
		w = os.Stderr
	}
	if !hasFd(w) {
		return w, nil
	}
	profile := profileFor(w)
	if w == io.Writer(os.Stderr) {
		// Resolve os.Stderr lazily so a replaced os.Stderr is honored.
		return opaqueWriter{}, &profile
	}
	// Charm retains renderers keyed by the output writer. A comparable value
	// preserves the destination identity across New and SetOutput calls without
	// a second global registry or exposing the file descriptor.
	if reflect.ValueOf(w).Comparable() {
		return opaqueWriter{w: w}, &profile
	}
	// Uncomparable custom writers still need a comparable registry key.
	return &opaqueWriter{w: w}, &profile
}

// newCharmLogger creates a charm logger bound to w without triggering terminal
// color queries, with a color profile derived from the real destination.
func newCharmLogger(w io.Writer, reportTimestamp bool) *charm.Logger {
	out, profile := newOutputWriter(w)
	l := charm.NewWithOptions(out, charm.Options{ReportTimestamp: reportTimestamp})
	if profile != nil {
		l.SetColorProfile(*profile)
	}
	return l
}

// setCharmOutput sets the logger output without triggering terminal color queries.
func setCharmOutput(l *charm.Logger, w io.Writer) {
	out, profile := newOutputWriter(w)
	l.SetOutput(out)
	if profile != nil {
		l.SetColorProfile(*profile)
	}
}
