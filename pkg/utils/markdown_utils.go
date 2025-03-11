// Package utils provides markdown utilities for error handling and output formatting
// in the Atmos CLI application.
package utils

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime/debug"

	log "github.com/charmbracelet/log"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/markdown"
)

// render is the global markdown renderer instance initialized via InitializeMarkdown.
var render *markdown.Renderer

// PrintErrorMarkdown prints an error message in Markdown format.
func PrintErrorMarkdown(title string, err error, suggestion string) {
	if err == nil {
		return
	}
	if render == nil {
		LogError(err)
		return
	}
	if title == "" {
		title = "Error"
	}
	title = cases.Title(language.English).String(title)
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
	if log.GetLevel() == log.DebugLevel {
		debug.PrintStack()
	}
}

// PrintfErrorMarkdown prints a formatted error message in Markdown format.
func PrintfErrorMarkdown(format string, a ...interface{}) {
	if render == nil {
		LogError(fmt.Errorf(format, a...))
		return
	}
	var markdown string
	var renderErr error
	markdown, renderErr = render.RenderErrorf(format, a...)
	if renderErr != nil {
		LogError(renderErr)
		LogError(fmt.Errorf(format, a...))
		return
	}
	_, err := os.Stderr.WriteString(fmt.Sprint(markdown + "\n"))
	LogError(err)
}

// PrintErrorMarkdownAndExit prints an error message in Markdown format and exist with the exit code 1.
func PrintErrorMarkdownAndExit(title string, err error, suggestion string) {
	PrintErrorMarkdown(title, err, suggestion)

	// Find the executed command's exit code from the error
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		os.Exit(exitError.ExitCode())
	}

	// TODO: Refactor so that we only call `os.Exit` in `main()` or `init()` functions.
	// Exiting here makes it difficult to test.
	// revive:disable-next-line:deep-exit
	os.Exit(1)
}

// PrintInvalidUsageErrorAndExit prints a message about the incorrect command usage and exist with the exit code 1.
func PrintInvalidUsageErrorAndExit(err error, suggestion string) {
	PrintErrorMarkdownAndExit("Incorrect Usage", err, suggestion)
}

// PrintfMarkdown prints a message in Markdown format.
func PrintfMarkdown(format string, a ...interface{}) {
	if render == nil {
		_, err := os.Stdout.WriteString(fmt.Sprintf(format, a...))
		LogError(err)
		return
	}
	message := fmt.Sprintf(format, a...)
	var markdown string
	var renderErr error
	markdown, renderErr = render.Render(message)
	if renderErr != nil {
		LogErrorAndExit(renderErr)
	}
	_, err := os.Stdout.WriteString(fmt.Sprint(markdown + "\n"))
	LogError(err)
}

// InitializeMarkdown initializes a new Markdown renderer.
func InitializeMarkdown(atmosConfig schema.AtmosConfiguration) {
	var err error
	render, err = markdown.NewTerminalMarkdownRenderer(atmosConfig)
	if err != nil {
		LogErrorAndExit(fmt.Errorf("failed to initialize markdown renderer: %w", err))
	}
}
