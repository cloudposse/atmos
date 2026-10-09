package logger

import (
	"bytes"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/muesli/termenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoggingColorPolicy(t *testing.T) {
	for _, tt := range []struct {
		name      string
		enabled   bool
		disabled  bool
		profile   termenv.Profile
		wantColor bool
	}{
		{"enabled retains CI color", true, false, termenv.TrueColor, true},
		{"enabled plain terminal", true, false, termenv.Ascii, false},
		{"false overrides color UI", false, false, termenv.TrueColor, false},
		{"true respects plain UI", true, false, termenv.Ascii, false},
		{"global veto overrides true", true, true, termenv.TrueColor, false},
		{"global veto overrides false", false, true, termenv.TrueColor, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			logger := New()
			logger.SetLevel(DebugLevel)
			logger.SetColorEnabled(tt.enabled, tt.disabled)
			logger.SetColorProfile(tt.profile)
			for range 2 {
				var output bytes.Buffer
				logger.SetOutput(&output)
				// UI initialization after SetOutput must not undo logging overrides.
				logger.SetColorProfile(tt.profile)
				logger.With("source", "startup").WithPrefix("test").Debug("record", "key", "value")
				require.Contains(t, output.String(), "record")
				assert.Equal(t, tt.wantColor, bytes.Contains(output.Bytes(), []byte("\x1b")))
			}
		})
	}
}

func TestLoggingColorStripsPreformattedRecords(t *testing.T) {
	logger := New()
	logger.SetColorEnabled(false, false)
	var output bytes.Buffer
	logger.SetOutput(&output)
	logger.With("field", "\x1b[32mvalue\x1b[0m").Error("\x1b[31mmessage\x1b[0m", "link", "\x1b]8;;https://example.com\x1b\\label\x1b]8;;\x1b\\")
	assert.NotContains(t, output.String(), "\x1b")
	assert.Contains(t, output.String(), "message")
	assert.Contains(t, output.String(), "value")
	assert.Contains(t, output.String(), "label")
	logger.SetColorEnabled(true, false)
	logger.SetColorProfile(termenv.TrueColor)
	output.Reset()
	logger.Error("colored again")
	assert.Contains(t, output.String(), "\x1b")
}

func TestDerivedLoggerHonorsLaterOptOut(t *testing.T) {
	logger := New()
	logger.SetColorProfile(termenv.TrueColor)
	derived := logger.With("source", "vendor")
	var output bytes.Buffer
	derived.SetOutput(&output)
	logger.SetColorEnabled(false, false)
	derived.Error("\x1b[31mmessage\x1b[0m")
	assert.Contains(t, output.String(), "message")
	assert.NotContains(t, output.String(), "\x1b")
}

type failingColorWriter struct{ err error }

func (w failingColorWriter) Write([]byte) (int, error) { return 0, w.err }

func TestColorWriterErrors(t *testing.T) {
	policy := &colorPolicy{}
	policy.plain.Store(true)
	writeErr := errors.New("write failed")
	for _, err := range []error{nil, writeErr} {
		writer := &colorWriter{Writer: failingColorWriter{err: err}, policy: policy}
		n, got := writer.Write([]byte("\x1b[31mmessage\x1b[0m"))
		assert.Zero(t, n)
		if err == nil {
			assert.ErrorIs(t, got, io.ErrShortWrite)
		} else {
			assert.ErrorIs(t, got, writeErr)
		}
	}
}

func TestColorWriterPreservesFileCapabilities(t *testing.T) {
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() { _ = reader.Close(); _ = writer.Close() })
	filter := &colorWriter{Writer: writer, policy: &colorPolicy{}}
	var _ interface {
		io.ReadWriter
		Fd() uintptr
	} = filter
	assert.Equal(t, writer.Fd(), filter.Fd())
	_, err = filter.Write([]byte("hello"))
	require.NoError(t, err)
	input := &colorWriter{Writer: reader, policy: &colorPolicy{}}
	buf := make([]byte, 5)
	_, err = io.ReadFull(input, buf)
	require.NoError(t, err)
	assert.Equal(t, "hello", string(buf))
	nonFile := &colorWriter{Writer: io.Discard, policy: &colorPolicy{}}
	assert.Equal(t, ^uintptr(0), nonFile.Fd())
	_, err = nonFile.Read(buf)
	assert.ErrorIs(t, err, io.EOF)
}
