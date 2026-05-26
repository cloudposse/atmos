package io

import (
	stdio "io"
	"os"
	"strings"
	"sync"
)

// NodeOutput contains composed stdout/stderr writers for one execution node.
type NodeOutput struct {
	Stdout stdio.Writer
	Stderr stdio.Writer
}

// NodeOutputSinks are the destinations for one node output stream.
type NodeOutputSinks struct {
	Terminal stdio.Writer
	File     stdio.Writer
	Capture  stdio.Writer
}

// NodeOutputOptions configures per-node output composition.
type NodeOutputOptions struct {
	NodeID string
	Stdout NodeOutputSinks
	Stderr NodeOutputSinks
}

// NewNodeOutput creates masked per-node stdout/stderr writers that can fan out
// to terminal, file, and capture sinks.
func NewNodeOutput(opts NodeOutputOptions) NodeOutput {
	stdout := opts.Stdout
	stderr := opts.Stderr
	if stdout.Terminal == nil && stdout.File == nil && stdout.Capture == nil {
		stdout.Terminal = os.Stdout
	}
	if stderr.Terminal == nil && stderr.File == nil && stderr.Capture == nil {
		stderr.Terminal = os.Stderr
	}

	return NodeOutput{
		Stdout: composeNodeOutput(opts.NodeID, stdout),
		Stderr: composeNodeOutput(opts.NodeID, stderr),
	}
}

func composeNodeOutput(nodeID string, sinks NodeOutputSinks) stdio.Writer {
	writers := make([]stdio.Writer, 0, 3)
	addSink := func(w stdio.Writer) {
		if w == nil {
			return
		}
		writers = append(writers, MaskWriter(NewPrefixedWriter(nodeID, w)))
	}
	addSink(sinks.Terminal)
	addSink(sinks.File)
	addSink(sinks.Capture)

	if len(writers) == 0 {
		return stdio.Discard
	}
	if len(writers) == 1 {
		return writers[0]
	}
	return stdio.MultiWriter(writers...)
}

// NewPrefixedWriter returns a writer that prefixes each line with [prefix].
func NewPrefixedWriter(prefix string, w stdio.Writer) stdio.Writer {
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
