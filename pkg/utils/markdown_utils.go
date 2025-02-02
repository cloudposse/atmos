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

var render *markdown.Renderer

func PrintErrorMarkdown(title string, err error, suggestion string) {
	if title == "" {
		title = "Error"
	}
	errorMarkdown, renderErr := render.RenderError(title, err.Error(), suggestion)
	if renderErr != nil {
		LogError(err)
		return
	}
	os.Stderr.WriteString(fmt.Sprint(errorMarkdown + "\n"))
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
	os.Stdout.WriteString(fmt.Sprint(markdown + "\n"))
}

func InitializeMarkdown(atmosConfig schema.AtmosConfiguration) {
	render, _ = markdown.NewTerminalMarkdownRenderer(atmosConfig)
}
