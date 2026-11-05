package io

import (
	stdio "io"
	"os"

	"github.com/cloudposse/atmos/pkg/perf"
)

// ChildProcessOutput preserves file handles when re-executing Atmos, whose child
// process applies its own masking. The os/exec package converts wrapped files into
// pipes, disabling terminal detection and progress indicators. Custom writers
// (including capture/tee writers) are preserved. Do not use for external commands
// that depend on the parent's masking or recording.
func ChildProcessOutput(writer stdio.Writer) stdio.Writer {
	defer perf.Track(nil, "io.ChildProcessOutput")()

	recorderState.mu.RLock()
	recording := recorderState.recorder != nil
	recorderState.mu.RUnlock()
	if recording {
		return writer
	}
	original := writer
	for {
		switch w := writer.(type) {
		case *os.File:
			return w
		case *dynamicWriter:
			writer = w.getWriter()
		case *dynamicMaskedWriter:
			writer = w.getWriter()
		case *maskedWriter:
			writer = w.underlying
		default:
			return original
		}
	}
}
