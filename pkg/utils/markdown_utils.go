// Package utils provides markdown utilities for error handling and output formatting
// in the Atmos CLI application.
package utils

import (
	"fmt"
	"os"
	"runtime/debug"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/markdown"

	l "github.com/charmbracelet/log"
	t "golang.org/x/term"
)

// render is the global markdown renderer instance initialized via InitializeMarkdown
var render *markdown.Renderer

func PrintErrorMarkdown(title string, err error, suggestion string) {
	if render == nil {
		LogError(err)
		return
	}
	if title == "" {
		title = "Error"
	}
	errorMarkdown, renderErr := render.RenderError(title, err.Error(), suggestion)
	if renderErr != nil {
		LogError(err)
		return
	}
	_, printErr := os.Stderr.WriteString(fmt.Sprint(errorMarkdown + "\n"))
	if printErr != nil {
		LogError(printErr)
		LogError(err)
	}
	// Print stack trace
	if l.GetLevel() == l.DebugLevel {
		debug.PrintStack()
	}
}

func PrintErrorMarkdownAndExit(title string, err error, suggestion string) {
	PrintErrorMarkdown(title, err, suggestion)
	os.Exit(1)
}

func PrintfMarkdown(format string, a ...interface{}) {
	if render == nil {
		_, err := os.Stdout.WriteString(fmt.Sprintf(format, a...))
		LogError(err)
		return
	}
	message := fmt.Sprintf(format, a...)
	var markdown string
	var renderErr error
	if !t.IsTerminal(int(os.Stdout.Fd())) {
		markdown, renderErr = render.RenderAscii(message)
	} else {
		markdown, renderErr = render.Render(message)
	}
	if renderErr != nil {
		LogErrorAndExit(renderErr)
	}
	_, err := os.Stdout.WriteString(fmt.Sprint(markdown + "\n"))
	LogError(err)
}

func InitializeMarkdown(atmosConfig schema.AtmosConfiguration) {
	var err error
	render, err = markdown.NewTerminalMarkdownRenderer(atmosConfig)
	if err != nil {
		LogErrorAndExit(fmt.Errorf("failed to initialize markdown renderer: %w", err))
	}
}
