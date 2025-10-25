package io

import (
	stdio "io"
	"os"
)

// streams implements the Streams interface.
type streams struct {
	input      stdio.Reader
	output     stdio.Writer // Masked
	error      stdio.Writer // Masked
	rawOutput  stdio.Writer // Unmasked
	rawError   stdio.Writer // Unmasked
	masker     Masker
	config     *Config
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
	return s.input
}

func (s *streams) Output() stdio.Writer {
	return s.output
}

func (s *streams) Error() stdio.Writer {
	return s.error
}

func (s *streams) RawOutput() stdio.Writer {
	return s.rawOutput
}

func (s *streams) RawError() stdio.Writer {
	return s.rawError
}

// maskedWriter wraps a stdio.Writer and automatically masks sensitive data.
type maskedWriter struct {
	underlying stdio.Writer
	masker     Masker
}

// Write implements stdio.Writer with automatic masking.
func (mw *maskedWriter) Write(p []byte) (n int, err error) {
	input := string(p)
	masked := mw.masker.Mask(input)

	written, err := mw.underlying.Write([]byte(masked))
	if err != nil {
		return written, err
	}

	// Return original length to maintain write semantics
	return len(p), nil
}
