package errors

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	log "github.com/charmbracelet/log"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/cloudposse/atmos/pkg/ui/markdown"
)

var (
	// render is the global Markdown renderer instance initialized via InitializeMarkdown.
	render *markdown.Renderer
)

// Variable declarations for functions that might be mocked in tests.
var (
	// PrintErrorMarkdownAndExitFn is a variable that holds the function reference for testing.
	PrintErrorMarkdownAndExitFn = printErrorMarkdownAndExitImpl
)

// PrintErrorMarkdown prints an error message in Markdown format.
func PrintErrorMarkdown(err error, title string, suggestion string) {
	if err == nil {
		return
	}
	if render == nil {
		log.Error(err)
		return
	}
	if title == "" {
		title = "Error"
	}
	title = cases.Title(language.English).String(title)
	errorMarkdown, renderErr := render.RenderError(title, err.Error(), suggestion)
	if renderErr != nil {
		log.Error(err)
		return
	}
	_, printErr := os.Stderr.WriteString(fmt.Sprint(errorMarkdown + "\n"))
	if printErr != nil {
		log.Error(printErr)
		log.Error(err)
	}
}

// PrintfErrorMarkdown prints a formatted error message in Markdown format.
func PrintfErrorMarkdown(format string, a ...interface{}) {
	if render == nil {
		log.Error(fmt.Errorf(format, a...))
		return
	}
	md, renderErr := render.RenderErrorf(format, a...)
	if renderErr != nil {
		log.Error(renderErr)
		log.Error(fmt.Errorf(format, a...))
		return
	}
	_, err := os.Stderr.WriteString(fmt.Sprint(md + "\n"))
	log.Error(err)
}

// printErrorMarkdownAndExitImpl is the implementation of PrintErrorMarkdownAndExit.
func printErrorMarkdownAndExitImpl(err error, title string, suggestion string) {
	if err == nil {
		return
	}

	PrintErrorMarkdown(err, title, suggestion)

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

// PrintErrorMarkdownAndExit prints an error message in Markdown format and exits with the exit code 1.
func PrintErrorMarkdownAndExit(err error, title string, suggestion string) {
	if err == nil {
		return
	}
	PrintErrorMarkdownAndExitFn(err, title, suggestion)
}
