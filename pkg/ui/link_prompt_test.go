package ui

import (
	"bytes"
	"errors"
	stdio "io"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	atmosansi "github.com/cloudposse/atmos/pkg/ansi"
	"github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui/link"
)

// errTestClipboardUnavailable stands in for a host with no native clipboard.
var errTestClipboardUnavailable = errors.New("clipboard unavailable")

type linkPromptTestStreams struct {
	output *bytes.Buffer
	error  *bytes.Buffer
}

func (s linkPromptTestStreams) Input() stdio.Reader     { return strings.NewReader("") }
func (s linkPromptTestStreams) Output() stdio.Writer    { return s.output }
func (s linkPromptTestStreams) Error() stdio.Writer     { return s.error }
func (s linkPromptTestStreams) RawOutput() stdio.Writer { return s.output }
func (s linkPromptTestStreams) RawError() stdio.Writer  { return s.error }

// testLongSignInURL builds a URL shaped like the 521-character signin.aws
// device-authorization link, with a mixed-case opaque token like the real one.
func testLongSignInURL() string {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_"
	var token strings.Builder
	for i := 0; i < 387; i++ {
		token.WriteByte(alphabet[(i*37+11)%len(alphabet)])
	}
	return "https://us-east-1.signin.aws/platform/authorize?request_uri=" +
		"urn%3Aietf%3Aparams%3Aoauth%3Arequest_uri%3A" + token.String() +
		"&identitystore_id=d-xxxxxxxxxx"
}

func testLinkPrompt() LinkPrompt {
	return LinkPrompt{URL: testLongSignInURL(), Label: "Open AWS sign-in", Hint: "If the browser doesn't open"}
}

// renderVisible renders a prompt and strips SGR styling, which depends on the
// color profile of the environment running the tests. OSC 8 and OSC 52
// sequences are not SGR, so they survive and can still be asserted.
func renderVisible(prompt LinkPrompt, mode link.Mode, copyFn func(string) error) (string, clipboardOutcome) {
	got, outcome := renderLinkPrompt(prompt, mode, copyFn)
	return atmosansi.Strip(got), outcome
}

// neverCopy fails the test if a mode that must not touch the clipboard does.
func neverCopy(t *testing.T) func(string) error {
	t.Helper()
	return func(string) error {
		t.Error("the clipboard must not be written in this mode")
		return nil
	}
}

func TestRenderLinkPrompt_Raw(t *testing.T) {
	prompt := testLinkPrompt()

	got, outcome := renderVisible(prompt, link.ModeRaw, neverCopy(t))
	assert.Equal(t, clipboardUntouched, outcome)

	assert.Contains(t, got, "If the browser doesn't open, visit:")
	assert.Equal(t, 1, strings.Count(got, prompt.URL), "the URL is printed once, in one piece")
	assert.NotContains(t, got, "\x1b]8;;", "raw mode emits no hyperlink sequence")
	assert.NotContains(t, got, "\x1b]52;", "raw mode emits no clipboard sequence")
	assert.NotContains(t, got, prompt.Label)
}

func TestRenderLinkPrompt_OSC8(t *testing.T) {
	prompt := testLinkPrompt()

	got, outcome := renderVisible(prompt, link.ModeOSC8, neverCopy(t))
	assert.Equal(t, clipboardUntouched, outcome)

	assert.Contains(t, got, "If the browser doesn't open, click:")
	assert.Contains(t, got, "\x1b]8;;"+prompt.URL+"\x1b\\", "the full URL is the hyperlink target")
	assert.Contains(t, got, prompt.Label)
	assert.True(t, strings.HasSuffix(got, "\x1b]8;;\x1b\\\n"), "the hyperlink is closed before the newline")
	assert.Equal(t, 1, strings.Count(got, prompt.URL), "the URL appears only as the link target, not as visible text")
}

func TestRenderLinkPrompt_ClipboardCopied(t *testing.T) {
	prompt := testLinkPrompt()
	var copied string

	got, outcome := renderVisible(prompt, link.ModeClipboard, func(text string) error {
		copied = text
		return nil
	})

	assert.Equal(t, prompt.URL, copied)
	assert.Equal(t, clipboardCopied, outcome)
	assert.Equal(t, "If the browser doesn't open, paste it into your browser.\n", got)
	assert.NotContains(t, got, prompt.URL, "a confirmed copy does not also print the URL")
	assert.NotContains(t, got, "\x1b]52;")
}

func TestRenderLinkPrompt_ClipboardUnavailableFallsBackToRaw(t *testing.T) {
	prompt := testLinkPrompt()

	got, outcome := renderVisible(prompt, link.ModeClipboard, func(string) error {
		return errTestClipboardUnavailable
	})

	assert.Equal(t, clipboardUnconfirmed, outcome)
	// The terminal may still honor OSC 52, but that cannot be confirmed, so the
	// URL is printed and nothing claims a copy happened.
	assert.Contains(t, got, link.ClipboardSequence(prompt.URL))
	assert.Contains(t, got, "If the browser doesn't open, visit:")
	assert.Contains(t, got, prompt.URL+"\n")
}

// initLinkPromptFormatter points the global formatter at in-memory streams and
// returns the stderr buffer.
func initLinkPromptFormatter(t *testing.T) *bytes.Buffer {
	t.Helper()

	var stdout, stderr bytes.Buffer
	ioCtx, err := io.NewContext(io.WithStreams(linkPromptTestStreams{output: &stdout, error: &stderr}))
	require.NoError(t, err)
	InitFormatter(ioCtx)
	t.Cleanup(Reset)
	return &stderr
}

