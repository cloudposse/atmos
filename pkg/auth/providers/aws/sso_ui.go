package aws

import (
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/ssooidc"
	"github.com/charmbracelet/lipgloss"

	"github.com/cloudposse/atmos/pkg/telemetry"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/link"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

const (
	// Clickable text shown when the verification URL is delivered as a terminal hyperlink.
	ssoSignInLinkLabel = "Open AWS sign-in"
	// Lead-in that says when the user needs the verification URL.
	ssoSignInLinkHint = "If the browser doesn't open"
	// Blank line between sections of the dialog.
	dialogSectionBreak = "\n\n"
	// Ends a link prompt with a blank line, matching the dialog's bottom margin.
	blankLine = "\n"
)

// fallbackVerificationURL returns the URL to show the user in case the browser
// does not open. It prefers the short verification URI, where the user types
// the code shown in the dialog (RFC 8628). The complete URI embeds the code and
// is what the browser opens, but AWS IAM Identity Center now returns complete
// URIs of several hundred characters, which are unwieldy to read or copy. The
// complete URI is used only when AWS returns no short one.
func fallbackVerificationURL(authResp *ssooidc.StartDeviceAuthorizationOutput) string {
	if authResp == nil {
		return ""
	}
	if authResp.VerificationUri != nil && *authResp.VerificationUri != "" {
		return *authResp.VerificationUri
	}
	if authResp.VerificationUriComplete != nil {
		return *authResp.VerificationUriComplete
	}
	return ""
}

// displayVerificationDialog shows a styled dialog with the verification code.
// A verification URL too long for the dialog is delivered below it, outside the
// border, in the form `settings.terminal.links` selects.
func displayVerificationDialog(code, url string) {
	dialog, containsURL := renderVerificationDialog(code, url)
	ui.Writef("%s\n", dialog)
	if containsURL {
		return
	}

	ui.WriteLinkPrompt(ui.LinkPrompt{URL: url, Label: ssoSignInLinkLabel, Hint: ssoSignInLinkHint})
	// Match the blank line the dialog's bottom margin leaves before the spinner.
	ui.Write(blankLine)
}

// renderVerificationDialog builds the device-authorization dialog and reports
// whether the verification URL fits inside it. AWS IAM Identity Center returns
// URLs of several hundred characters, which would stretch the box far past the
// terminal width and wrap its border into the URL.
func renderVerificationDialog(code, url string) (string, bool) {
	// Styles using Atmos theme colors.
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(theme.GetCurrentColorScheme().Link)).
		PaddingLeft(1).
		PaddingRight(1)

	codeStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(theme.GetCurrentColorScheme().Success)).
		Background(lipgloss.Color(theme.GetCurrentColorScheme().Surface)).
		Padding(0, 1)

	urlStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.GetCurrentColorScheme().Border)).
		Italic(true)

	instructionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.GetCurrentColorScheme().TextMuted))

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(theme.GetCurrentColorScheme().Border)).
		Padding(1, 2).
		MarginTop(1).
		MarginBottom(1)

	// Build the content.
	var content strings.Builder
	content.WriteString(titleStyle.Render("🔐 AWS SSO Authentication Required"))
	content.WriteString(dialogSectionBreak)
	content.WriteString("Verification Code: ")
	content.WriteString(codeStyle.Render(code))

	containsURL := link.FitsInline(url)
	switch {
	case containsURL:
		content.WriteString(dialogSectionBreak)
		content.WriteString(urlStyle.Render(url))
		content.WriteString(dialogSectionBreak)
		content.WriteString(instructionStyle.Render("Opening browser... If it doesn't open, visit the URL above and enter the code."))
	case url != "":
		content.WriteString(dialogSectionBreak)
		content.WriteString(instructionStyle.Render("Opening browser for authentication..."))
	}

	return boxStyle.Render(content.String()), containsURL
}

// displayVerificationPlainText shows verification code in plain text (for non-TTY/CI).
func displayVerificationPlainText(code, url string) {
	ui.Writef("🔐 **AWS SSO Authentication Required**\n")
	ui.Writef("Verification Code: **%s**\n", code)

	if url != "" {
		if !telemetry.IsCI() {
			ui.Writef("Opening browser for authentication...\n")
			ui.Writef("If the browser does not open, visit %s and enter the code.\n", url)
		} else {
			ui.Writef("Verification URL: %s\n", url)
		}
	}

	ui.Writef("Waiting for authentication...\n")
}
