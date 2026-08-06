package io

import (
	"bytes"
	"errors"
	stdio "io"
	"os"
	"runtime"
	"testing"

	xterm "github.com/charmbracelet/x/term"
	"github.com/creack/pty"
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
			want:  "The secret is <MASKED>",
		},
		{
			name:  "multiple secrets",
			input: "secret123 and password456",
			want:  "<MASKED> and <MASKED>",
		},
		{
			name:  "secret at start",
			input: "secret123 is the value",
			want:  "<MASKED> is the value",
		},
		{
			name:  "secret at end",
			input: "The value is secret123",
			want:  "The value is <MASKED>",
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

// shortWriteWriter is a writer that always reports fewer bytes written than
// provided, without returning an error itself.
type shortWriteWriter struct {
	buf bytes.Buffer
}

func (s *shortWriteWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	n, err := s.buf.Write(p[:len(p)-1])
	return n, err
}

func TestDynamicWriter_Write(t *testing.T) {
	t.Run("full write succeeds", func(t *testing.T) {
		var buf bytes.Buffer
		dw := &dynamicWriter{
			stream:    DataStream,
			getWriter: func() stdio.Writer { return &buf },
		}

		n, err := dw.Write([]byte("hello"))
		if err != nil {
			t.Fatalf("Write() error = %v", err)
		}
		if n != 5 {
			t.Errorf("Write() returned %d, want 5", n)
		}
		if buf.String() != "hello" {
			t.Errorf("buf = %q, want %q", buf.String(), "hello")
		}
	})

	t.Run("underlying writer error propagates", func(t *testing.T) {
		dw := &dynamicWriter{
			stream:    DataStream,
			getWriter: func() stdio.Writer { return &errorWriter{} },
		}

		_, err := dw.Write([]byte("hello"))
		if err == nil {
			t.Fatal("Write() expected error, got nil")
		}
	})

	t.Run("short write returns ErrShortWrite", func(t *testing.T) {
		sw := &shortWriteWriter{}
		dw := &dynamicWriter{
			stream:    DataStream,
			getWriter: func() stdio.Writer { return sw },
		}

		_, err := dw.Write([]byte("hello"))
		if err == nil {
			t.Fatal("Write() expected ErrShortWrite, got nil")
		}
	})
}

// TestMaskedWriter_TermFileCapability verifies that *maskedWriter transparently
// exposes the file-descriptor/terminal-ness of a real *os.File it wraps (the
// fix for Bubble Tea's TTY detection silently failing on a masked os.Stdout;
// see internal/exec/vendor_model.go), while remaining safe/inert for
// non-file underlying writers like a bytes.Buffer.
func TestMaskedWriter_TermFileCapability(t *testing.T) {
	cfg := &Config{DisableMasking: false}
	masker := newMasker(cfg)

	t.Run("wrapping a real pty file satisfies term.File and reports as a terminal", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("pty not supported on Windows")
		}

		ptmx, tty, err := pty.Open()
		if err != nil {
			t.Skipf("pty not available in this environment: %v", err)
		}
		defer ptmx.Close()
		defer tty.Close()

		mw := &maskedWriter{underlying: tty, masker: masker}

		// Compile-time + runtime proof that *maskedWriter satisfies term.File
		// (io.ReadWriteCloser + Fd() uintptr) once it wraps a real file.
		var tf xterm.File = mw

		if tf.Fd() != tty.Fd() {
			t.Errorf("Fd() = %d, want delegated fd %d", tf.Fd(), tty.Fd())
		}
		if !xterm.IsTerminal(tf.Fd()) {
			t.Error("expected masked writer wrapping a pty slave to report as a terminal")
		}
	})

	t.Run("wrapping a bytes.Buffer never crashes and reports non-terminal", func(t *testing.T) {
		var buf bytes.Buffer
		mw := &maskedWriter{underlying: &buf, masker: masker}

		fd := mw.Fd()
		if fd == 0 {
			t.Error("Fd() must never return 0 for a non-file underlying writer: 0 is stdin's real fd and would alias it")
		}
		if fd != invalidFd {
			t.Errorf("Fd() = %d, want invalidFd sentinel %d", fd, invalidFd)
		}
		if xterm.IsTerminal(fd) {
			t.Error("expected masked writer wrapping a bytes.Buffer to report as a non-terminal")
		}

		n, err := mw.Read(make([]byte, 10))
		if n != 0 || !errors.Is(err, stdio.EOF) {
			t.Errorf("Read() = (%d, %v), want (0, io.EOF)", n, err)
		}

		if err := mw.Close(); err != nil {
			t.Errorf("Close() = %v, want nil", err)
		}
	})

	t.Run("Close() on a masked os.Stdout does not close the real file", func(t *testing.T) {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatalf("os.Pipe() error = %v", err)
		}
		defer r.Close()

		origStdout := os.Stdout
		os.Stdout = w
		defer func() { os.Stdout = origStdout }()

		mw := &maskedWriter{underlying: os.Stdout, masker: masker}
		if err := mw.Close(); err != nil {
			t.Fatalf("Close() = %v, want nil", err)
		}

		// If Close() had actually closed os.Stdout (the pipe's write end here),
		// this write would fail with a "file already closed" error.
		if _, err := os.Stdout.Write([]byte("still open")); err != nil {
			t.Errorf("os.Stdout is no longer writable after maskedWriter.Close(): %v", err)
		}
	})

	t.Run("Close() delegates to a real (non-stdout/stderr) underlying Closer", func(t *testing.T) {
		f, err := os.CreateTemp(t.TempDir(), "masked-writer-close-*")
		if err != nil {
			t.Fatalf("os.CreateTemp() error = %v", err)
		}

		mw := &maskedWriter{underlying: f, masker: masker}
		if err := mw.Close(); err != nil {
			t.Fatalf("Close() = %v, want nil", err)
		}

		// The underlying file must have actually been closed this time (unlike os.Stdout/Stderr).
		if _, err := f.Write([]byte("x")); err == nil {
			t.Error("expected the underlying file to be closed after maskedWriter.Close()")
		}
	})

	t.Run("Read() returns EOF for an underlying writer with no Read method at all", func(t *testing.T) {
		mw := &maskedWriter{underlying: &errorWriter{}, masker: masker}

		n, err := mw.Read(make([]byte, 10))
		if n != 0 || !errors.Is(err, stdio.EOF) {
			t.Errorf("Read() = (%d, %v), want (0, io.EOF)", n, err)
		}
	})

	t.Run("Read() delegates to an underlying reader when present", func(t *testing.T) {
		src := bytes.NewBufferString("hello")
		mw := &maskedWriter{underlying: readWriteStub{Reader: src}, masker: masker}

		buf := make([]byte, 5)
		n, err := mw.Read(buf)
		if err != nil {
			t.Fatalf("Read() error = %v", err)
		}
		if string(buf[:n]) != "hello" {
			t.Errorf("Read() = %q, want %q", buf[:n], "hello")
		}
	})
}

// readWriteStub adapts a stdio.Reader into something that also implements
// Write (required to satisfy maskedWriter.underlying's stdio.Writer field)
// purely for exercising maskedWriter.Read()'s delegation path in tests.
type readWriteStub struct {
	stdio.Reader
}

func (readWriteStub) Write(p []byte) (int, error) {
	return len(p), nil
}
