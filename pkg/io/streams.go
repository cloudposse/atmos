package io

import (
	stdio "io"
	"os"

	"github.com/cloudposse/atmos/pkg/perf"
)

// streams implements the Streams interface.
// Note: rawOutput/rawError are not cached - RawOutput()/RawError() return os.Stdout/os.Stderr
// dynamically to support test output capture via os.Stdout redirection.
type streams struct {
	input  stdio.Reader
	output stdio.Writer // Masked
	error  stdio.Writer // Masked
	masker Masker
	config *Config
}

// newStreams creates a new Streams with automatic masking.
// Output and error streams use dynamic references to os.Stdout/os.Stderr
// to support test output capture via os.Stdout redirection.
func newStreams(masker Masker, config *Config) Streams {
	s := &streams{
		input:  os.Stdin,
		masker: masker,
		config: config,
	}

	// Wrap output and error with masking (if enabled).
	// Use dynamic writers that resolve os.Stdout/os.Stderr at write time,
	// allowing tests to capture output by redirecting these variables.
	if masker.Enabled() {
		s.output = &dynamicMaskedWriter{
			masker:    masker,
			getWriter: func() stdio.Writer { return os.Stdout },
		}
		s.error = &dynamicMaskedWriter{
			masker:    masker,
			getWriter: func() stdio.Writer { return os.Stderr },
		}
	} else {
		s.output = &dynamicWriter{getWriter: func() stdio.Writer { return os.Stdout }}
		s.error = &dynamicWriter{getWriter: func() stdio.Writer { return os.Stderr }}
	}

	return s
}

func (s *streams) Input() stdio.Reader {
	defer perf.Track(nil, "io.streams.Input")()

	return s.input
}

func (s *streams) Output() stdio.Writer {
	defer perf.Track(nil, "io.streams.Output")()

	return s.output
}

func (s *streams) Error() stdio.Writer {
	defer perf.Track(nil, "io.streams.Error")()

	return s.error
}

func (s *streams) RawOutput() stdio.Writer {
	defer perf.Track(nil, "io.streams.RawOutput")()

	// Return os.Stdout dynamically to support test output capture.
	return os.Stdout
}

func (s *streams) RawError() stdio.Writer {
	defer perf.Track(nil, "io.streams.RawError")()

	// Return os.Stderr dynamically to support test output capture.
	return os.Stderr
}

// dynamicWriter wraps a dynamic writer getter for unmasked output.
// This allows the underlying writer to be resolved at write time,
// supporting test output capture via os.Stdout redirection.
type dynamicWriter struct {
	getWriter func() stdio.Writer
}

// Write implements stdio.Writer by delegating to the dynamic writer.
func (dw *dynamicWriter) Write(p []byte) (n int, err error) {
	defer perf.Track(nil, "io.dynamicWriter.Write")()

	return dw.getWriter().Write(p)
}

// maskedWriter wraps a stdio.Writer and automatically masks sensitive data.
// Used by MaskWriter() to wrap arbitrary writers with masking.
type maskedWriter struct {
	underlying stdio.Writer
	masker     Masker
}

// Write implements stdio.Writer with automatic masking.
func (mw *maskedWriter) Write(p []byte) (n int, err error) {
	defer perf.Track(nil, "io.maskedWriter.Write")()

	input := string(p)
	masked := mw.masker.Mask(input)
	maskedBytes := []byte(masked)

	written, err := mw.underlying.Write(maskedBytes)
	if err != nil {
		return 0, err
	}

	// Check for partial write.
	if written < len(maskedBytes) {
		return 0, stdio.ErrShortWrite
	}

	// Return original length to maintain write semantics.
	return len(p), nil
}

// dynamicMaskedWriter wraps a dynamic writer getter with automatic masking.
// This allows the underlying writer to be resolved at write time,
// supporting test output capture via os.Stdout redirection.
type dynamicMaskedWriter struct {
	masker    Masker
	getWriter func() stdio.Writer
}

// Write implements stdio.Writer with automatic masking and dynamic writer resolution.
func (dmw *dynamicMaskedWriter) Write(p []byte) (n int, err error) {
	defer perf.Track(nil, "io.dynamicMaskedWriter.Write")()

	input := string(p)
	masked := dmw.masker.Mask(input)
	maskedBytes := []byte(masked)

	written, err := dmw.getWriter().Write(maskedBytes)
	if err != nil {
		return 0, err
	}

	// Check for partial write.
	if written < len(maskedBytes) {
		return 0, stdio.ErrShortWrite
	}

	// Return original length to maintain write semantics.
	return len(p), nil
}
