package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"

	"github.com/cloudposse/atmos/pkg/ai"
	aiTypes "github.com/cloudposse/atmos/pkg/ai/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

func (m *ChatModel) updateViewportContent() {
	// PERFORMANCE OPTIMIZATION: Only render new messages, reuse cached renders.
	// This dramatically improves performance with many messages.

	// Calculate how many messages need rendering.
	numCached := len(m.renderedMessages)
	numTotal := len(m.messages)

	// If cache is empty or invalid, render all messages.
	if numCached == 0 || numCached > numTotal {
		m.renderedMessages = make([]string, 0, numTotal*3) // Pre-allocate: header + content + empty line per message.
		numCached = 0
	}

	// Render only new messages (from numCached to numTotal).
	for i := numCached; i < numTotal; i++ {
		m.renderAndCacheMessage(m.messages[i])
	}

	// Build final content from cache with empty line at top.
	finalContent := append([]string{""}, m.renderedMessages...)
	m.viewport.SetContent(strings.Join(finalContent, newlineChar))
	m.viewport.GotoBottom()
}

// renderAndCacheMessage renders a single message and appends it to the rendered cache.
func (m *ChatModel) renderAndCacheMessage(msg ChatMessage) {
	header := m.buildMessageHeader(msg)
	renderedContent := m.renderMessageContent(msg)

	// Cache the rendered message parts.
	m.renderedMessages = append(m.renderedMessages, header)
	m.renderedMessages = append(m.renderedMessages, renderedContent)
	m.renderedMessages = append(m.renderedMessages, "") // Empty line between messages.
}

// buildMessageHeader creates the styled header line for a message (role + timestamp).
func (m *ChatModel) buildMessageHeader(msg ChatMessage) string {
	var style lipgloss.Style
	var prefix string

	switch msg.Role {
	case roleUser:
		style = lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.ColorGreen)).
			Bold(true)
		prefix = "You:"
	case roleAssistant:
		style = lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.ColorCyan))
		prefix = m.buildAssistantPrefix(msg)
	case roleSystem:
		style = lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.ColorRed)).
			Italic(true)
		prefix = "System:"
	}

	timestamp := msg.Time.Format("15:04")
	timeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	return fmt.Sprintf("%s %s", style.Render(prefix), timeStyle.Render(timestamp))
}

// buildAssistantPrefix builds the prefix for assistant messages including provider and skill info.
func (m *ChatModel) buildAssistantPrefix(msg ChatMessage) string {
	provider := msg.Provider
	if provider == "" {
		provider = "unknown"
	}
	skillIcon := ""
	if m.currentSkill != nil {
		skillIcon = " " + getSkillIcon(m.currentSkill.Name)
	}
	return fmt.Sprintf("Atmos AI \U0001f47d (%s)%s:", provider, skillIcon)
}

// renderMessageContent renders the content of a message, applying markdown for assistant messages.
func (m *ChatModel) renderMessageContent(msg ChatMessage) string {
	if msg.Role == roleAssistant {
		return m.renderMarkdown(msg.Content)
	}
	// Plain text for user and system messages.
	contentStyle := lipgloss.NewStyle().
		PaddingLeft(2).
		Width(m.viewport.Width - 4)
	return contentStyle.Render(msg.Content)
}

// renderMarkdown renders markdown content with syntax highlighting using the cached glamour renderer.
// PERFORMANCE: Uses cached renderer instead of creating new one each time.
// Tables are detected and rendered using lipgloss.Table for better formatting.
func (m *ChatModel) renderMarkdown(content string) string {
	// Detect and extract markdown tables for special rendering.
	if hasMarkdownTable(content) {
		return m.renderMarkdownWithTables(content)
	}

	// Fallback to plain text if no cached renderer available.
	if m.markdownRenderer == nil {
		return lipgloss.NewStyle().
			PaddingLeft(2).
			Width(m.viewport.Width - 4).
			Render(content)
	}

	// Use cached renderer for performance.
	rendered, err := m.markdownRenderer.Render(content)
	if err != nil {
		// Log the error and content length for debugging.
		log.Debugf("Failed to render markdown (content length: %d): %v", len(content), err)
		// Fallback to plain text if rendering fails.
		return lipgloss.NewStyle().
			PaddingLeft(2).
			Width(m.viewport.Width - 4).
			Render(content)
	}

	return padRenderedLines(rendered)
}

