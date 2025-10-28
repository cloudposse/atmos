// Package utils provides markdown utilities for error handling and output formatting
// in the Atmos CLI application.
package utils

import (
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/markdown"
)

const (
	// Newline is the newline character constant.
	Newline = "\n"
)

// render is the global Markdown renderer instance initialized via InitializeMarkdown.
var render *markdown.Renderer

// isWriterTerminal checks if the writer is a terminal.
// This function checks arbitrary io.Writer, not just stdout/stderr, so it uses term.IsTerminal directly.
func isWriterTerminal(w io.Writer) bool {
	if file, ok := w.(*os.File); ok {
		return term.IsTerminal(int(file.Fd()))
	}
	return false
}

// trimRenderedMarkdown trims trailing spaces from each line of rendered markdown
// when not outputting to a terminal. This prevents unnecessary whitespace in
// non-terminal outputs (logs, files, pipes).
func trimRenderedMarkdown(md string, isTerminal bool) string {
	if isTerminal {
		return md
	}

	lines := strings.Split(md, Newline)
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.Join(lines, Newline)
}

// printfMarkdownTo prints a message in Markdown format to the specified writer.
func printfMarkdownTo(w io.Writer, format string, a ...interface{}) {
	if render == nil {
		_, err := fmt.Fprintf(w, format, a...)
		errUtils.CheckErrorAndPrint(err, "", "")
		return
	}
	message := fmt.Sprintf(format, a...)
	var md string
	var renderErr error
	md, renderErr = render.Render(message)
	if renderErr != nil {
		errUtils.CheckErrorPrintAndExit(renderErr, "", "")
	}

	isTerminal := isWriterTerminal(w)
	md = trimRenderedMarkdown(md, isTerminal)

	// Ensure rendered markdown ends with a newline to prevent content from
	// running into subsequent output (e.g., log lines).
	if !strings.HasSuffix(md, Newline) {
		md += Newline
	}

	_, err := fmt.Fprint(w, md)
	errUtils.CheckErrorAndPrint(err, "", "")
}

// PrintfMarkdown prints a message in Markdown format.
func PrintfMarkdown(format string, a ...interface{}) {
	defer perf.Track(nil, "utils.PrintfMarkdown")()

	printfMarkdownTo(os.Stdout, format, a...)
}

// PrintfMarkdownToTUI prints a message in Markdown format to stderr.
// This is useful for notices, warnings, and other messages that should not
// interfere with stdout when piping command output.
func PrintfMarkdownToTUI(format string, a ...interface{}) {
	defer perf.Track(nil, "utils.PrintfMarkdownToTUI")()

	printfMarkdownTo(os.Stderr, format, a...)
}

// InitializeMarkdown initializes a new Markdown renderer.
func InitializeMarkdown(atmosConfig schema.AtmosConfiguration) {
	defer perf.Track(&atmosConfig, "utils.InitializeMarkdown")()

	var err error
	render, err = markdown.NewTerminalMarkdownRenderer(atmosConfig)
	if err != nil {
		errUtils.CheckErrorPrintAndExit(fmt.Errorf("failed to initialize markdown renderer: %w", err), "", "")
	}
}
