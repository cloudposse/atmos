package io

import (
	stdio "io"
	"sync"

	"github.com/cloudposse/atmos/pkg/perf"
)

// uiSinkOverride, when non-nil, replaces the UI-stream (stderr) writer for all ui.*
// output. It is consulted by context.Write for the UIStream branch. Access is guarded
// by globalMu (declared in global.go).
var uiSinkOverride stdio.Writer

// SuppressUI temporarily routes UI-stream output (ui.* writes) to io.Discard, returning
// a restore func. Unlike swapping the process-global os.Stderr, this affects ONLY the
// pkg/io UI stream — os.Stderr, stdout, the logger, panics, and the Go runtime are
// untouched. The restore func is idempotent (safe to call more than once) and safe to
// defer.
//
// Intended for full-screen TUIs that own the terminal and must silence mid-render ui.*
// writes that would corrupt a sticky spinner.
func SuppressUI() func() {
	defer perf.Track(nil, "io.SuppressUI")()

	return PushUIWriter(stdio.Discard)
}

// PushUIWriter temporarily redirects UI-stream output to w, returning an idempotent
// restore func that reinstates the previous sink.
func PushUIWriter(w stdio.Writer) func() {
	defer perf.Track(nil, "io.PushUIWriter")()

	globalMu.Lock()
	prev := uiSinkOverride
	uiSinkOverride = w
	globalMu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			globalMu.Lock()
			uiSinkOverride = prev
			globalMu.Unlock()
		})
	}
}

// uiWriterOverride returns the current UI-stream override sink, or nil if none is set.
func uiWriterOverride() stdio.Writer {
	globalMu.RLock()
	defer globalMu.RUnlock()

	return uiSinkOverride
}
