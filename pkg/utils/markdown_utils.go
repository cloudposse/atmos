// Package utils provides markdown utilities for error handling and output formatting
// in the Atmos CLI application.
package utils

import (
	"fmt"
	"os"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/markdown"
)

// render is the global Markdown renderer instance initialized via InitializeMarkdown.
var render *markdown.Renderer

// PrintfMarkdown prints a message in Markdown format.
func PrintfMarkdown(format string, a ...interface{}) {
	defer perf.Track(nil, "utils.PrintfMarkdown")()

	if render == nil {
		_, err := os.Stdout.WriteString(fmt.Sprintf(format, a...))
		errUtils.HandleError(err)
		return
	}
	message := fmt.Sprintf(format, a...)
	var md string
	var renderErr error
	md, renderErr = render.Render(message)
	if renderErr != nil {
		// Fall back to plain text output if markdown rendering fails (non-fatal UI error).
		_, _ = os.Stdout.WriteString(message + "\n")
		return
	}
	_, err := os.Stdout.WriteString(fmt.Sprint(md + "\n"))
	errUtils.HandleError(err)
}

// InitializeMarkdown initializes a new Markdown renderer.
func InitializeMarkdown(atmosConfig schema.AtmosConfiguration) {
	defer perf.Track(&atmosConfig, "utils.InitializeMarkdown")()

	var err error
	render, err = markdown.NewTerminalMarkdownRenderer(atmosConfig)
	if err != nil {
		// Initialization failure is non-fatal - PrintfMarkdown will fall back to plain text.
		// This can happen in unusual terminal environments.
		render = nil
	}
}
