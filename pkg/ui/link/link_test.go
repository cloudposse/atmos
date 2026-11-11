package link

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// envOf builds a terminal Environment from a fixed set of variables.
func envOf(isTTY bool, vars map[string]string) Environment {
	return Environment{
		IsTTY: isTTY,
		Lookup: func(name string) (string, bool) {
			value, ok := vars[name]
			return value, ok
		},
	}
}

func TestParseMode(t *testing.T) {
	tests := []struct {
		value string
		want  Mode
		ok    bool
	}{
		{value: "", want: ModeAuto, ok: true},
		{value: "auto", want: ModeAuto, ok: true},
		{value: "osc8", want: ModeOSC8, ok: true},
		{value: "clipboard", want: ModeClipboard, ok: true},
		{value: "raw", want: ModeRaw, ok: true},
		{value: "  OSC8 ", want: ModeOSC8, ok: true},
		{value: "hyperlink", want: ModeAuto, ok: false},
		{value: "true", want: ModeAuto, ok: false},
	}

	for _, tt := range tests {
		t.Run("value="+tt.value, func(t *testing.T) {
			got, ok := ParseMode(tt.value)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.ok, ok)
		})
	}
}

func TestSupportsHyperlinks(t *testing.T) {
	tests := []struct {
		name  string
		isTTY bool
		vars  map[string]string
		want  bool
	}{
		{name: "iTerm2 at the minimum version", isTTY: true, vars: map[string]string{"TERM_PROGRAM": "iTerm.app", "TERM_PROGRAM_VERSION": "3.1.0"}, want: true},
		{name: "iTerm2 newer major", isTTY: true, vars: map[string]string{"TERM_PROGRAM": "iTerm.app", "TERM_PROGRAM_VERSION": "4.0.1"}, want: true},
		{name: "iTerm2 too old", isTTY: true, vars: map[string]string{"TERM_PROGRAM": "iTerm.app", "TERM_PROGRAM_VERSION": "3.0.15"}, want: false},
		{name: "iTerm2 without a version", isTTY: true, vars: map[string]string{"TERM_PROGRAM": "iTerm.app"}, want: false},
		{name: "iTerm2 with an unparseable version", isTTY: true, vars: map[string]string{"TERM_PROGRAM": "iTerm.app", "TERM_PROGRAM_VERSION": "beta.x"}, want: false},
		{name: "VS Code at the minimum version", isTTY: true, vars: map[string]string{"TERM_PROGRAM": "vscode", "TERM_PROGRAM_VERSION": "1.72.0"}, want: true},
		{name: "VS Code too old", isTTY: true, vars: map[string]string{"TERM_PROGRAM": "vscode", "TERM_PROGRAM_VERSION": "1.71.2"}, want: false},
		{name: "WezTerm", isTTY: true, vars: map[string]string{"TERM_PROGRAM": "WezTerm"}, want: true},
		{name: "Ghostty by TERM_PROGRAM", isTTY: true, vars: map[string]string{"TERM_PROGRAM": "ghostty"}, want: true},
		{name: "Ghostty by TERM", isTTY: true, vars: map[string]string{"TERM": "xterm-ghostty"}, want: true},
		{name: "kitty by TERM", isTTY: true, vars: map[string]string{"TERM": "xterm-kitty"}, want: true},
		{name: "kitty by window id", isTTY: true, vars: map[string]string{"KITTY_WINDOW_ID": "1"}, want: true},
		{name: "Windows Terminal", isTTY: true, vars: map[string]string{"WT_SESSION": "abc"}, want: true},
		{name: "Orca terminal pane", isTTY: true, vars: map[string]string{"ORCA_PANE_KEY": "tab-1:pane-1"}, want: true},
		{name: "Orca by TERM_PROGRAM", isTTY: true, vars: map[string]string{"TERM_PROGRAM": "Orca"}, want: true},
		{name: "FORCE_HYPERLINK=1 enables an unknown terminal", isTTY: true, vars: map[string]string{"FORCE_HYPERLINK": "1"}, want: true},
		{name: "FORCE_HYPERLINK=0 disables a known terminal", isTTY: true, vars: map[string]string{"FORCE_HYPERLINK": "0", "TERM_PROGRAM": "ghostty"}, want: false},
		{name: "FORCE_HYPERLINK overrides tmux", isTTY: true, vars: map[string]string{"FORCE_HYPERLINK": "1", "TMUX": "/tmp/tmux-501/default,1,0"}, want: true},
		{name: "empty FORCE_HYPERLINK enables", isTTY: true, vars: map[string]string{"FORCE_HYPERLINK": ""}, want: true},
		{name: "FORCE_HYPERLINK cannot enable non-TTY output", isTTY: false, vars: map[string]string{"FORCE_HYPERLINK": "1"}, want: false},
		{name: "Orca pane inside tmux", isTTY: true, vars: map[string]string{"ORCA_PANE_KEY": "tab-1:pane-1", "TMUX": "/tmp/tmux-501/default,1,0"}, want: false},
		{name: "VTE at the minimum version", isTTY: true, vars: map[string]string{"VTE_VERSION": "5000"}, want: true},
		{name: "VTE too old", isTTY: true, vars: map[string]string{"VTE_VERSION": "4999"}, want: false},
		{name: "Konsole at the minimum version", isTTY: true, vars: map[string]string{"KONSOLE_VERSION": "201200"}, want: true},
		{name: "Konsole too old", isTTY: true, vars: map[string]string{"KONSOLE_VERSION": "201199"}, want: false},
		{name: "Apple Terminal", isTTY: true, vars: map[string]string{"TERM_PROGRAM": "Apple_Terminal", "TERM_PROGRAM_VERSION": "455"}, want: false},
		{name: "unidentified terminal", isTTY: true, vars: map[string]string{"TERM": "xterm-256color"}, want: false},
		{name: "supported terminal that is not a TTY", isTTY: false, vars: map[string]string{"TERM_PROGRAM": "WezTerm"}, want: false},
		{name: "dumb terminal overrides identification", isTTY: true, vars: map[string]string{"TERM": "dumb", "WT_SESSION": "abc"}, want: false},
		{name: "tmux hides the outer terminal", isTTY: true, vars: map[string]string{"TMUX": "/tmp/tmux-501/default,1,0", "KITTY_WINDOW_ID": "1"}, want: false},
		{name: "screen hides the outer terminal", isTTY: true, vars: map[string]string{"STY": "123.pts-0", "TERM_PROGRAM": "WezTerm"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, SupportsHyperlinks(envOf(tt.isTTY, tt.vars)))
		})
	}
}

