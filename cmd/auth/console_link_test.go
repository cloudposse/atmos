package auth

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
)

// errTestBrowserUnavailable stands in for a host that cannot launch a browser.
var errTestBrowserUnavailable = errors.New("browser unavailable")

type consoleLinkTestStreams struct {
	output *bytes.Buffer
	error  *bytes.Buffer
}

func (s consoleLinkTestStreams) Input() io.Reader     { return strings.NewReader("") }
func (s consoleLinkTestStreams) Output() io.Writer    { return s.output }
func (s consoleLinkTestStreams) Error() io.Writer     { return s.error }
func (s consoleLinkTestStreams) RawOutput() io.Writer { return s.output }
func (s consoleLinkTestStreams) RawError() io.Writer  { return s.error }

// captureConsoleUI points the UI at in-memory streams and returns them.
func captureConsoleUI(t *testing.T) (stdout, stderr *bytes.Buffer) {
	t.Helper()

	stdout, stderr = &bytes.Buffer{}, &bytes.Buffer{}
	ioCtx, err := iolib.NewContext(iolib.WithStreams(consoleLinkTestStreams{output: stdout, error: stderr}))
	require.NoError(t, err)
	ui.InitFormatter(ioCtx)
	t.Cleanup(ui.Reset)
	return stdout, stderr
}

// longConsoleURL builds a URL shaped like an AWS federation sign-in link.
func longConsoleURL() string {
	return "https://signin.aws.amazon.com/federation?Action=login&Issuer=atmos&Destination=" +
		"https%3A%2F%2Fconsole.aws.amazon.com%2F&SigninToken=" + strings.Repeat("t", 1400)
}

// TestPrintConsoleURL_NonTTYPrintsRawURL verifies the console URL reaches the
// UI channel exactly once and intact when output is not a terminal, whatever
// link mode is configured, and that nothing is written to stdout.
func TestPrintConsoleURL_NonTTYPrintsRawURL(t *testing.T) {
	url := longConsoleURL()

	for _, mode := range []string{"", "osc8", "clipboard", "raw"} {
		t.Run("mode="+mode, func(t *testing.T) {
			stdout, stderr := captureConsoleUI(t)
			ui.SetLinkMode(mode)
			t.Cleanup(func() { ui.SetLinkMode("") })

			printConsoleURL(url)

			got := stderr.String()
			assert.Equal(t, 1, strings.Count(got, url))
			assert.Contains(t, got, consoleLinkLabel+":")
			assert.NotContains(t, got, "\x1b]8;;", "no hyperlink sequence off a terminal")
			assert.NotContains(t, got, "\x1b]52;", "no clipboard sequence off a terminal")
			assert.Empty(t, stdout.String(), "the console URL prompt belongs on the UI channel")
		})
	}
}

// TestHandleBrowserOpen_ShowsURLWhenNotOpened verifies every path that does not
// open a browser still hands the URL to the user.
func TestHandleBrowserOpen_ShowsURLWhenNotOpened(t *testing.T) {
	url := longConsoleURL()

	t.Run("skipOpen shows the URL without calling the opener", func(t *testing.T) {
		_, stderr := captureConsoleUI(t)
		op := &fakeOpener{}

		handleBrowserOpen(url, true, op)

		assert.Zero(t, op.calls)
		assert.Equal(t, 1, strings.Count(stderr.String(), url))
	})

	t.Run("nil opener shows the URL", func(t *testing.T) {
		_, stderr := captureConsoleUI(t)

		handleBrowserOpen(url, false, nil)

		assert.Equal(t, 1, strings.Count(stderr.String(), url))
	})

	t.Run("a failed or skipped open shows the URL", func(t *testing.T) {
		_, stderr := captureConsoleUI(t)
		op := &fakeOpener{openErr: errTestBrowserUnavailable}

		// In CI the opener is skipped and the URL is shown; elsewhere the open
		// fails and the URL is shown with a warning. Either way it is shown once.
		handleBrowserOpen(url, false, op)

		assert.Equal(t, 1, strings.Count(stderr.String(), url))
	})
}
