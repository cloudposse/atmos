package step

import (
	"context"
	"errors"
	stdio "io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
)

// failAfterNWriter succeeds for the first `remaining` writes, then returns an
// error for every write after that. This lets tests deterministically drive
// recordCastTypedText's write-error branches (the style-prefix write and the
// per-character write inside the typing loop) without depending on real I/O
// failures.
type failAfterNWriter struct {
	remaining int
}

func (w *failAfterNWriter) Write(p []byte) (int, error) {
	if w.remaining > 0 {
		w.remaining--
		return len(p), nil
	}
	return 0, errors.New("simulated write failure")
}

// failStreams is a minimal iolib.Streams implementation that routes all
// output through a failAfterNWriter.
type failStreams struct {
	out *failAfterNWriter
}

func (s *failStreams) Input() stdio.Reader     { return nil }
func (s *failStreams) Output() stdio.Writer    { return s.out }
func (s *failStreams) Error() stdio.Writer     { return s.out }
func (s *failStreams) RawOutput() stdio.Writer { return s.out }
func (s *failStreams) RawError() stdio.Writer  { return s.out }

// withFailingDataWriter installs a data writer that fails after
// allowedWrites successful writes, and restores a working writer afterward
// so later tests in this package are not affected (mirrors the pattern used
// by output_mode_execution_test.go's initOutputModeTestIO helper).
func withFailingDataWriter(t *testing.T, allowedWrites int) {
	t.Helper()

	failCtx, err := iolib.NewContext(iolib.WithStreams(&failStreams{out: &failAfterNWriter{remaining: allowedWrites}}))
	require.NoError(t, err)
	data.InitWriter(failCtx)

	t.Cleanup(func() {
		goodCtx, err := iolib.NewContext()
		require.NoError(t, err)
		data.InitWriter(goodCtx)
	})
}

// TestRecordCastTypedTextReturnsStylePrefixWriteError covers the write-error
// branch guarding the style-prefix write in recordCastTypedText: when the
// rendered line differs from the raw line (producing a non-empty style
// prefix) and writing that prefix fails, the error must propagate.
func TestRecordCastTypedTextReturnsStylePrefixWriteError(t *testing.T) {
	withFailingDataWriter(t, 0)

	err := recordCastTypedText(context.Background(), &schema.SimulatePrompt{Style: "command"}, "atmos version", 0, 0)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "simulated write failure")
}

// TestRecordCastTypedTextReturnsCharWriteError covers the write-error branch
// inside recordCastTypedText's per-character typing loop: the style-prefix
// write succeeds, but the first character write fails and the error must
// propagate.
func TestRecordCastTypedTextReturnsCharWriteError(t *testing.T) {
	withFailingDataWriter(t, 1)

	err := recordCastTypedText(context.Background(), &schema.SimulatePrompt{Style: "command"}, "atmos version", 0, 0)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "simulated write failure")
}
