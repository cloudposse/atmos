package io

import (
	"errors"
	stdio "io"
	"os"
	"strings"
	"sync"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Output contains composed stdout/stderr writers for one execution scope.
//
// Each sink is masked with a StreamingMaskWriter so a secret split across two writes is still
// masked. Call Flush once the producer has finished to release any held tail.
type Output struct {
	Stdout stdio.Writer
	Stderr stdio.Writer

	maskers []*StreamingMaskWriter
}

// Flush releases the tail held back by the masked sinks. Call it when the producer is done.
func (o Output) Flush() error {
	defer perf.Track(nil, "io.Output.Flush")()

	var errs []error
	for _, m := range o.maskers {
		errs = append(errs, m.Flush())
	}
	return errors.Join(errs...)
}

// OutputSinks are the destinations for one output stream.
type OutputSinks struct {
	Terminal stdio.Writer
	File     stdio.Writer
	Capture  stdio.Writer
}

// OutputOptions configures masked, prefixed output composition.
type OutputOptions struct {
	Prefix string
	Stdout OutputSinks
	Stderr OutputSinks
}

// NewOutput creates masked stdout/stderr writers that can fan out
// to terminal, file, and capture sinks.
func NewOutput(opts OutputOptions) Output {
	stdout := opts.Stdout
	stderr := opts.Stderr
	if stdout.Terminal == nil && stdout.File == nil && stdout.Capture == nil {
		stdout.Terminal = os.Stdout
	}
	if stderr.Terminal == nil && stderr.File == nil && stderr.Capture == nil {
		stderr.Terminal = os.Stderr
	}

	stdoutWriter, stdoutMaskers := composeOutput(opts.Prefix, stdout)
	stderrWriter, stderrMaskers := composeOutput(opts.Prefix, stderr)
	return Output{
		Stdout:  stdoutWriter,
		Stderr:  stderrWriter,
		maskers: append(stdoutMaskers, stderrMaskers...),
	}
}

func composeOutput(prefix string, sinks OutputSinks) (stdio.Writer, []*StreamingMaskWriter) {
	writers := make([]stdio.Writer, 0, 3)
	maskers := make([]*StreamingMaskWriter, 0, 3)
	addSink := func(w stdio.Writer) {
		if w == nil {
			return
		}
		sw := NewStreamingMaskWriter(NewPrefixedWriter(prefix, w))
		writers = append(writers, sw)
		maskers = append(maskers, sw)
	}
	addSink(sinks.Terminal)
	addSink(sinks.File)
	addSink(sinks.Capture)

	if len(writers) == 0 {
		return stdio.Discard, nil
	}
	if len(writers) == 1 {
		return writers[0], maskers
	}
	return stdio.MultiWriter(writers...), maskers
}

// NewPrefixedWriter returns a writer that prefixes each line with [prefix].
func NewPrefixedWriter(prefix string, w stdio.Writer) stdio.Writer {
	if w == nil {
		return stdio.Discard
	}
	if prefix == "" {
		return w
	}
	return &prefixedWriter{
		prefix: "[" + prefix + "] ",
		w:      w,
	}
}

type prefixedWriter struct {
	mu              sync.Mutex
	prefix          string
	w               stdio.Writer
	wroteLastByte   bool
	lastByteNewline bool
}

func (w *prefixedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if len(p) == 0 {
		return 0, nil
	}

	var b strings.Builder
	b.Grow(len(p) + len(w.prefix))
	atLineStart := !w.wroteLastByte || w.lastByteNewline
	for _, c := range p {
		if atLineStart {
			b.WriteString(w.prefix)
			atLineStart = false
		}
		b.WriteByte(c)
		if c == '\n' {
			atLineStart = true
		}
	}

	out := b.String()
	n, err := stdio.WriteString(w.w, out)
	if err != nil {
		return 0, err
	}
	if n < len(out) {
		return 0, stdio.ErrShortWrite
	}

	w.wroteLastByte = true
	w.lastByteNewline = p[len(p)-1] == '\n'
	return len(p), nil
}
