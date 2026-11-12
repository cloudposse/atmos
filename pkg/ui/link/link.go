// Package link decides how a URL the user may need to open by hand is delivered
// to the terminal: as an OSC 8 hyperlink with a short label, through the
// clipboard, or as the raw URL. It holds the pure decision logic and the escape
// sequences; writing to the terminal lives in pkg/ui.
package link

import (
	"encoding/base64"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// MaxInlineWidth is the widest URL a bordered dialog may contain. Longer URLs
// are delivered outside the box so the border never wraps and never ends up in
// copied text, while short device-code URLs stay with their instructions.
const MaxInlineWidth = 80

// Mode selects how a long URL is delivered. It is configured with
// `settings.terminal.links` in atmos.yaml or ATMOS_TERMINAL_LINKS.
type Mode string

const (
	// ModeAuto uses an OSC 8 hyperlink when the terminal is known to support
	// one and the raw URL otherwise.
	ModeAuto Mode = "auto"
	// ModeOSC8 always emits an OSC 8 hyperlink with a short label.
	ModeOSC8 Mode = "osc8"
	// ModeClipboard copies the URL to the clipboard.
	ModeClipboard Mode = "clipboard"
	// ModeRaw always prints the full URL as plain text.
	ModeRaw Mode = "raw"
)

const (
	// Escape sequence framing (ECMA-48): OSC introducer and string terminator.
	escOSC = "\x1b]"
	escST  = "\x1b\\"
)

// ParseMode converts a configured value into a Mode. An empty value means
// ModeAuto. The boolean is false when the value is not a known mode, in which
// case ModeAuto is returned so a typo never hides a sign-in URL.
func ParseMode(value string) (Mode, bool) {
	switch mode := Mode(strings.ToLower(strings.TrimSpace(value))); mode {
	case "":
		return ModeAuto, true
	case ModeAuto, ModeOSC8, ModeClipboard, ModeRaw:
		return mode, true
	default:
		return ModeAuto, false
	}
}

// Resolve turns a configured mode into the mode to use for this terminal. It
// never returns ModeAuto. Output that is not a terminal always gets the raw
// URL, because escape sequences and clipboard writes are wrong for pipes, logs
// and CI.
func Resolve(mode Mode, env Environment) Mode {
	if !env.IsTTY {
		return ModeRaw
	}
	switch mode {
	case ModeOSC8, ModeClipboard, ModeRaw:
		return mode
	default:
		if SupportsHyperlinks(env) {
			return ModeOSC8
		}
		return ModeRaw
	}
}

// FitsInline reports whether a URL is short enough to sit inside a dialog box.
func FitsInline(url string) bool {
	return url != "" && lipgloss.Width(url) <= MaxInlineWidth
}

// Hyperlink wraps label in an OSC 8 hyperlink to url. Terminals that support
// OSC 8 show only the label and open url when it is clicked.
func Hyperlink(url, label string) string {
	return escOSC + "8;;" + sanitize(url) + escST + label + escOSC + "8;;" + escST
}

// ClipboardSequence returns the OSC 52 sequence that asks the terminal to put
// text on the system clipboard. Unlike a native clipboard write it travels
// through containers and SSH, but the terminal gives no confirmation.
func ClipboardSequence(text string) string {
	return escOSC + "52;c;" + base64.StdEncoding.EncodeToString([]byte(text)) + escST
}

// sanitize drops control characters so a URL can never terminate the escape
// sequence it is embedded in and inject further sequences.
func sanitize(url string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, url)
}
