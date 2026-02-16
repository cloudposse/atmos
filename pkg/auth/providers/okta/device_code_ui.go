package okta

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-isatty"

	errUtils "github.com/cloudposse/atmos/errors"
	oktaCloud "github.com/cloudposse/atmos/pkg/auth/cloud/okta"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	"github.com/cloudposse/atmos/pkg/utils"
)

// pollForTokenWithSpinner polls for token with a spinner UI.
func pollForTokenWithSpinner(ctx context.Context, provider *deviceCodeProvider, deviceAuth *oktaCloud.DeviceAuthorizationResponse) (*oktaCloud.OktaTokens, error) {
	resultCh := make(chan struct {
		tokens *oktaCloud.OktaTokens
		err    error
	}, 1)

	// Start token polling in background.
	go func() {
		tokens, err := provider.pollForToken(ctx, deviceAuth)
		resultCh <- struct {
			tokens *oktaCloud.OktaTokens
			err    error
		}{tokens, err}
	}()

	// Run spinner.
	model := newOktaSpinnerModel()
	prog := tea.NewProgram(model, tea.WithOutput(os.Stderr))

	go func() {
		result := <-resultCh
		prog.Send(oktaAuthCompleteMsg{
			tokens: result.tokens,
			err:    result.err,
		})
	}()

	finalModel, err := prog.Run()
	if err != nil {
		return nil, fmt.Errorf("%w: failed to run spinner: %w", errUtils.ErrAuthenticationFailed, err)
	}

	m := finalModel.(*oktaSpinnerModel)
	if m.authErr != nil {
		return nil, m.authErr
	}

	return m.tokens, nil
}

// displayDeviceCodePrompt displays the device code, opens the browser, and waits for user authentication.
func displayDeviceCodePrompt(userCode, verificationURL string) {
	log.Debug("Displaying Okta authentication prompt",
		"url", verificationURL,
		"code", userCode,
		"isTTY", isTTY(),
	)

	// Check if we have a TTY for fancy output.
	if isTTY() {
		displayVerificationDialog(userCode, verificationURL)
	} else {
		// Fallback to simple text output for non-TTY or CI environments.
		displayVerificationPlainText(userCode, verificationURL)
	}

	// Open browser if in interactive terminal.
	if isTTY() && verificationURL != "" {
		if err := utils.OpenUrl(verificationURL); err != nil {
			log.Debug("Failed to open browser automatically", "error", err)
		} else {
			log.Debug("Browser opened successfully", "url", verificationURL)
		}
	}
	log.Debug("Finished displaying device code prompt, waiting for user authentication")
}

// isTTY checks if stderr is a terminal.
func isTTY() bool {
	return isatty.IsTerminal(os.Stderr.Fd()) || isatty.IsCygwinTerminal(os.Stderr.Fd())
}

// displayVerificationDialog shows a styled dialog with the verification code.
func displayVerificationDialog(code, url string) {
	// Simpler, clearer output without complex borders.
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(theme.ColorCyan))

	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.ColorGray))

	codeStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(theme.ColorGreen)).
		Background(lipgloss.Color("#1a1a1a")).
		Padding(0, 2)

	urlStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.ColorBlue))

	// Build simple, readable output.
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, titleStyle.Render("🔐 Okta Authentication Required"))
	fmt.Fprintln(os.Stderr)
	fmt.Fprintf(os.Stderr, "%s  %s\n", labelStyle.Render("Verification Code:"), codeStyle.Render(code))
	fmt.Fprintln(os.Stderr)
	fmt.Fprintf(os.Stderr, "%s  %s\n", labelStyle.Render("Verification URL:"), urlStyle.Render(url))
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, labelStyle.Render("Opening browser..."))
	fmt.Fprintln(os.Stderr)
}

// displayVerificationPlainText shows plain text authentication prompt.
func displayVerificationPlainText(code, url string) {
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "🔐 Okta Authentication Required")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintf(os.Stderr, "Verification Code: %s\n", code)
	fmt.Fprintf(os.Stderr, "Verification URL:  %s\n", url)
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Please open the URL above and enter the verification code to authenticate.")
	fmt.Fprintln(os.Stderr, "")
}

// Spinner model for Okta authentication polling.

type oktaAuthCompleteMsg struct {
	tokens *oktaCloud.OktaTokens
	err    error
}

type oktaSpinnerModel struct {
	spinner  spinner.Model
	tokens   *oktaCloud.OktaTokens
	authErr  error
	quitting bool
}

func newOktaSpinnerModel() *oktaSpinnerModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorCyan))
	return &oktaSpinnerModel{spinner: s}
}

func (m *oktaSpinnerModel) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m *oktaSpinnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case oktaAuthCompleteMsg:
		m.tokens = msg.tokens
		m.authErr = msg.err
		m.quitting = true
		return m, tea.Quit

	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			m.quitting = true
			m.authErr = fmt.Errorf("%w: authentication cancelled by user", errUtils.ErrAuthenticationFailed)
			return m, tea.Quit
		}

	default:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m *oktaSpinnerModel) View() string {
	if m.quitting {
		if m.authErr != nil {
			return ""
		}
		successStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorGreen))
		return successStyle.Render("✓") + " Authentication successful!\n"
	}
	return m.spinner.View() + " Waiting for Okta authentication...\n"
}

// Silence unused variable warning for time import.
var _ = time.Second
