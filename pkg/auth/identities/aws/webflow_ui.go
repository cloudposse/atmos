package aws

// UI helpers for the browser-based webflow: TTY detection, display dialogs,
// and the bubbletea spinner model used by the interactive path.

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-isatty"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	"github.com/cloudposse/atmos/pkg/utils"
)

// webflowSpinnerPollInterval is the polling interval used by the spinner
// model to check for token-exchange results between bubbletea ticks.
const webflowSpinnerPollInterval = 100 * time.Millisecond

// webflowIsTTY checks if stderr is a terminal.
func webflowIsTTY() bool {
	return isatty.IsTerminal(os.Stderr.Fd()) || isatty.IsCygwinTerminal(os.Stderr.Fd())
}

// webflowIsInteractive checks if we're running in an interactive terminal.
// Respects --force-tty / ATMOS_FORCE_TTY for environments where TTY detection fails.
func webflowIsInteractive() bool {
	if viper.GetBool("force-tty") {
		return true
	}
	return webflowIsTTYFunc()
}

// displayWebflowDialog shows a styled dialog with the authentication URL (TTY mode).
func displayWebflowDialog(authURL string) {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(theme.ColorCyan)).
		PaddingLeft(1).
		PaddingRight(1)

	urlStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.ColorBorder)).
		Italic(true)

	instructionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.ColorDarkGray))

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(theme.ColorBorder)).
		Padding(1, 2).
		MarginTop(1).
		MarginBottom(1)

	var content strings.Builder
	content.WriteString(titleStyle.Render("🔐 AWS Browser Authentication"))
	content.WriteString("\n\n")
	content.WriteString(instructionStyle.Render("Opening browser for authentication..."))
	content.WriteString("\n")
	content.WriteString(instructionStyle.Render("If the browser doesn't open, visit:"))
	content.WriteString("\n\n")
	content.WriteString(urlStyle.Render(authURL))

	fmt.Fprintf(os.Stderr, "%s\n", boxStyle.Render(content.String()))
}

// displayWebflowDialogPlainText shows the authentication URL in plain text (non-TTY).
func displayWebflowDialogPlainText(authURL string) {
	utils.PrintfMessageToTUI("🔐 **AWS Browser Authentication (Non-Interactive)**\n")
	utils.PrintfMessageToTUI("Visit this URL on a device with a browser:\n")
	utils.PrintfMessageToTUI("%s\n", authURL)
	utils.PrintfMessageToTUI("After signing in, paste the authorization code below.\n")
}

// Spinner model for interactive waiting (follows SSO pattern from sso.go).

// webflowSpinnerTokenResult wraps the token exchange result.
type webflowSpinnerTokenResult struct {
	resp *webflowTokenResponse
	err  error
}

// newWebflowSpinnerModel constructs the bubbletea model for the auth spinner.
func newWebflowSpinnerModel(tokenCh <-chan webflowSpinnerTokenResult, cancel context.CancelFunc) webflowSpinnerModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = theme.GetCurrentStyles().Spinner
	return webflowSpinnerModel{
		spinner: s,
		message: "Waiting for browser authentication",
		tokenCh: tokenCh,
		cancel:  cancel,
	}
}

// webflowSpinnerModel is a bubbletea model for the authentication spinner.
type webflowSpinnerModel struct {
	spinner spinner.Model
	message string
	done    bool
	tokenCh <-chan webflowSpinnerTokenResult
	cancel  context.CancelFunc
	result  *webflowSpinnerTokenResult
}

//nolint:gocritic // Bubbletea framework requires value receivers, not pointer receivers.
func (m webflowSpinnerModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.checkResult())
}

//nolint:gocritic // Bubbletea framework requires value receivers, not pointer receivers.
func (m webflowSpinnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			m.done = true
			m.result = &webflowSpinnerTokenResult{err: errUtils.ErrUserAborted}
			if m.cancel != nil {
				m.cancel()
			}
			return m, tea.Quit
		}
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case webflowSpinnerTokenResult:
		m.done = true
		m.result = &msg
		if m.cancel != nil {
			m.cancel()
		}
		return m, tea.Quit
	}
	return m, nil
}

//nolint:gocritic // Bubbletea framework requires value receivers, not pointer receivers.
func (m webflowSpinnerModel) View() string {
	if m.done {
		if m.result != nil && m.result.err != nil {
			return fmt.Sprintf("%s Authentication failed\n", theme.Styles.XMark)
		}
		return ""
	}
	return fmt.Sprintf("%s %s...", m.spinner.View(), m.message)
}

//nolint:gocritic // Bubbletea framework requires value receivers, not pointer receivers.
func (m webflowSpinnerModel) checkResult() tea.Cmd {
	return func() tea.Msg {
		select {
		case res := <-m.tokenCh:
			return webflowSpinnerTokenResult{resp: res.resp, err: res.err}
		case <-time.After(webflowSpinnerPollInterval):
			return m.checkResult()()
		}
	}
}
