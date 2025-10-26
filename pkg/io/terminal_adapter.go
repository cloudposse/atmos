package io

// TerminalWriter adapts io.Context to satisfy terminal.IOWriter interface.
// This avoids circular dependency while allowing terminal to write through I/O layer.
type TerminalWriter struct {
	ctx Context
}

// NewTerminalWriter creates a terminal-compatible writer from io.Context.
func NewTerminalWriter(ctx Context) *TerminalWriter {
	return &TerminalWriter{ctx: ctx}
}

// Write implements terminal.IOWriter interface.
// It accepts int stream values (0=Data, 1=UI) and converts to io.Stream.
func (tw *TerminalWriter) Write(stream int, content string) error {
	// stream values: 0=Data, 1=UI (matches io.DataStream and io.UIStream)
	return tw.ctx.Write(Stream(stream), content)
}