// padRenderedLines adds left padding to rendered markdown lines to match other messages.
func padRenderedLines(rendered string) string {
	paddedLines := make([]string, 0)
	for _, line := range strings.Split(rendered, newlineChar) {
		paddedLines = append(paddedLines, "  "+line)
	}

	return strings.TrimRight(strings.Join(paddedLines, newlineChar), newlineChar)
}

// hasMarkdownTable detects if content contains a markdown table.
func hasMarkdownTable(content string) bool {
	lines := strings.Split(content, newlineChar)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Look for separator line like |---|---|---|.
		if strings.HasPrefix(trimmed, pipeChar) && strings.Contains(trimmed, "---") {
			return true
		}
	}
	return false
}

// renderMarkdownWithTables renders markdown content with special handling for tables.
func (m *ChatModel) renderMarkdownWithTables(content string) string {
	lines := strings.Split(content, newlineChar)
	var result strings.Builder
	var tableLines []string
	inTable := false

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// Check if this is a table line.
		isTableLine := strings.HasPrefix(trimmed, pipeChar) && strings.Contains(trimmed, pipeChar)

		if isTableLine {
			if !inTable {
				inTable = true
				tableLines = []string{}
			}
			tableLines = append(tableLines, line)
			continue
		}

		// End of table - render it.
		if inTable {
			result.WriteString(m.renderTable(tableLines))
			result.WriteString(newlineChar)
			inTable = false
			tableLines = nil
		}

		// Render non-table content.
		m.renderNonTableLine(&result, trimmed, line)
	}

	// Handle table at end of content.
	if inTable && len(tableLines) > 0 {
		result.WriteString(m.renderTable(tableLines))
	}

	return strings.TrimRight(result.String(), newlineChar)
}

// renderNonTableLine renders a single non-table line using glamour or plain text.
func (m *ChatModel) renderNonTableLine(result *strings.Builder, trimmed, line string) {
	if trimmed == "" {
		result.WriteString(newlineChar)
		return
	}

	if m.markdownRenderer == nil {
		result.WriteString("  " + line + newlineChar)
		return
	}

	rendered, err := m.markdownRenderer.Render(line)
	if err != nil {
		result.WriteString("  " + line + newlineChar)
		return
	}
	result.WriteString("  " + strings.TrimSpace(rendered) + newlineChar)
}

// renderTable renders a markdown table using lipgloss.Table for better formatting.
func (m *ChatModel) renderTable(lines []string) string {
	if len(lines) < 2 {
		// Not a valid table.
		return strings.Join(lines, newlineChar)
	}

	headers, rows := parseTableStructure(lines)

	// Create lipgloss table.
	t := table.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("240"))).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == 0 {
				// Header style.
				return lipgloss.NewStyle().
					Foreground(lipgloss.Color(theme.ColorCyan)).
					Bold(true).
					Padding(0, 1)
			}
			// Data cell style.
			return lipgloss.NewStyle().Padding(0, 1)
		})

	// Set headers.
	if len(headers) > 0 {
		t.Headers(headers...)
	}

	// Add rows.
	for _, row := range rows {
		t.Row(row...)
	}

	// Render and add left padding.
	return padRenderedLines(t.Render())
}

// parseTableStructure parses markdown table lines into headers and data rows.
func parseTableStructure(lines []string) ([]string, [][]string) {
	var headers []string
	var rows [][]string

	for i, line := range lines {
		cells := parseTableCells(line)
		if len(cells) == 0 {
			continue
		}

		switch i {
		case 0:
			headers = cells
		case 1:
			// Separator row - skip.
			continue
		default:
			rows = append(rows, cells)
		}
	}

	return headers, rows
}

// parseTableCells splits a pipe-delimited table line into cells.
func parseTableCells(line string) []string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return nil
	}

	parts := strings.Split(trimmed, pipeChar)
	var cells []string
	for _, part := range parts {
		cell := strings.TrimSpace(part)
		if cell != "" && cell != "---" && !strings.Contains(cell, "---") {
			cells = append(cells, cell)
		}
	}
	return cells
}

