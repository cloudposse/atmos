package io

import (
	"sync"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Recorder receives already-masked terminal events from the I/O layer.
type Recorder interface {
	Record(stream, content string)
}

var recorderState struct {
	mu       sync.RWMutex
	recorder Recorder
}

// SetRecorder installs a process-wide I/O recorder and returns a restore function.
func SetRecorder(rec Recorder) func() {
	defer perf.Track(nil, "io.SetRecorder")()

	recorderState.mu.Lock()
	previous := recorderState.recorder
	recorderState.recorder = rec
	recorderState.mu.Unlock()

	return func() {
		recorderState.mu.Lock()
		if recorderState.recorder == rec {
			recorderState.recorder = previous
		}
		recorderState.mu.Unlock()
	}
}

func recordOutput(stream Stream, content string) {
	recorderState.mu.RLock()
	rec := recorderState.recorder
	recorderState.mu.RUnlock()
	if rec == nil || content == "" {
		return
	}
	switch stream {
	case DataStream:
		rec.Record("o", content)
	case UIStream:
		rec.Record("e", content)
	}
}

// RecordMaskedOutput records content that has already been routed to a stream.
// Use this only from stream adapters that perform their own writing and masking
// outside Context.Write, such as PTY bridges.
func RecordMaskedOutput(stream Stream, content string) {
	defer perf.Track(nil, "io.RecordMaskedOutput")()

	recordOutput(stream, content)
}
