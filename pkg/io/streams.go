package io

import (
	stdio "io"
	"os"

	"github.com/cloudposse/atmos/pkg/perf"
)

// streams implements the Streams interface.
type streams struct {
	input     stdio.Reader
	output    stdio.Writer // Masked
	error     stdio.Writer // Masked
	rawOutput stdio.Writer // Unmasked
	rawError  stdio.Writer // Unmasked
	masker    Masker
	config    *Config
}

// newStreams creates a new Streams with automatic masking.
func newStreams(masker Masker, config *Config) Streams {
	s := &streams{
		input:     os.Stdin,
		rawOutput: os.Stdout,
		rawError:  os.Stderr,
		masker:    masker,
		config:    config,
	}

	// Wrap output and error with masking (if enabled)
	if masker.Enabled() {
		s.output = &maskedWriter{
			underlying: os.Stdout,
			masker:     masker,
		}
		s.error = &maskedWriter{
			underlying: os.Stderr,
			masker:     masker,
		}
	} else {
		s.output = os.Stdout
		s.error = os.Stderr
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

	return s.rawOutput
}

func (s *streams) RawError() stdio.Writer {
	defer perf.Track(nil, "io.streams.RawError")()

	return s.rawError
}

// maskedWriter wraps a stdio.Writer and automatically masks sensitive data.
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

	// Check for partial write
	if written < len(maskedBytes) {
		return 0, stdio.ErrShortWrite
	}

	// Return original length to maintain write semantics
	return len(p), nil
}
