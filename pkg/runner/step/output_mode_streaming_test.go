package step

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
)

// Compile-time guard: lineForwarder must remain an io.Writer.
var _ io.Writer = (*lineForwarder)(nil)

// syncBuffer is a goroutine-safe bytes.Buffer used as an output sink.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// streamSinks implements iolib.Streams over in-memory sinks.
type streamSinks struct {
	stdout, stderr *syncBuffer
}

func (s *streamSinks) Input() io.Reader     { return nil }
func (s *streamSinks) Output() io.Writer    { return s.stdout }
func (s *streamSinks) Error() io.Writer     { return s.stderr }
func (s *streamSinks) RawOutput() io.Writer { return s.stdout }
func (s *streamSinks) RawError() io.Writer  { return s.stderr }

// initStreamSinks wires the data and UI channels to in-memory sinks and restores working defaults afterward.
func initStreamSinks(t *testing.T) (*syncBuffer, *syncBuffer) {
	t.Helper()
	t.Setenv("NO_COLOR", "1")

	sinks := &streamSinks{stdout: &syncBuffer{}, stderr: &syncBuffer{}}
	ioCtx, err := iolib.NewContext(iolib.WithStreams(sinks))
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)

	t.Cleanup(func() {
		goodCtx, err := iolib.NewContext()
		require.NoError(t, err)
		data.InitWriter(goodCtx)
		ui.InitFormatter(goodCtx)
	})
	return sinks.stdout, sinks.stderr
}

func TestLogModeStreamsLinesBeforeRunnerReturns(t *testing.T) {
	stdoutSink, stderrSink := initStreamSinks(t)
	writer := NewOutputModeWriter(OutputModeLog, "stream_step", nil)

	line1Written := make(chan struct{})
	release := make(chan struct{})
	var visibleBeforeReturn string

	done := make(chan struct{})
	var gotStdout, gotStderr string
	var gotErr error
	go func() {
		defer close(done)
		gotStdout, gotStderr, gotErr = writer.ExecuteWithIO(func(stdout, _ io.Writer) error {
			_, _ = stdout.Write([]byte("line1\n"))
			close(line1Written)
			<-release
			_, _ = stdout.Write([]byte("line2"))
			return nil
		})
	}()

	<-line1Written
	require.Eventually(t, func() bool {
		visibleBeforeReturn = stdoutSink.String()
		return strings.Contains(visibleBeforeReturn, "line1\n")
	}, 5*time.Second, time.Millisecond, "line1 must be forwarded while the runner is still blocked")
	assert.NotContains(t, visibleBeforeReturn, "line2")
	assert.NotContains(t, stderrSink.String(), "stream_step completed", "footer must not appear before the runner returns")

	close(release)
	<-done

	require.NoError(t, gotErr)
	assert.Equal(t, "line1\nline2", gotStdout)
	assert.Empty(t, gotStderr)
	assert.Equal(t, "line1\nline2", stdoutSink.String(), "trailing partial line must be flushed on return")
	assert.Contains(t, stderrSink.String(), "[stream_step]")
	assert.Contains(t, stderrSink.String(), "stream_step completed")
}

func TestLogModeStreamsStderrAndPropagatesError(t *testing.T) {
	stdoutSink, stderrSink := initStreamSinks(t)
	writer := NewOutputModeWriter(OutputModeLog, "err_step", nil)
	runErr := errors.New("boom")

	stdout, stderr, err := writer.ExecuteWithIO(func(stdout, stderr io.Writer) error {
		_, _ = stdout.Write([]byte("out\n"))
		_, _ = stderr.Write([]byte("err-line\npartial"))
		return runErr
	})

	require.ErrorIs(t, err, runErr)
	assert.Equal(t, "out\n", stdout)
	assert.Equal(t, "err-line\npartial", stderr)
	assert.Equal(t, "out\n", stdoutSink.String())
	assert.Contains(t, stderrSink.String(), "err-line\npartial")
	assert.Contains(t, stderrSink.String(), "err_step failed")
}

func TestLineForwarder(t *testing.T) {
	tests := []struct {
		name          string
		writes        []string
		wantBeforeEnd []string
		wantAfterEnd  []string
	}{
		{name: "complete line forwarded immediately", writes: []string{"a\n"}, wantBeforeEnd: []string{"a\n"}},
		{name: "partial held until flush", writes: []string{"abc"}, wantBeforeEnd: nil, wantAfterEnd: []string{"abc"}},
		{name: "split line joined", writes: []string{"ab", "c\nd"}, wantBeforeEnd: []string{"abc\n"}, wantAfterEnd: []string{"d"}},
		{name: "multiple lines batched per write", writes: []string{"a\nb\nc"}, wantBeforeEnd: []string{"a\nb\n"}, wantAfterEnd: []string{"c"}},
		{name: "empty write is a no-op", writes: []string{""}, wantBeforeEnd: nil},
		{name: "blank lines preserved", writes: []string{"\n\n"}, wantBeforeEnd: []string{"\n\n"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var mu sync.Mutex
			var got []string
			f := newLineForwarder(&mu, func(s string) { got = append(got, s) })

			var want strings.Builder
			for _, w := range tt.writes {
				n, err := f.Write([]byte(w))
				require.NoError(t, err)
				assert.Equal(t, len(w), n)
				want.WriteString(w)
			}
			assert.Equal(t, tt.wantBeforeEnd, got)

			f.Flush()
			assert.Equal(t, append(append([]string(nil), tt.wantBeforeEnd...), tt.wantAfterEnd...), got)
			assert.Equal(t, want.String(), f.String(), "captured output must be byte-identical to input")

			// A second flush must not re-emit anything.
			f.Flush()
			assert.Len(t, got, len(tt.wantBeforeEnd)+len(tt.wantAfterEnd))
		})
	}
}

func TestLogModeConcurrentStreamsDoNotTearLines(t *testing.T) {
	stdoutSink, stderrSink := initStreamSinks(t)
	writer := NewOutputModeWriter(OutputModeLog, "concurrent_step", nil)

	const lines = 500
	var wantOut, wantErr strings.Builder
	for i := range lines {
		fmt.Fprintf(&wantOut, "out-%04d-%s\n", i, strings.Repeat("x", 40))
		fmt.Fprintf(&wantErr, "err-%04d-%s\n", i, strings.Repeat("y", 40))
	}

	stdout, stderr, err := writer.ExecuteWithIO(func(stdout, stderr io.Writer) error {
		var wg sync.WaitGroup
		write := func(w io.Writer, prefix, fill string) {
			defer wg.Done()
			for i := range lines {
				_, _ = fmt.Fprintf(w, "%s-%04d-%s\n", prefix, i, strings.Repeat(fill, 40))
			}
		}
		wg.Add(2)
		go write(stdout, "out", "x")
		go write(stderr, "err", "y")
		wg.Wait()
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, wantOut.String(), stdout)
	assert.Equal(t, wantErr.String(), stderr)
	assert.Equal(t, wantOut.String(), stdoutSink.String(), "stdout sink must contain every line, in order, untorn")

	// The UI sink also carries the header/footer; every other line must be an intact stderr line.
	var gotErrLines []string
	for _, line := range strings.Split(stderrSink.String(), "\n") {
		if strings.HasPrefix(line, "err-") {
			gotErrLines = append(gotErrLines, line+"\n")
		}
	}
	assert.Equal(t, wantErr.String(), strings.Join(gotErrLines, ""))
}
