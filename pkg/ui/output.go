package ui

import (
	"fmt"
	"strings"

	"github.com/cloudposse/atmos/pkg/io"
)

// output implements the Output interface.
type output struct {
	ioCtx                  io.Context
	formatter              Formatter
	trimTrailingWhitespace bool
}

// NewOutput creates a new Output.
func NewOutput(ioCtx io.Context, opts ...OutputOption) Output {
	o := &output{
		ioCtx:                  ioCtx,
		formatter:              newFormatter(ioCtx),
		trimTrailingWhitespace: false, // Default to false
	}

	// Apply options
	for _, opt := range opts {
		opt(o)
	}

	return o
}

// OutputOption configures Output behavior.
type OutputOption func(*output)

// WithTrimTrailingWhitespace enables/disables trailing whitespace trimming.
func WithTrimTrailingWhitespace(enabled bool) OutputOption {
	return func(o *output) {
		o.trimTrailingWhitespace = enabled
	}
}

// Data output (to stdout - pipeable).
func (o *output) Print(a ...interface{}) {
	s := fmt.Sprint(a...)
	s = o.processOutput(s)
	fmt.Fprint(o.ioCtx.Streams().Output(), s)
}

func (o *output) Printf(format string, a ...interface{}) {
	s := fmt.Sprintf(format, a...)
	s = o.processOutput(s)
	fmt.Fprint(o.ioCtx.Streams().Output(), s)
}

func (o *output) Println(a ...interface{}) {
	s := fmt.Sprintln(a...)
	s = o.processOutput(s)
	fmt.Fprint(o.ioCtx.Streams().Output(), s)
}

// UI output (to stderr - human-readable).
func (o *output) Message(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	msg = o.processOutput(msg)
	fmt.Fprintln(o.ioCtx.Streams().Error(), msg)
}

func (o *output) Success(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	msg = o.formatter.Success(msg)
	msg = o.processOutput(msg)
	fmt.Fprintln(o.ioCtx.Streams().Error(), msg)
}

func (o *output) Warning(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	msg = o.formatter.Warning(msg)
	msg = o.processOutput(msg)
	fmt.Fprintln(o.ioCtx.Streams().Error(), msg)
}

func (o *output) Error(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	msg = o.formatter.Error(msg)
	msg = o.processOutput(msg)
	fmt.Fprintln(o.ioCtx.Streams().Error(), msg)
}

func (o *output) Info(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	msg = o.formatter.Info(msg)
	msg = o.processOutput(msg)
	fmt.Fprintln(o.ioCtx.Streams().Error(), msg)
}

// Markdown output to stdout.
func (o *output) Markdown(content string) error {
	rendered, _ := o.formatter.RenderMarkdown(content)
	rendered = o.processOutput(rendered)
	fmt.Fprint(o.ioCtx.Streams().Output(), rendered)
	return nil
}

// Markdown output to stderr.
func (o *output) MarkdownUI(content string) error {
	rendered, _ := o.formatter.RenderMarkdown(content)
	rendered = o.processOutput(rendered)
	fmt.Fprint(o.ioCtx.Streams().Error(), rendered)
	return nil
}

// Output options.
func (o *output) SetTrimTrailingWhitespace(enabled bool) {
	o.trimTrailingWhitespace = enabled
}

func (o *output) TrimTrailingWhitespace() bool {
	return o.trimTrailingWhitespace
}

func (o *output) Formatter() Formatter {
	return o.formatter
}

func (o *output) IOContext() io.Context {
	return o.ioCtx
}

// processOutput applies output transformations (trimming, etc.).
func (o *output) processOutput(s string) string {
	if o.trimTrailingWhitespace {
		return trimTrailingSpaces(s)
	}
	return s
}

// trimTrailingSpaces removes trailing whitespace before newlines.
func trimTrailingSpaces(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.Join(lines, "\n")
}