func TestWriteLinkPrompt_NonTTYAlwaysPrintsRawURL(t *testing.T) {
	prompt := testLinkPrompt()

	// Every configured mode degrades to the raw URL when stderr is not a
	// terminal, and none of them may touch the clipboard.
	for _, mode := range []string{"", "auto", "osc8", "clipboard", "raw"} {
		t.Run("mode="+mode, func(t *testing.T) {
			stderr := initLinkPromptFormatter(t)
			SetLinkMode(mode)
			t.Cleanup(func() { SetLinkMode("") })
			original := copyToClipboard
			copyToClipboard = neverCopy(t)
			t.Cleanup(func() { copyToClipboard = original })

			WriteLinkPrompt(prompt)

			got := stderr.String()
			// The realistic, opaque URL must survive the masking layer intact.
			assert.Equal(t, 1, strings.Count(got, prompt.URL))
			assert.Contains(t, got, "If the browser doesn't open, visit:")
			assert.NotContains(t, got, "\x1b]8;;")
			assert.NotContains(t, got, "\x1b]52;")
		})
	}
}

func TestWriteLinkPrompt_EmptyURLWritesNothing(t *testing.T) {
	stderr := initLinkPromptFormatter(t)

	WriteLinkPrompt(LinkPrompt{Label: "Open AWS sign-in", Hint: "If the browser doesn't open"})

	assert.Empty(t, stderr.String())
}

func TestWriteLinkPrompt_BeforeInitFormatterDoesNotPanic(t *testing.T) {
	Reset()

	assert.NotPanics(t, func() { WriteLinkPrompt(testLinkPrompt()) })
}

func TestSetLinkMode(t *testing.T) {
	t.Cleanup(func() { SetLinkMode("") })

	SetLinkMode("clipboard")
	assert.Equal(t, link.ModeClipboard, configuredLinkMode())

	SetLinkMode("")
	assert.Equal(t, link.ModeAuto, configuredLinkMode())

	// An unknown value must not leave a previous non-default mode in place.
	SetLinkMode("raw")
	SetLinkMode("not-a-mode")
	assert.Equal(t, link.ModeAuto, configuredLinkMode())
}

func TestRenderLinkPrompt_WithoutHint(t *testing.T) {
	prompt := LinkPrompt{URL: testLongSignInURL(), Label: "Open console"}

	t.Run("raw introduces the URL with the label", func(t *testing.T) {
		got, _ := renderVisible(prompt, link.ModeRaw, neverCopy(t))

		assert.True(t, strings.HasPrefix(got, "Open console:\n"), "got %q", got[:40])
		assert.Equal(t, 1, strings.Count(got, prompt.URL))
		assert.NotContains(t, got, ", visit:")
	})

	t.Run("osc8 shows only the clickable label", func(t *testing.T) {
		got, _ := renderVisible(prompt, link.ModeOSC8, neverCopy(t))

		assert.True(t, strings.HasPrefix(got, "\x1b]8;;"+prompt.URL+"\x1b\\"), "the line starts with the hyperlink, with no lead-in")
		assert.Contains(t, got, "Open console")
		assert.NotContains(t, got, "click:")
	})

	t.Run("a confirmed clipboard copy needs no further text", func(t *testing.T) {
		got, outcome := renderVisible(prompt, link.ModeClipboard, func(string) error { return nil })

		assert.Equal(t, clipboardCopied, outcome)
		assert.Empty(t, got, "the status message alone reports the copy")
	})
}

// forceTTYLinkPromptFormatter makes the UI treat its in-memory stderr as a
// terminal, so clipboard mode is honored, and returns the stderr buffer.
func forceTTYLinkPromptFormatter(t *testing.T) *bytes.Buffer {
	t.Helper()

	viper.Set("force-tty", true)
	t.Cleanup(func() { viper.Set("force-tty", false) })
	return initLinkPromptFormatter(t)
}

func TestWriteLinkPrompt_ClipboardReportsCopyAsStatus(t *testing.T) {
	prompt := testLinkPrompt()
	stderr := forceTTYLinkPromptFormatter(t)
	SetLinkMode("clipboard")
	t.Cleanup(func() { SetLinkMode("") })
	original := copyToClipboard
	var copied string
	copyToClipboard = func(text string) error {
		copied = text
		return nil
	}
	t.Cleanup(func() { copyToClipboard = original })

	WriteLinkPrompt(prompt)

	// Status messages are styled, so compare the visible text.
	got := atmosansi.Strip(stderr.String())
	require.Equal(t, prompt.URL, copied, "prerequisite: clipboard mode must be active under a forced TTY")
	assert.Contains(t, got, clipboardCopiedMessage)
	assert.Contains(t, got, "paste it into your browser")
	assert.NotContains(t, got, prompt.URL, "a confirmed copy does not also print the URL")
}

func TestWriteLinkPrompt_ClipboardFailureWarnsAndPrintsURL(t *testing.T) {
	prompt := testLinkPrompt()
	stderr := forceTTYLinkPromptFormatter(t)
	SetLinkMode("clipboard")
	t.Cleanup(func() { SetLinkMode("") })
	original := copyToClipboard
	copyToClipboard = func(string) error { return errTestClipboardUnavailable }
	t.Cleanup(func() { copyToClipboard = original })

	WriteLinkPrompt(prompt)

	raw := stderr.String()
	// Status messages are styled, so compare the visible text.
	got := atmosansi.Strip(raw)
	assert.Contains(t, got, clipboardUnconfirmedMessage)
	assert.NotContains(t, got, clipboardCopiedMessage)
	assert.Contains(t, raw, link.ClipboardSequence(prompt.URL), "an OSC 52 copy is still requested")
	assert.Contains(t, got, prompt.URL, "the URL is printed so it is never lost")
}
