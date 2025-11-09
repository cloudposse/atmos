package data

import (
	"encoding/json"
	"fmt"
	"sync"

	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/perf"
)

// MarkdownRenderer is the interface for rendering markdown.
// This avoids circular dependency with pkg/ui.
type MarkdownRenderer interface {
	Markdown(content string) (string, error)
}

var (
	globalIOContext      io.Context
	globalMarkdownRender MarkdownRenderer
	ioMu                 sync.RWMutex
)

// InitWriter initializes the global data writer with an I/O context.
// This should be called once at application startup (in root.go).
func InitWriter(ioCtx io.Context) {
	defer perf.Track(nil, "data.InitWriter")()

	ioMu.Lock()
	defer ioMu.Unlock()
	globalIOContext = ioCtx
}

// SetMarkdownRenderer sets the markdown renderer for data.Markdown().
// This should be called after ui.InitFormatter() in root.go.
func SetMarkdownRenderer(renderer MarkdownRenderer) {
	defer perf.Track(nil, "data.SetMarkdownRenderer")()

	ioMu.Lock()
	defer ioMu.Unlock()
	globalMarkdownRender = renderer
}

// getIOContext returns the global I/O context instance.
// Panics if not initialized (programming error, not runtime error).
func getIOContext() io.Context {
	ioMu.RLock()
	defer ioMu.RUnlock()

	if globalIOContext == nil {
		panic("data.InitWriter() must be called before using data package functions")
	}

	return globalIOContext
}

// Write writes content to the data channel (stdout).
// Flow: data.Write() → io.Write(DataStream) → masking → stdout.
func Write(content string) error {
	defer perf.Track(nil, "data.Write")()

	return getIOContext().Write(io.DataStream, content)
}

// Writef writes formatted content to the data channel (stdout).
// Flow: data.Writef() → io.Write(DataStream) → masking → stdout.
func Writef(format string, a ...interface{}) error {
	defer perf.Track(nil, "data.Writef")()

	return getIOContext().Write(io.DataStream, fmt.Sprintf(format, a...))
}

// Writeln writes content followed by a newline to the data channel (stdout).
// Flow: data.Writeln() → io.Write(DataStream) → masking → stdout.
func Writeln(content string) error {
	defer perf.Track(nil, "data.Writeln")()

	return getIOContext().Write(io.DataStream, content+"\n")
}

// WriteJSON marshals v to JSON and writes to the data channel (stdout).
// Flow: data.WriteJSON() → io.Write(DataStream) → masking → stdout.
func WriteJSON(v interface{}) error {
	defer perf.Track(nil, "data.WriteJSON")()

	output, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return Write(string(output) + "\n")
}

// WriteYAML marshals v to YAML and writes to the data channel (stdout).
// Flow: data.WriteYAML() → io.Write(DataStream) → masking → stdout.
func WriteYAML(v interface{}) error {
	defer perf.Track(nil, "data.WriteYAML")()

	output, err := yaml.Marshal(v)
	if err != nil {
		return err
	}
	return Write(string(output))
}

// Markdown renders markdown content and writes to the data channel (stdout).
// Use this for help text, documentation, and other pipeable formatted content.
// Flow: data.Markdown() → MarkdownRenderer.Markdown() → io.Write(DataStream) → masking → stdout.
func Markdown(content string) error {
	defer perf.Track(nil, "data.Markdown")()

	ioMu.RLock()
	renderer := globalMarkdownRender
	ioCtx := globalIOContext
	ioMu.RUnlock()

	if ioCtx == nil {
		panic("data.InitWriter() must be called before using data.Markdown()")
	}

	if renderer == nil {
		return errUtils.ErrUIFormatterNotInitialized
	}

	rendered, err := renderer.Markdown(content)
	if err != nil {
		// Degrade gracefully - write plain content if rendering fails
		rendered = content
	}

	return ioCtx.Write(io.DataStream, rendered)
}

// Markdownf renders formatted markdown content and writes to the data channel (stdout).
// Flow: data.Markdownf() → data.Markdown() → io.Write(DataStream) → masking → stdout.
func Markdownf(format string, a ...interface{}) error {
	defer perf.Track(nil, "data.Markdownf")()

	content := fmt.Sprintf(format, a...)
	return Markdown(content)
}
