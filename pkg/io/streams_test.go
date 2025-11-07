package io

import (
	"bytes"
	"testing"
)

func TestStreams_Output(t *testing.T) {
	cfg := &Config{DisableMasking: false}
	masker := newMasker(cfg)
	masker.RegisterValue("secret123")

	streams := newStreams(masker, cfg)

	// Test that Output() returns a masked writer
	_, err := streams.Output().Write([]byte("contains secret123"))
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
}

func TestStreams_RawOutput(t *testing.T) {
	cfg := &Config{DisableMasking: false}
	masker := newMasker(cfg)

	streams := newStreams(masker, cfg)

	// RawOutput should return the underlying stream
	if streams.RawOutput() == nil {
		t.Error("RawOutput() returned nil")
	}
}

func TestMaskedWriter_Write(t *testing.T) {
	cfg := &Config{DisableMasking: false}
	masker := newMasker(cfg)
	masker.RegisterValue("secret123")
	masker.RegisterValue("password456")

	var buf bytes.Buffer
	mw := &maskedWriter{
		underlying: &buf,
		masker:     masker,
	}

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "plain text",
			input: "hello world",
			want:  "hello world",
		},
		{
			name:  "contains secret",
			input: "The secret is secret123",
			want:  "The secret is ***MASKED***",
		},
		{
			name:  "multiple secrets",
			input: "secret123 and password456",
			want:  "***MASKED*** and ***MASKED***",
		},
		{
			name:  "secret at start",
			input: "secret123 is the value",
			want:  "***MASKED*** is the value",
		},
		{
			name:  "secret at end",
			input: "The value is secret123",
			want:  "The value is ***MASKED***",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf.Reset()

			n, err := mw.Write([]byte(tt.input))
			if err != nil {
				t.Fatalf("Write() error = %v", err)
			}

			// Should return original length
			if n != len(tt.input) {
				t.Errorf("Write() returned %d bytes, want %d", n, len(tt.input))
			}

			got := buf.String()
			if got != tt.want {
				t.Errorf("Write() output = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMaskedWriter_WriteLargeData(t *testing.T) {
	cfg := &Config{DisableMasking: false}
	masker := newMasker(cfg)
	masker.RegisterValue("SECRET")

	var buf bytes.Buffer
	mw := &maskedWriter{
		underlying: &buf,
		masker:     masker,
	}

	// Test with large input containing secret
	input := make([]byte, 1024*1024) // 1MB
	copy(input, []byte("PREFIX SECRET SUFFIX"))

	n, err := mw.Write(input)
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	if n != len(input) {
		t.Errorf("Write() returned %d bytes, want %d", n, len(input))
	}

	output := buf.String()
	if bytes.Contains([]byte(output), []byte("SECRET")) {
		t.Error("SECRET was not masked in large data")
	}
}

func TestStreams_DisabledMasking(t *testing.T) {
	cfg := &Config{DisableMasking: true}
	masker := newMasker(cfg)
	masker.RegisterValue("secret123")

	streams := newStreams(masker, cfg)

	// When masking is disabled, Output() should still work
	// but not mask anything

	// We can't directly test stdout, but we can verify the stream exists
	if streams.Output() == nil {
		t.Error("Output() returned nil")
	}

	if streams.Error() == nil {
		t.Error("Error() returned nil")
	}
}

func TestStreams_Input(t *testing.T) {
	cfg := &Config{}
	masker := newMasker(cfg)
	streams := newStreams(masker, cfg)

	// Verify Input() returns stdin
	if streams.Input() == nil {
		t.Error("Input() returned nil")
	}
}

func TestStreams_RawError(t *testing.T) {
	cfg := &Config{}
	masker := newMasker(cfg)
	streams := newStreams(masker, cfg)

	// Verify RawError() returns stderr without masking
	if streams.RawError() == nil {
		t.Error("RawError() returned nil")
	}
}

func TestMaskedWriter_WriteError(t *testing.T) {
	// Test that Write correctly propagates errors from underlying writer
	cfg := &Config{DisableMasking: false}
	masker := newMasker(cfg)

	// Use errorWriter that always returns an error
	errWriter := &errorWriter{}
	mw := &maskedWriter{
		underlying: errWriter,
		masker:     masker,
	}

	_, err := mw.Write([]byte("test"))
	if err == nil {
		t.Error("Write() expected error from underlying writer, got nil")
	}
}

// errorWriter is a writer that always returns an error.
type errorWriter struct{}

func (e *errorWriter) Write(p []byte) (int, error) {
	return 0, bytes.ErrTooLarge
}
