package starlark

import (
	"bytes"
	"io"
	"sync"

	"go.starlark.net/starlark"

	"github.com/cloudposse/atmos/pkg/perf"
)

const outputKey = "atmos.starlark.output"

type outputStream int

const (
	stdoutStream outputStream = iota
	stderrStream
)

// sessionWriter serializes writes to one destination across every thread of a session.
type sessionWriter struct {
	s      *session
	writer io.Writer
}

func (w sessionWriter) Write(p []byte) (int, error) {
	defer perf.Track(nil, "starlark.sessionWriter.Write")()

	w.s.mu.Lock()
	defer w.s.mu.Unlock()
	return w.writer.Write(p)
}

// lineWriter buffers partial lines so concurrent tasks never tear each other's output.
// Each complete line is emitted with the prefix, in one write to the session writer.
type lineWriter struct {
	mu     sync.Mutex
	prefix string
	dst    io.Writer
	buf    []byte
}

func (w *lineWriter) Write(p []byte) (int, error) {
	defer perf.Track(nil, "starlark.lineWriter.Write")()

	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	var out []byte
	for {
		end := bytes.IndexByte(w.buf, '\n')
		if end < 0 {
			break
		}
		out = append(out, w.prefix...)
		out = append(out, w.buf[:end+1]...)
		w.buf = w.buf[end+1:]
	}
	if len(out) == 0 {
		return len(p), nil
	}
	if _, err := w.dst.Write(out); err != nil {
		return 0, err
	}
	return len(p), nil
}

// flush emits a trailing partial line, terminated so it cannot merge with later output.
func (w *lineWriter) flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.buf) == 0 {
		return
	}
	out := append([]byte(w.prefix), w.buf...)
	out = append(out, '\n')
	w.buf = nil
	_, _ = w.dst.Write(out)
}

// taskOutput is the per-task-thread output sink stored as a thread local.
type taskOutput struct {
	prefix         string
	stdout, stderr *lineWriter
}

func (s *session) newTaskOutput(prefix string) *taskOutput {
	return &taskOutput{
		prefix: prefix,
		stdout: &lineWriter{prefix: prefix, dst: sessionWriter{s: s, writer: s.spec.Stdout}},
		stderr: &lineWriter{prefix: prefix, dst: sessionWriter{s: s, writer: s.spec.Stderr}},
	}
}

func (o *taskOutput) flush() {
	o.stdout.flush()
	o.stderr.flush()
}

// threadOutput returns the task output attached to a thread, or nil for the main thread.
func threadOutput(t *starlark.Thread) *taskOutput {
	if t == nil {
		return nil
	}
	out, _ := t.Local(outputKey).(*taskOutput)
	return out
}

// writer returns the destination for a thread: the task's prefixed line writer inside
// steps.parallel tasks, otherwise the mutex-guarded session stream.
func (s *session) writer(t *starlark.Thread, stream outputStream) io.Writer {
	if out := threadOutput(t); out != nil {
		if stream == stderrStream {
			return out.stderr
		}
		return out.stdout
	}
	if stream == stderrStream {
		return sessionWriter{s: s, writer: s.spec.Stderr}
	}
	return sessionWriter{s: s, writer: s.spec.Stdout}
}

// threadPrefix returns the line prefix of the task running on a thread, if any.
func threadPrefix(t *starlark.Thread) string {
	if out := threadOutput(t); out != nil {
		return out.prefix
	}
	return ""
}
