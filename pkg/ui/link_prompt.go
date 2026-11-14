package ui

import (
	"os"
	"strings"
	"sync"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/lipgloss"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/terminal"
	"github.com/cloudposse/atmos/pkg/ui/link"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

// LinkPrompt describes a URL the user may need to open by hand, typically one
// too long to draw inside a dialog box.
type LinkPrompt struct {
	// URL is the full address to open.
	URL string
	// Label is the short clickable text shown for an OSC 8 hyperlink, for
	// example "Open AWS sign-in".
	Label string
	// Hint is an optional lead-in clause that says when the link is needed, for
	// example "If the browser doesn't open". It carries no trailing punctuation.
	// Without a hint, the label introduces the URL.
	Hint string
}

// copyToClipboard writes to the native system clipboard. It is a variable so
// tests never touch the real clipboard.
var copyToClipboard = clipboard.WriteAll

// linkMode is the configured delivery mode, set once at startup by SetLinkMode.
var (
	linkMode   = link.ModeAuto
	linkModeMu sync.RWMutex
)

// SetLinkMode applies `settings.terminal.links` (or ATMOS_TERMINAL_LINKS, or --links) to
// every link prompt written afterwards. An empty value means `auto`. An unknown
// value is reported and treated as `auto`, so a typo never hides a sign-in URL.
func SetLinkMode(value string) {
	defer perf.Track(nil, "ui.SetLinkMode")()

	mode, ok := link.ParseMode(value)
	if !ok {
		log.Warn("Ignoring unknown terminal links mode; using auto", "value", value, "valid", "auto, osc8, clipboard, raw")
	}
	linkModeMu.Lock()
	defer linkModeMu.Unlock()
	linkMode = mode
}

// configuredLinkMode returns the mode set by SetLinkMode.
func configuredLinkMode() link.Mode {
	linkModeMu.RLock()
	defer linkModeMu.RUnlock()
	return linkMode
}

// WriteLinkPrompt writes a URL to stderr (UI channel) in the form selected by
// `settings.terminal.links`: an OSC 8 hyperlink with a short label, a clipboard
// copy, or the raw URL. With the default `auto`, a hyperlink is used only when
// the terminal is known to support one. The raw URL is always the fallback, so
// the URL is never lost. An empty URL writes nothing.
func WriteLinkPrompt(prompt LinkPrompt) {
	defer perf.Track(nil, "ui.WriteLinkPrompt")()

	if prompt.URL == "" {
		return
	}
	f, err := getFormatter()
	if err != nil {
		log.Debug("ui.WriteLinkPrompt called before InitFormatter")
		return
	}

	env := link.Environment{
		IsTTY:  f.terminal.IsTTY(terminal.Stderr),
		Lookup: os.LookupEnv,
	}
	text, outcome := renderLinkPrompt(prompt, link.Resolve(configuredLinkMode(), env), copyToClipboard)
	// Say what happened to the clipboard as a status message, because a copy
	// replaces whatever the user had there.
	switch outcome {
	case clipboardCopied:
		Success(clipboardCopiedMessage)
	case clipboardUnconfirmed:
		Warning(clipboardUnconfirmedMessage)
	case clipboardUntouched:
	}
	if text != "" {
		Write(text)
	}
}

// clipboardOutcome records what a link prompt did to the clipboard.
type clipboardOutcome int

const (
	// The clipboard was not involved.
	clipboardUntouched clipboardOutcome = iota
	// The operating system accepted the copy.
	clipboardCopied
	// The native copy failed and an OSC 52 request was sent instead, which the
	// terminal does not acknowledge.
	clipboardUnconfirmed
)

const (
	clipboardCopiedMessage      = "Copied the URL to your clipboard"
	clipboardUnconfirmedMessage = "Could not confirm the URL was copied to your clipboard, so it is printed below"
)

// renderLinkPrompt builds the text for a resolved delivery mode and reports
// what happened to the clipboard. A clipboard copy that the operating system
// rejects falls back to the raw URL, preceded by an OSC 52 request in case the
// terminal can still honor it (as it can from a container or an SSH session,
// where no native clipboard exists).
func renderLinkPrompt(prompt LinkPrompt, mode link.Mode, copyFn func(string) error) (string, clipboardOutcome) {
	scheme := theme.GetCurrentColorScheme()
	hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(scheme.TextMuted))
	urlStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(scheme.Border)).Italic(true)
	linkStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(scheme.Link)).Underline(true)

	outcome := clipboardUntouched
	var out strings.Builder
	switch mode {
	case link.ModeOSC8:
		if prompt.Hint != "" {
			out.WriteString(hintStyle.Render(prompt.Hint + ", click:"))
			out.WriteString(space)
		}
		out.WriteString(link.Hyperlink(prompt.URL, linkStyle.Render(prompt.Label)))
		out.WriteString(newline)
		return out.String(), outcome
	case link.ModeClipboard:
		err := copyFn(prompt.URL)
		if err == nil {
			if prompt.Hint != "" {
				out.WriteString(hintStyle.Render(prompt.Hint + ", paste it into your browser."))
				out.WriteString(newline)
			}
			return out.String(), clipboardCopied
		}
		log.Debug("Native clipboard copy failed; requesting an OSC 52 copy and printing the URL", "error", err)
		out.WriteString(link.ClipboardSequence(prompt.URL))
		outcome = clipboardUnconfirmed
	}

	out.WriteString(hintStyle.Render(rawLeadIn(prompt)))
	out.WriteString(newline)
	out.WriteString(urlStyle.Render(prompt.URL))
	out.WriteString(newline)
	return out.String(), outcome
}

// rawLeadIn is the line printed above a raw URL.
func rawLeadIn(prompt LinkPrompt) string {
	if prompt.Hint == "" {
		return prompt.Label + ":"
	}
	return prompt.Hint + ", visit:"
}
