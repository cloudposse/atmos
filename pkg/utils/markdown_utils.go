// Package utils provides markdown utilities for error handling and output formatting
// in the Atmos CLI application.
package utils

import (
	"fmt"
	"io"
	"os"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/markdown"
)

// render is the global Markdown renderer instance initialized via InitializeMarkdown.
var render *markdown.Renderer

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
	_, err := fmt.Fprint(w, md+"\n")
	errUtils.CheckErrorAndPrint(err, "", "")
}

// PrintfMarkdown prints a message in Markdown format.
func PrintfMarkdown(format string, a ...interface{}) {
	printfMarkdownTo(os.Stdout, format, a...)
}

// PrintfMarkdownToTUI prints a message in Markdown format to stderr.
// This is useful for notices, warnings, and other messages that should not
// interfere with stdout when piping command output.
func PrintfMarkdownToTUI(format string, a ...interface{}) {
	printfMarkdownTo(os.Stderr, format, a...)
}

// InitializeMarkdown initializes a new Markdown renderer.
func InitializeMarkdown(atmosConfig schema.AtmosConfiguration) {
	var err error
	render, err = markdown.NewTerminalMarkdownRenderer(atmosConfig)
	if err != nil {
		errUtils.CheckErrorPrintAndExit(fmt.Errorf("failed to initialize markdown renderer: %w", err), "", "")
	}
}
