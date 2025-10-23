package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/cloudposse/atmos/pkg/ai/session"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

// sessionListLoadedMsg is sent when the session list has been loaded.
type sessionListLoadedMsg struct {
	sessions []*session.Session
	err      error
}

// sessionSwitchedMsg is sent when a session switch is complete.
type sessionSwitchedMsg struct {
	session  *session.Session
	messages []*session.Message
	err      error
}

// sessionListStyles holds the styles for the session list view.
type sessionListStyles struct {
	title    lipgloss.Style
	help     lipgloss.Style
	session  lipgloss.Style
	selected lipgloss.Style
	error    lipgloss.Style
}

// handleSessionListLoaded processes the session list loaded message.
func (m *ChatModel) handleSessionListLoaded(msg sessionListLoadedMsg) {
	if msg.err != nil {
		m.sessionListError = msg.err.Error()
	} else {
		m.availableSessions = msg.sessions
		m.selectedSessionIndex = 0
		m.sessionListError = ""
		m.currentView = viewModeSessionList
	}
}

// handleSessionSwitched processes the session switched message.
func (m *ChatModel) handleSessionSwitched(msg sessionSwitchedMsg) {
	if msg.err != nil {
		m.sessionListError = msg.err.Error()
	} else {
		m.sess = msg.session
		m.messages = make([]ChatMessage, 0)
		// Convert session messages to chat messages.
		for _, sessionMsg := range msg.messages {
			m.messages = append(m.messages, ChatMessage{
				Role:    sessionMsg.Role,
				Content: sessionMsg.Content,
				Time:    sessionMsg.CreatedAt,
			})
		}
		m.updateViewportContent()
		m.currentView = viewModeChat
		m.sessionListError = ""
	}
}

// sessionListView renders the session list interface.
func (m *ChatModel) sessionListView() string {
	styles := m.sessionListStyles()
	var content strings.Builder

	content.WriteString(styles.title.Render("Session List"))
	content.WriteString("\n")
	content.WriteString(styles.help.Render("↑/↓: Navigate | Enter: Select | Esc/q: Back | Ctrl+C: Quit"))
	content.WriteString("\n\n")

	if m.sessionListError != "" {
		content.WriteString(styles.error.Render(fmt.Sprintf("Error: %s", m.sessionListError)))
		content.WriteString("\n\n")
	}

	if len(m.availableSessions) == 0 {
		content.WriteString(styles.session.Render("No sessions available"))
	} else {
		m.renderSessionList(&content, &styles)
	}

	return content.String()
}

// sessionListStyles creates the styles for the session list view.
func (m *ChatModel) sessionListStyles() sessionListStyles {
	return sessionListStyles{
		title: lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.ColorCyan)).
			Bold(true).
			Padding(1, 2),
		help: lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Padding(0, 2),
		session: lipgloss.NewStyle().
			Padding(0, 2),
		selected: lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.ColorGreen)).
			Bold(true).
			Padding(0, 2),
		error: lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.ColorRed)).
			Padding(0, 2),
	}
}

// renderSessionList renders the list of sessions.
func (m *ChatModel) renderSessionList(content *strings.Builder, styles *sessionListStyles) {
	for i, sess := range m.availableSessions {
		prefix := "  "
		style := styles.session
		if i == m.selectedSessionIndex {
			prefix = "→ "
			style = styles.selected
		}

		msgCount := m.getSessionMessageCount(sess.ID)
		sessionInfo := fmt.Sprintf("%s%s (%s, %d messages)",
			prefix,
			sess.Name,
			sess.CreatedAt.Format("Jan 02, 15:04"),
			msgCount)

		content.WriteString(style.Render(sessionInfo))
		content.WriteString("\n")
	}
}

// getSessionMessageCount returns the message count for a session.
func (m *ChatModel) getSessionMessageCount(sessionID string) int {
	if m.manager == nil {
		return 0
	}

	ctx := context.Background()
	count, _ := m.manager.GetMessageCount(ctx, sessionID)
	return count
}
