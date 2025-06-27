package errors

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	log "github.com/charmbracelet/log"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/markdown"
)

var (
	// render is the global Markdown renderer instance initialized via InitializeMarkdown.
	render *markdown.Renderer
)

// InitializeMarkdown initializes a new Markdown renderer.
func InitializeMarkdown(atmosConfig schema.AtmosConfiguration) {
	var err error
	render, err = markdown.NewTerminalMarkdownRenderer(atmosConfig)
	if err != nil {
		CheckErrorPrintMarkdownAndExit(fmt.Errorf("failed to initialize markdown renderer: %w", err), "", "")
	}
}

// CheckErrorAndPrintMarkdown prints an error message in Markdown format.
func CheckErrorAndPrintMarkdown(err error, title string, suggestion string) {
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

// CheckErrorPrintMarkdownAndExit prints an error message in Markdown format and exits with the exit code 1.
func CheckErrorPrintMarkdownAndExit(err error, title string, suggestion string) {
	if err == nil {
		return
	}

	CheckErrorAndPrintMarkdown(err, title, suggestion)

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
