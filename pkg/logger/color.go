package logger

import (
	"io"
	"os"
	"sync"
	"sync/atomic"

	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

// colorPolicy is shared by derived loggers and their output writers.
type colorPolicy struct {
	mu          sync.Mutex
	logDisabled bool
	disabled    bool
	profile     termenv.Profile
	profileSet  bool
	// downgraded records that the renderer was forced to Ascii by an opt-out, so a later
	// re-enable without an explicit profile must restore automatic detection.
	downgraded bool
	// out is the underlying writer (before the ANSI filter) used to re-detect the profile.
	out   io.Writer
	plain atomic.Bool
}

// SetColorEnabled controls logging color independently of the UI profile.
// Enabled logs follow terminal detection; disabled is the global color veto.
func (l *AtmosLogger) SetColorEnabled(enabled bool, disabled bool) {
	l.color.mu.Lock()
	defer l.color.mu.Unlock()
	l.color.logDisabled = !enabled
	l.color.disabled = disabled
	l.applyColorProfile()
}

// applyColorProfile requires color.mu to be held.
func (l *AtmosLogger) applyColorProfile() {
	profile := l.color.profile
	switch {
	case l.color.disabled || l.color.logDisabled:
		profile = termenv.Ascii
		l.color.downgraded = true
	case !l.color.profileSet:
		if l.color.downgraded {
			// An earlier opt-out forced the renderer to Ascii; restore automatic detection.
			l.color.downgraded = false
			l.color.plain.Store(false)
			l.charm.SetColorProfile(detectColorProfile(l.color.out))
		}
		return // Otherwise preserve automatic detection until a terminal profile is available.
	}
	l.color.plain.Store(profile == termenv.Ascii)
	l.charm.SetColorProfile(profile)
}

// detectColorProfile mirrors the automatic detection Charm performs when it builds a renderer.
func detectColorProfile(w io.Writer) termenv.Profile {
	if w == nil {
		w = os.Stderr
	}
	return termenv.NewOutput(w).EnvColorProfile()
}

// colorWriter strips preformatted ANSI as well as renderer-generated styling.
// Charm writes complete records; the filter does not alter the original writer.
type colorWriter struct {
	io.Writer
	policy *colorPolicy
}

// Fd preserves terminal detection through the logging filter.
func (w *colorWriter) Fd() uintptr {
	if file, ok := w.Writer.(interface{ Fd() uintptr }); ok {
		return file.Fd()
	}
	return ^uintptr(0)
}

// Read preserves termenv.File compatibility for writers backed by a terminal.
func (w *colorWriter) Read(p []byte) (int, error) {
	if reader, ok := w.Writer.(io.Reader); ok {
		return reader.Read(p)
	}
	return 0, io.EOF
}

// Write filters ANSI when the shared policy requires plain output and reports the
// original input length on success, preserving io.Writer semantics after filtering.
func (w *colorWriter) Write(p []byte) (int, error) {
	if !w.policy.plain.Load() {
		return w.Writer.Write(p)
	}
	plain := []byte(ansi.Strip(string(p)))
	n, err := w.Writer.Write(plain)
	if err == nil && n != len(plain) {
		err = io.ErrShortWrite
	}
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

// setColorOutput wraps a new destination with the shared ANSI filter and reapplies
// opt-outs after Charm replaces its renderer and detects the destination's color support.
func (l *AtmosLogger) setColorOutput(w io.Writer) {
	l.color.mu.Lock()
	defer l.color.mu.Unlock()
	if w == nil {
		w = os.Stderr
	}
	l.color.out = w
	if w != io.Discard {
		w = &colorWriter{Writer: w, policy: l.color}
	}
	l.charm.SetOutput(w)
	// An explicit profile describes the previous destination, so the new renderer keeps
	// Charm's automatic detection. Callers that force color reapply it after SetOutput.
	l.color.profileSet = false
	l.color.downgraded = false
	l.color.plain.Store(false)
	// SetOutput replaces Charm's renderer, so always reapply the opt-out policy afterwards.
	l.applyColorProfile()
}