// Custom message types.
type sendMessageMsg string

type aiResponseMsg struct {
	content string
	usage   *aiTypes.Usage
}

type aiErrorMsg string

type compactStatusMsg struct {
	stage        string // "starting", "completed", "failed".
	messageCount int
	savings      int
	err          error
}

type providerSwitchedMsg struct {
	provider       string
	providerConfig *schema.AIProviderConfig
	newClient      ai.Client
	err            error
}

// turnStepKind distinguishes an AI network round-trip from a tool execution within a turn.
type turnStepKind int

const (
	turnStepKindAICall turnStepKind = iota
	turnStepKindTool
)

// turnStepStatus is the lifecycle stage of a turnStep.
type turnStepStatus int

const (
	turnStepRunning turnStepStatus = iota
	turnStepDone
	turnStepError
)

// turnStep is one unit of visible work within the current AI turn (one AI call or one tool
// execution), used to render a running checklist in the footer while m.isLoading.
type turnStep struct {
	kind      turnStepKind
	label     string
	status    turnStepStatus
	err       error
	startedAt time.Time
	duration  time.Duration
}

// turnStepStartedMsg signals a new step has begun. Sent via m.program.Send from the same
// goroutine driving the (blocking) AI/tool call, exactly like statusMsg.
type turnStepStartedMsg turnStep

// turnStepFinishedMsg marks the most recently started step complete. Steps run strictly
// sequentially within a turn (no concurrent AI calls or tool executions), so this always
// applies to the last entry in ChatModel.turnSteps.
type turnStepFinishedMsg struct {
	err error
}

// maxDisplayedTurnSteps bounds the footer's step log so it stays inside the fixed footer
// content budget (9 lines: textarea 7 + newline 1 + help 1) reserved by
// calculateViewportHeight, since the chat TUI runs in alt-screen mode where content taller
// than the terminal is clipped rather than scrolled.
const maxDisplayedTurnSteps = 6

// renderTurnSteps renders the turn's step log: a checkmark/xmark line per completed step
// and the live spinner + elapsed time on the in-flight one, so tool execution history stays
// visible for the rest of the turn instead of being silently overwritten.
func (m *ChatModel) renderTurnSteps(usageStr, cancelHint string) string {
	start := 0
	var lines []string
	if len(m.turnSteps) > maxDisplayedTurnSteps {
		start = len(m.turnSteps) - maxDisplayedTurnSteps
		mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Italic(true)
		lines = append(lines, mutedStyle.Render(fmt.Sprintf("… %d earlier step(s) omitted", start)))
	}
	for i := start; i < len(m.turnSteps); i++ {
		lines = append(lines, m.renderTurnStepLine(&m.turnSteps[i], usageStr, cancelHint))
	}
	return strings.Join(lines, newlineChar)
}

// renderTurnStepLine renders a single turn step: a checkmark/xmark for a finished step, or
// the live spinner and elapsed time for the in-flight one.
func (m *ChatModel) renderTurnStepLine(step *turnStep, usageStr, cancelHint string) string {
	styles := theme.GetCurrentStyles()
	switch step.status {
	case turnStepDone:
		return fmt.Sprintf("%s %s %s", styles.Checkmark, step.label, mutedElapsed(step.duration))
	case turnStepError:
		errSuffix := ""
		if step.err != nil {
			errSuffix = " — " + step.err.Error()
		}
		return fmt.Sprintf("%s %s%s", styles.XMark, step.label, errSuffix)
	default: // turnStepRunning.
		return fmt.Sprintf("%s %s %s%s %s", m.spinner.View(), step.label, mutedElapsed(time.Since(step.startedAt)), usageStr, cancelHint)
	}
}

// mutedElapsed formats a duration as "(1.2s)" for display next to a step, or "" if under a second.
func mutedElapsed(d time.Duration) string {
	if d < time.Second {
		return ""
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(fmt.Sprintf("(%.1fs)", d.Seconds()))
}
