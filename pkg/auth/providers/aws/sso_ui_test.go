package aws

// Tests for the device-authorization prompt (sso_ui.go).

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/ssooidc"
	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/link"
)

// testShortVerificationURL is the legacy device-code URL, which fits in the dialog.
const testShortVerificationURL = "https://device.sso.us-east-1.amazonaws.com/?user_code=ABCD-1234"

// maxVerificationDialogWidth is the widest the dialog may render: the inline URL
// limit plus the box border and horizontal padding.
const maxVerificationDialogWidth = link.MaxInlineWidth + 6

type ssoDialogTestStreams struct {
	output *bytes.Buffer
	error  *bytes.Buffer
}

func (s ssoDialogTestStreams) Input() io.Reader     { return strings.NewReader("") }
func (s ssoDialogTestStreams) Output() io.Writer    { return s.output }
func (s ssoDialogTestStreams) Error() io.Writer     { return s.error }
func (s ssoDialogTestStreams) RawOutput() io.Writer { return s.output }
func (s ssoDialogTestStreams) RawError() io.Writer  { return s.error }

// longVerificationURL builds a URL shaped like the signin.aws device-authorization
// link AWS IAM Identity Center now returns (521 characters).
func longVerificationURL() string {
	return "https://us-east-1.signin.aws/platform/authorize?request_uri=" +
		"urn%3Aietf%3Aparams%3Aoauth%3Arequest_uri%3A" + strings.Repeat("x", 387) +
		"&identitystore_id=d-xxxxxxxxxx"
}

// captureVerificationDialogOutput renders the dialog through the real UI layer
// and returns everything written to stderr.
func captureVerificationDialogOutput(t *testing.T, code, url string) string {
	t.Helper()

	return captureVerificationOutput(t, func() { displayVerificationDialog(code, url) })
}

// captureVerificationOutput runs render with the UI formatter writing to a
// buffer and returns what it wrote to stderr.
func captureVerificationOutput(t *testing.T, render func()) string {
	t.Helper()

	var stdout, stderr bytes.Buffer
	ioCtx, err := iolib.NewContext(iolib.WithStreams(ssoDialogTestStreams{
		output: &stdout,
		error:  &stderr,
	}))
	require.NoError(t, err)
	ui.InitFormatter(ioCtx)
	t.Cleanup(func() {
		defaultIOCtx, initErr := iolib.NewContext()
		require.NoError(t, initErr)
		ui.InitFormatter(defaultIOCtx)
	})

	render()
	return stderr.String()
}

// boxLines returns the lines of output that belong to the bordered dialog.
func boxLines(output string) []string {
	var lines []string
	for _, line := range strings.Split(output, "\n") {
		if strings.ContainsAny(line, "│╭╰") {
			lines = append(lines, line)
		}
	}
	return lines
}

func TestDisplayVerificationDialog_LongURLStaysOutsideBox(t *testing.T) {
	url := longVerificationURL()
	require.Len(t, url, 521)

	output := captureVerificationDialogOutput(t, "ABCD-1234", url)

	lines := boxLines(output)
	require.NotEmpty(t, lines, "the dialog box must still render")
	for _, line := range lines {
		assert.LessOrEqual(t, lipgloss.Width(line), maxVerificationDialogWidth, "box line is wider than the dialog limit")
		assert.NotContains(t, line, "signin.aws", "the long URL must not be drawn inside the box")
	}

	// The URL is printed exactly once, in one piece, so it can be copied intact.
	assert.Equal(t, 1, strings.Count(output, url))
	assert.Contains(t, output, "ABCD-1234")
	assert.Contains(t, output, "If the browser doesn't open, visit:")
}

func TestDisplayVerificationDialog_ShortURLStaysInBox(t *testing.T) {
	output := captureVerificationDialogOutput(t, "ABCD-1234", testShortVerificationURL)

	var inBox bool
	for _, line := range boxLines(output) {
		if strings.Contains(line, testShortVerificationURL) {
			inBox = true
		}
	}
	assert.True(t, inBox, "a short URL stays with its instructions inside the box")
	assert.Equal(t, 1, strings.Count(output, testShortVerificationURL))
	assert.NotContains(t, output, "If the browser doesn't open, visit:")
}

