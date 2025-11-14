package azure

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/AzureAD/microsoft-authentication-library-for-go/apps/public"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-isatty"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	"github.com/cloudposse/atmos/pkg/utils"
)

// waitForAuthWithSpinner waits for device code authentication to complete with a spinner.
func waitForAuthWithSpinner(authCtx context.Context, deviceCode *public.DeviceCode) (string, time.Time, error) {
	resultCh := make(chan struct {
		token     string
		expiresOn time.Time
		err       error
	}, 1)

	// Start authentication in background.
	go func() {
		result, err := deviceCode.AuthenticationResult(authCtx)
		if err != nil {
			resultCh <- struct {
				token     string
				expiresOn time.Time
				err       error
			}{"", time.Time{}, err}
			return
		}
		resultCh <- struct {
			token     string
			expiresOn time.Time
			err       error
		}{result.AccessToken, result.ExpiresOn, nil}
	}()

	// Run spinner.
	model := newSpinnerModel()
	prog := tea.NewProgram(model, tea.WithOutput(os.Stderr))

	go func() {
		result := <-resultCh
		prog.Send(authCompleteMsg{
			token:     result.token,
			expiresOn: result.expiresOn,
			err:       result.err,
		})
	}()

	finalModel, err := prog.Run()
	if err != nil {
		return "", time.Time{}, fmt.Errorf("%w: failed to run spinner: %w", errUtils.ErrAuthenticationFailed, err)
	}

	m := finalModel.(*spinnerModel)
	if m.authErr != nil {
		return "", time.Time{}, m.authErr
	}

	return m.token, m.expiresOn, nil
}

// displayDeviceCodePrompt displays the device code, opens the browser, and waits for user authentication.
func displayDeviceCodePrompt(userCode, verificationURL string) {
	log.Debug("Displaying Azure authentication prompt",
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
	// Azure supports pre-filling the code with ?otc=CODE parameter.
	if isTTY() && verificationURL != "" {
		urlToOpen := fmt.Sprintf("%s?otc=%s", verificationURL, userCode)
		if err := utils.OpenUrl(urlToOpen); err != nil {
			log.Debug("Failed to open browser automatically", "error", err)
		} else {
			log.Debug("Browser opened successfully", "url", urlToOpen)
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
	fmt.Fprintln(os.Stderr, titleStyle.Render("🔐 Azure Authentication Required"))
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
	fmt.Fprintln(os.Stderr, "🔐 Azure Authentication Required")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintf(os.Stderr, "Verification Code: %s\n", code)
	fmt.Fprintf(os.Stderr, "Verification URL:  %s\n", url)
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Please open the URL above and enter the verification code to authenticate.")
	fmt.Fprintln(os.Stderr, "")
}

// Spinner model for authentication polling.

type authCompleteMsg struct {
	token     string
	expiresOn time.Time
	err       error
}

type spinnerModel struct {
	spinner   spinner.Model
	token     string
	expiresOn time.Time
	authErr   error
	quitting  bool
}

func newSpinnerModel() *spinnerModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorCyan))
	return &spinnerModel{spinner: s}
}

func (m *spinnerModel) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m *spinnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case authCompleteMsg:
		m.token = msg.token
		m.expiresOn = msg.expiresOn
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

func (m *spinnerModel) View() string {
	if m.quitting {
		if m.authErr != nil {
			return ""
		}
		successStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorGreen))
		return successStyle.Render("✓") + " Authentication successful!\n"
	}
	return m.spinner.View() + " Waiting for authentication...\n"
}
