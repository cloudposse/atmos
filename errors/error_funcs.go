package errors

import (
	"errors"
	"os"
	"os/exec"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/markdown"
)

// OsExit is a variable for testing, so we can mock os.Exit.
var OsExit = os.Exit

// render is the global Markdown renderer instance initialized via InitializeMarkdown.
var render *markdown.Renderer

// InitializeMarkdown initializes a new Markdown renderer.
func InitializeMarkdown(atmosConfig schema.AtmosConfiguration) {
	var err error
	render, err = markdown.NewTerminalMarkdownRenderer(atmosConfig)
	if err != nil {
		log.Error("failed to initialize Markdown renderer", "error", err)
	}
}

// CheckErrorAndPrint prints an error message.
func CheckErrorAndPrint(err error, title string, suggestion string) {
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
		log.Error(renderErr)
		log.Error(err)
		return
	}
	_, printErr := os.Stderr.WriteString(errorMarkdown + "\n")
	if printErr != nil {
		log.Error(printErr)
		log.Error(err)
	}
}

// CheckErrorPrintAndExit prints an error message and exits with exit code 1.
func CheckErrorPrintAndExit(err error, title string, suggestion string) {
	if err == nil {
		return
	}

	CheckErrorAndPrint(err, title, suggestion)

	// Check for ExitCodeError (from ShellRunner preserving interp.ExitStatus)
	var exitCodeErr ExitCodeError
	if errors.As(err, &exitCodeErr) {
		Exit(exitCodeErr.Code)
	}

	// Find the executed command's exit code from the error
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		Exit(exitError.ExitCode())
	}

	// TODO: Refactor so that we only call `os.Exit` in `main()` or `init()` functions.
	// Exiting here makes it difficult to test.
	// revive:disable-next-line:deep-exit
	Exit(1)
}

// Exit exits the program with the specified exit code.
func Exit(exitCode int) {
	OsExit(exitCode)
}