func TestRenderVerificationDialog_URLPlacement(t *testing.T) {
	urlWithWidth := func(width int) string {
		return "https://" + strings.Repeat("x", width-len("https://"))
	}

	tests := []struct {
		name        string
		url         string
		containsURL bool
	}{
		{name: "URL at the inline limit stays in the dialog", url: urlWithWidth(link.MaxInlineWidth), containsURL: true},
		{name: "URL one column over the limit leaves the dialog", url: urlWithWidth(link.MaxInlineWidth + 1), containsURL: false},
		{name: "empty URL is never reported as contained", url: "", containsURL: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dialog, containsURL := renderVerificationDialog("ABCD-1234", tt.url)

			assert.Equal(t, tt.containsURL, containsURL)
			assert.Contains(t, dialog, "AWS SSO Authentication Required")
			assert.Contains(t, dialog, "ABCD-1234")
			if tt.url != "" {
				assert.Equal(t, tt.containsURL, strings.Contains(dialog, tt.url))
			}
		})
	}
}

func TestDisplayVerificationDialog_EmptyURLPrintsNoLinkPrompt(t *testing.T) {
	output := captureVerificationDialogOutput(t, "WXYZ-5678", "")

	assert.Contains(t, output, "WXYZ-5678")
	assert.NotContains(t, output, "If the browser doesn't open")
}

func TestDisplayVerificationDialog(t *testing.T) {
	tests := []struct {
		name string
		code string
		url  string
	}{
		{
			name: "with verification code and URL",
			code: "ABCD-1234",
			url:  "https://device.sso.us-east-1.amazonaws.com/",
		},
		{
			name: "with empty code",
			code: "",
			url:  "https://device.sso.us-east-1.amazonaws.com/",
		},
		{
			name: "with empty URL",
			code: "WXYZ-5678",
			url:  "",
		},
		{
			name: "with both empty",
			code: "",
			url:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This function outputs to stderr, so we just verify it doesn't panic.
			assert.NotPanics(t, func() {
				displayVerificationDialog(tt.code, tt.url)
			})
		})
	}
}

func TestFallbackVerificationURL(t *testing.T) {
	short := "https://device.sso.us-east-1.amazonaws.com/"
	long := longVerificationURL()
	empty := ""

	tests := []struct {
		name     string
		resp     *ssooidc.StartDeviceAuthorizationOutput
		expected string
	}{
		{name: "short URI preferred over the complete URL", resp: &ssooidc.StartDeviceAuthorizationOutput{VerificationUri: &short, VerificationUriComplete: &long}, expected: short},
		{name: "complete URL used when no short URI", resp: &ssooidc.StartDeviceAuthorizationOutput{VerificationUriComplete: &long}, expected: long},
		{name: "complete URL used when short URI is empty", resp: &ssooidc.StartDeviceAuthorizationOutput{VerificationUri: &empty, VerificationUriComplete: &long}, expected: long},
		{name: "short URI alone", resp: &ssooidc.StartDeviceAuthorizationOutput{VerificationUri: &short}, expected: short},
		{name: "neither URL", resp: &ssooidc.StartDeviceAuthorizationOutput{}, expected: ""},
		{name: "nil response", resp: nil, expected: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, fallbackVerificationURL(tt.resp))
		})
	}
}

func TestDisplayVerificationDialog_ShortURIStaysInBoxWithCodeInstruction(t *testing.T) {
	short := "https://device.sso.us-east-1.amazonaws.com/"
	output := captureVerificationDialogOutput(t, "ABCD-1234", short)

	assert.Contains(t, output, short)
	assert.Contains(t, output, "enter the code")
	assert.NotContains(t, output, "If the browser doesn't open, visit:")
	for _, line := range boxLines(output) {
		assert.LessOrEqual(t, lipgloss.Width(line), maxVerificationDialogWidth)
	}
}

func TestDisplayVerificationPlainText_CITellsUserToEnterCode(t *testing.T) {
	t.Setenv("CI", "true")
	short := "https://device.sso.us-east-1.amazonaws.com/"
	output := captureVerificationOutput(t, func() { displayVerificationPlainText("ABCD-1234", short) })

	assert.Contains(t, output, "ABCD-1234")
	assert.Contains(t, output, "Visit "+short+" and enter the code.")
	assert.NotContains(t, output, "Opening browser")
}

func TestDisplayVerificationDialog_LinkPromptEndsWithBlankLine(t *testing.T) {
	output := captureVerificationDialogOutput(t, "ABCD-1234", longVerificationURL())

	assert.True(t, strings.HasSuffix(output, "\n\n"), "a blank line separates the link prompt from the spinner")
}