func TestSupportsHyperlinks_NilLookup(t *testing.T) {
	assert.False(t, SupportsHyperlinks(Environment{IsTTY: true}))
}

func TestResolve(t *testing.T) {
	supported := map[string]string{"TERM_PROGRAM": "WezTerm"}
	unsupported := map[string]string{"TERM": "xterm-256color"}

	tests := []struct {
		name string
		mode Mode
		env  Environment
		want Mode
	}{
		{name: "auto uses a hyperlink when detected", mode: ModeAuto, env: envOf(true, supported), want: ModeOSC8},
		{name: "auto uses the raw URL when not detected", mode: ModeAuto, env: envOf(true, unsupported), want: ModeRaw},
		{name: "osc8 overrides failed detection", mode: ModeOSC8, env: envOf(true, unsupported), want: ModeOSC8},
		{name: "raw overrides successful detection", mode: ModeRaw, env: envOf(true, supported), want: ModeRaw},
		{name: "clipboard is honored on a TTY", mode: ModeClipboard, env: envOf(true, unsupported), want: ModeClipboard},
		{name: "auto without a TTY is raw", mode: ModeAuto, env: envOf(false, supported), want: ModeRaw},
		{name: "osc8 without a TTY is raw", mode: ModeOSC8, env: envOf(false, supported), want: ModeRaw},
		{name: "clipboard without a TTY is raw", mode: ModeClipboard, env: envOf(false, supported), want: ModeRaw},
		{name: "an unknown mode behaves like auto", mode: Mode("bogus"), env: envOf(true, unsupported), want: ModeRaw},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, Resolve(tt.mode, tt.env))
		})
	}
}

func TestFitsInline(t *testing.T) {
	urlWithWidth := func(width int) string {
		return "https://" + strings.Repeat("x", width-len("https://"))
	}

	assert.True(t, FitsInline(urlWithWidth(MaxInlineWidth)))
	assert.False(t, FitsInline(urlWithWidth(MaxInlineWidth+1)))
	assert.False(t, FitsInline(""), "an empty URL has nothing to show inline")
}

func TestHyperlink(t *testing.T) {
	got := Hyperlink("https://example.com/a?b=c", "Open sign-in")

	assert.Equal(t, "\x1b]8;;https://example.com/a?b=c\x1b\\Open sign-in\x1b]8;;\x1b\\", got)
}

func TestHyperlink_StripsControlCharactersFromURL(t *testing.T) {
	// A URL carrying an escape or a terminator must not be able to close the
	// hyperlink early and inject its own sequence.
	got := Hyperlink("https://example.com/\x1b\\\x1b]52;c;AAAA\x07\x7f", "label")

	target := strings.TrimSuffix(strings.TrimPrefix(got, "\x1b]8;;"), "\x1b\\label\x1b]8;;\x1b\\")
	require.NotEqual(t, got, target, "the hyperlink framing must be intact")
	assert.Equal(t, "https://example.com/\\]52;c;AAAA", target)
	assert.NotContains(t, target, "\x1b")
	assert.NotContains(t, target, "\x07")
}

func TestClipboardSequence(t *testing.T) {
	const text = "https://example.com/authorize?request_uri=abc"

	got := ClipboardSequence(text)

	require.True(t, strings.HasPrefix(got, "\x1b]52;c;"))
	require.True(t, strings.HasSuffix(got, "\x1b\\"))
	payload := strings.TrimSuffix(strings.TrimPrefix(got, "\x1b]52;c;"), "\x1b\\")
	decoded, err := base64.StdEncoding.DecodeString(payload)
	require.NoError(t, err)
	assert.Equal(t, text, string(decoded))
}
