package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/session"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

const (
	doubleNewline = "\n\n"
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

// sessionDeletedMsg is sent when a session has been deleted.
type sessionDeletedMsg struct {
	sessionID string
	err       error
}

// sessionRenamedMsg is sent when a session has been renamed.
type sessionRenamedMsg struct {
	sessionID string
	newName   string
	err       error
}

// sessionListStyles holds the styles for the session list view.
type sessionListStyles struct {
	title    lipgloss.Style
	help     lipgloss.Style
	session  lipgloss.Style
	selected lipgloss.Style
	error    lipgloss.Style
	warning  lipgloss.Style
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

	// Show different help text based on state
	switch {
	case m.deleteConfirm:
		content.WriteString(styles.help.Render("y: Confirm Delete | n/Esc: Cancel"))
	case m.renameMode:
		content.WriteString(styles.help.Render("Enter: Save | Esc: Cancel"))
	default:
		content.WriteString(styles.help.Render("↑/↓: Navigate | Enter: Select | d: Delete | r: Rename | n/Ctrl+N: New | Esc/q: Back | Ctrl+C: Quit"))
	}
	content.WriteString(doubleNewline)

	if m.sessionListError != "" {
		content.WriteString(styles.error.Render(fmt.Sprintf("Error: %s", m.sessionListError)))
		content.WriteString(doubleNewline)
	}

	// Show delete confirmation if active
	if m.deleteConfirm && m.deleteSessionID != "" {
		m.renderDeleteConfirmation(&content, &styles)
		content.WriteString(doubleNewline)
	}

	// Show rename dialog if active
	if m.renameMode && m.renameSessionID != "" {
		m.renderRenameDialog(&content, &styles)
		content.WriteString(doubleNewline)
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
		warning: lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.ColorYellow)).
			Bold(true).
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

// deleteSession deletes the specified session.
func (m *ChatModel) deleteSession(sessionID string) tea.Cmd {
	return func() tea.Msg {
		if m.manager == nil {
			return sessionDeletedMsg{
				sessionID: sessionID,
				err:       errUtils.ErrAISessionManagerNotAvailable,
			}
		}

		ctx := context.Background()
		if err := m.manager.DeleteSession(ctx, sessionID); err != nil {
			return sessionDeletedMsg{
				sessionID: sessionID,
				err:       err,
			}
		}

		return sessionDeletedMsg{
			sessionID: sessionID,
			err:       nil,
		}
	}
}

// handleSessionDeleted processes the session deleted message.
func (m *ChatModel) handleSessionDeleted(msg sessionDeletedMsg) tea.Cmd {
	if msg.err != nil {
		m.sessionListError = fmt.Sprintf("Failed to delete session: %v", msg.err)
		m.deleteConfirm = false
		m.deleteSessionID = ""
		return nil
	}

	// Session deleted successfully
	m.sessionListError = ""
	m.deleteConfirm = false
	m.deleteSessionID = ""

	// If we deleted the current session, clear it
	if m.sess != nil && m.sess.ID == msg.sessionID {
		m.sess = nil
		m.messages = make([]ChatMessage, 0)
		m.updateViewportContent()
	}

	// Reload the session list
	return m.loadSessionList()
}

// renderDeleteConfirmation renders the delete confirmation dialog.
func (m *ChatModel) renderDeleteConfirmation(content *strings.Builder, styles *sessionListStyles) {
	// Find the session name to display
	var sessionName string
	for _, sess := range m.availableSessions {
		if sess.ID == m.deleteSessionID {
			sessionName = sess.Name
			break
		}
	}

	if sessionName == "" {
		sessionName = "Unknown Session"
	}

	warning := fmt.Sprintf("⚠️  Delete session '%s'? This action cannot be undone.", sessionName)
	content.WriteString(styles.warning.Render(warning))
}

// renameSession renames the specified session.
func (m *ChatModel) renameSession(sessionID, newName string) tea.Cmd {
	return func() tea.Msg {
		if m.manager == nil {
			return sessionRenamedMsg{
				sessionID: sessionID,
				newName:   newName,
				err:       errUtils.ErrAISessionManagerNotAvailable,
			}
		}

		ctx := context.Background()
		sess, err := m.manager.GetSession(ctx, sessionID)
		if err != nil {
			return sessionRenamedMsg{
				sessionID: sessionID,
				newName:   newName,
				err:       err,
			}
		}

		// Update the session name
		sess.Name = newName
		if err := m.manager.UpdateSession(ctx, sess); err != nil {
			return sessionRenamedMsg{
				sessionID: sessionID,
				newName:   newName,
				err:       err,
			}
		}

		return sessionRenamedMsg{
			sessionID: sessionID,
			newName:   newName,
			err:       nil,
		}
	}
}

// handleSessionRenamed processes the session renamed message.
func (m *ChatModel) handleSessionRenamed(msg sessionRenamedMsg) tea.Cmd {
	if msg.err != nil {
		m.sessionListError = fmt.Sprintf("Failed to rename session: %v", msg.err)
		m.renameMode = false
		m.renameSessionID = ""
		return nil
	}

	// Session renamed successfully
	m.sessionListError = ""
	m.renameMode = false
	m.renameSessionID = ""

	// If we renamed the current session, update it
	if m.sess != nil && m.sess.ID == msg.sessionID {
		m.sess.Name = msg.newName
	}

	// Reload the session list
	return m.loadSessionList()
}

// renderRenameDialog renders the rename session dialog.
func (m *ChatModel) renderRenameDialog(content *strings.Builder, styles *sessionListStyles) {
	// Find the session name to display
	var sessionName string
	for _, sess := range m.availableSessions {
		if sess.ID == m.renameSessionID {
			sessionName = sess.Name
			break
		}
	}

	if sessionName == "" {
		sessionName = "Unknown Session"
	}

	info := fmt.Sprintf("✏️  Rename session '%s':", sessionName)
	content.WriteString(styles.warning.Render(info))
	content.WriteString("\n")
	content.WriteString(lipgloss.NewStyle().Padding(0, 2).Render(m.renameInput.View()))
}
