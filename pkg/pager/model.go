package pager

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/muesli/ansi"
	"github.com/muesli/reflow/truncate"
	"github.com/muesli/termenv"
)

const (
	minPercent               float64 = 0.0
	maxPercent               float64 = 1.0
	percentToStringMagnitude float64 = 100.0

	statusMessageTimeout = time.Second * 3 // how long to show status messages like "stashed!"
	ellipsis             = "…"
	statusBarHeight      = 1
)

type pagerState int

var nextLine = "\n"

const (
	pagerStateBrowse pagerState = iota
	pagerStateStatusMessage
)

type pagerStatusMessage struct {
	message string
	isError bool
}

var (
	pagerHelpHeight int

	logo = statusBarHelpStyle(" \U0001F47D  ")

	mintGreen = lipgloss.AdaptiveColor{Light: "#89F0CB", Dark: "#89F0CB"}
	darkGreen = lipgloss.AdaptiveColor{Light: "#1C8760", Dark: "#1C8760"}
	errorRed  = lipgloss.AdaptiveColor{Light: "#FF5555", Dark: "#FF5555"}
	darkGray  = lipgloss.AdaptiveColor{Light: "#333333", Dark: "#333333"}

	statusBarNoteFg = lipgloss.AdaptiveColor{Light: "#656565", Dark: "#7D7D7D"}
	statusBarBg     = lipgloss.AdaptiveColor{Light: "#E6E6E6", Dark: "#242424"}

	statusBarNoteStyle = lipgloss.NewStyle().
				Foreground(statusBarNoteFg).
				Background(statusBarBg).
				Render

	statusBarHelpStyle = lipgloss.NewStyle().
				Foreground(statusBarNoteFg).
				Background(lipgloss.AdaptiveColor{Light: "#DCDCDC", Dark: "#323232"}).
				Render

	statusBarMessageStyle = lipgloss.NewStyle().
				Foreground(mintGreen).
				Background(darkGreen).
				Render
	errorMessageStyle = lipgloss.NewStyle().
				Foreground(errorRed).
				Background(darkGray).
				Render

	statusBarMessageScrollPosStyle = lipgloss.NewStyle().
					Foreground(mintGreen).
					Background(darkGreen).
					Render

	statusBarMessageHelpStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("#B6FFE4")).
					Background(green).
					Render

	helpViewStyle = lipgloss.NewStyle().
			Foreground(statusBarNoteFg).
			Background(lipgloss.AdaptiveColor{Light: "#f2f2f2", Dark: "#1B1B1B"}).
			Render

	// Add highlight style for search matches.
	highlightStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#FFFF00")).
			Foreground(lipgloss.Color("#000000")).
			Bold(true).
			Render

	green = lipgloss.Color("#04B575")
)

// Common stuff we'll need to access in all models.
type commonModel struct {
	width  int
	height int
}

var (
	titleStyle = func() lipgloss.Style {
		b := lipgloss.RoundedBorder()
		b.Right = "├"
		return lipgloss.NewStyle().BorderStyle(b).Padding(0, 1)
	}()

	infoStyle = func() lipgloss.Style {
		b := lipgloss.RoundedBorder()
		b.Left = "┤"
		return titleStyle.BorderStyle(b)
	}()
)

type model struct {
	title                string
	content              string
	originalContent      string   // Store original content without highlighting
	originalContentLines []string // Store original content lines for search
	ready                bool
	viewport             viewport.Model
	common               commonModel
	showHelp             bool

	state               pagerState
	statusMessage       pagerStatusMessage
	statusMessageTimer  *time.Timer
	forwardSlashPressed bool
	searchTerm          string
}

func (m *model) Init() tea.Cmd {
	return nil
}

var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

// StripANSI removes ANSI escape sequences (like terminal colors) from input.
func StripANSI(input string) string {
	return ansiRegex.ReplaceAllString(input, "")
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKeyPress(msg)

	case statusMessageTimeoutMsg:
		m.state = pagerStateBrowse
		// Window size is received when starting up and on every resize
	case tea.WindowSizeMsg:
		m.common.width = msg.Width
		m.common.height = msg.Height

		footerHeight := lipgloss.Height(m.footerView())
		verticalMarginHeight := footerHeight

		if !m.ready {
			// Since this program is using the full size of the viewport we
			// need to wait until we've received the window dimensions before
			// we can initialize the viewport. The initial dimensions come in
			// quickly, though asynchronously, which is why we wait for them
			// here.
			m.viewport = viewport.New(msg.Width, msg.Height-verticalMarginHeight)
			m.viewport.YPosition = 0
			// Store original content and set initial content
			if m.originalContent == "" {
				m.originalContent = m.content
				m.originalContentLines = strings.Split(m.content, nextLine)
			}
			m.viewport.SetContent(m.content)
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - verticalMarginHeight
		}
	}

	// Handle keyboard and mouse events in the viewport
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

// highlightSearchTerm applies highlighting to search matches in the content.
func (m *model) highlightSearchTerm() {
	if m.searchTerm == "" {
		m.content = m.originalContent
		m.viewport.SetContent(m.content)
		return
	}

	// Use case-insensitive search
	searchTermLower := strings.ToLower(m.searchTerm)
	var highlighted string

	// Find all matches (case-insensitive)
	lines := m.originalContentLines
	var highlightedLines []string

	for _, line := range lines {
		lineLower := strings.ToLower(line)
		if strings.Contains(lineLower, searchTermLower) {
			// Find the actual case-preserving match
			highlightedLine := line
			startIdx := 0
			for {
				idx := strings.Index(strings.ToLower(highlightedLine[startIdx:]), searchTermLower)
				if idx == -1 {
					break
				}
				actualIdx := startIdx + idx
				// Get the actual text (preserving case)
				actualMatch := highlightedLine[actualIdx : actualIdx+len(m.searchTerm)]
				// Replace with highlighted version
				highlightedLine = highlightedLine[:actualIdx] +
					highlightStyle(actualMatch) +
					highlightedLine[actualIdx+len(m.searchTerm):]
				// Move past this match for next iteration
				startIdx = actualIdx + len(highlightStyle(actualMatch))
			}
			highlightedLines = append(highlightedLines, highlightedLine)
		} else {
			highlightedLines = append(highlightedLines, line)
		}
	}

	highlighted = strings.Join(highlightedLines, nextLine)
	m.content = highlighted
	m.viewport.SetContent(m.content)
}

func (m *model) forwardSlashPressedCase(msg tea.KeyMsg, cmds []tea.Cmd) (tea.Model, tea.Cmd) {
	// Check if the forward slash key is pressed
	switch msg.String() {
	case "enter":
		// Finish search
		m.forwardSlashPressed = false
		if m.searchTerm != "" {
			m.highlightSearchTerm()
			m.scrollToSearchMatch()
		}
	case "backspace", "ctrl+h":
		// Handle backspace in search
		if len(m.searchTerm) > 0 {
			m.searchTerm = m.searchTerm[:len(m.searchTerm)-1]
			if m.searchTerm == "" {
				// Clear highlights if search term is empty
				m.content = m.originalContent
				m.viewport.SetContent(m.content)
			} else {
				m.highlightSearchTerm()
				m.scrollToSearchMatch()
			}
		}

	case "esc", "ctrl+c":
		// Cancel search
		m.forwardSlashPressed = false
		m.searchTerm = ""
		m.content = m.originalContent
		m.viewport.SetContent(m.content)
	default:
		// Add character to search term
		if len(msg.String()) == 1 {
			m.searchTerm += msg.String()
			m.highlightSearchTerm()
			m.scrollToSearchMatch()
		}
	}
	return m, tea.Batch(cmds...)
}

// handleKeyPress processes keyboard input and returns the updated model and command.
func (m *model) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	if m.forwardSlashPressed {
		return m.forwardSlashPressedCase(msg, cmds)
	}

	switch msg.String() {
	case "q", "esc", "ctrl+c":
		if m.searchTerm == "" {
			return m, tea.Quit
		}
		m.cancelSearch()
	case "home", "g":
		m.viewport.GotoTop()
	case "end", "G":
		m.viewport.GotoBottom()
	case "c":
		// Copy using OSC 52 - use original content without highlighting
		termenv.Copy(StripANSI(m.originalContent))
		if err := clipboard.WriteAll(StripANSI(m.originalContent)); err != nil {
			cmds = append(cmds, m.showStatusMessage(pagerStatusMessage{"Failed to copy to clipboard", true}))
		} else {
			cmds = append(cmds, m.showStatusMessage(pagerStatusMessage{"Copied contents", false}))
		}
	case "?":
		m.toggleHelp()
	case "/":
		m.forwardSlashPressed = true
		m.searchTerm = "" // Reset search term when starting new search
	case "n":
		// Find next match
		if m.searchTerm != "" {
			m.scrollToNextSearchMatch()
		}
	case "N":
		// Find previous match
		if m.searchTerm != "" {
			m.scrollToPreviousSearchMatch()
		}
	}

	// Handle keyboard and mouse events in the viewport
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m *model) cancelSearch() {
	m.forwardSlashPressed = false
	m.searchTerm = ""
	m.content = m.originalContent // Reset content to original without highlights
	m.viewport.SetContent(m.content)
}

func (m *model) scrollToSearchMatch() {
	if m.searchTerm == "" {
		return
	}

	lines := m.originalContentLines
	searchTermLower := strings.ToLower(m.searchTerm)

	for i, line := range lines {
		if strings.Contains(strings.ToLower(line), searchTermLower) { // Found the match. Scroll to that line.
			m.viewport.SetYOffset(i - 1)
			break
		}
	}
}

func (m *model) scrollToNextSearchMatch() {
	if m.searchTerm == "" {
		return
	}

	lines := m.originalContentLines
	searchTermLower := strings.ToLower(m.searchTerm)
	currentLine := m.viewport.YOffset

	// Look for matches after current position
	for i := currentLine + 1; i < len(lines); i++ {
		if strings.Contains(strings.ToLower(lines[i]), searchTermLower) {
			m.viewport.SetYOffset(i)
			return
		}
	}

	// If no match found after current position, wrap to beginning
	for i := 0; i <= currentLine; i++ {
		if strings.Contains(strings.ToLower(lines[i]), searchTermLower) {
			m.viewport.SetYOffset(i)
			return
		}
	}
}

func (m *model) scrollToPreviousSearchMatch() {
	if m.searchTerm == "" {
		return
	}

	lines := m.originalContentLines
	searchTermLower := strings.ToLower(m.searchTerm)
	currentLine := m.viewport.YOffset

	// Look for matches before current position (in reverse)
	for i := currentLine - 1; i >= 0; i-- {
		if strings.Contains(strings.ToLower(lines[i]), searchTermLower) {
			m.viewport.SetYOffset(i)
			return
		}
	}

	// If no match found before current position, wrap to end
	for i := len(lines) - 1; i >= currentLine; i-- {
		if strings.Contains(strings.ToLower(lines[i]), searchTermLower) {
			m.viewport.SetYOffset(i)
			return
		}
	}
}

type statusMessageTimeoutMsg applicationContext

// applicationContext indicates the area of the application something applies
// to. Occasionally used as an argument to commands and messages.
type applicationContext int

const (
	stashContext applicationContext = iota
	pagerContext
)

// Perform stuff that needs to happen after a successful markdown stash. Note
// that the the returned command should be sent back the through the pager
// update function.
func (m *model) showStatusMessage(msg pagerStatusMessage) tea.Cmd {
	// Show a success message to the user
	m.state = pagerStateStatusMessage
	m.statusMessage = msg

	if m.statusMessageTimer != nil {
		m.statusMessageTimer.Stop()
	}

	m.statusMessageTimer = time.NewTimer(statusMessageTimeout)

	return waitForStatusMessageTimeout(pagerContext, m.statusMessageTimer)
}

func waitForStatusMessageTimeout(appCtx applicationContext, t *time.Timer) tea.Cmd {
	return func() tea.Msg {
		<-t.C
		return statusMessageTimeoutMsg(appCtx)
	}
}

func (m *model) toggleHelp() {
	m.showHelp = !m.showHelp
	m.setSize(m.common.width, m.common.height)
	if m.viewport.PastBottom() {
		m.viewport.GotoBottom()
	}
}

func (m *model) setSize(w, h int) {
	m.viewport.Width = w
	m.viewport.Height = h - statusBarHeight

	if m.showHelp {
		if pagerHelpHeight == 0 {
			pagerHelpHeight = strings.Count(m.helpView(), nextLine)
		}
		m.viewport.Height -= (statusBarHeight + pagerHelpHeight)
	}
}

func (m *model) View() string {
	if !m.ready {
		return "\n  Initializing..."
	}

	view := fmt.Sprintf("%s\n%s", m.viewport.View(), m.footerView())

	// Show search prompt if in search mode
	if m.forwardSlashPressed {
		searchPrompt := fmt.Sprintf("Search: %s", m.searchTerm)
		view += nextLine + statusBarMessageStyle(" "+searchPrompt+" ")
	}

	return view
}

func (m *model) helpView() (s string) {
	col1 := []string{
		"g/home  go to top",
		"G/end   go to bottom",
		"c       copy contents",
		"/       search",
		"n       next match",
		"N       prev match",
		"q       quit",
	}
	i := 0
	s += nextLine
	s += "k/↑      up                  " + col1[i] + nextLine
	i += 1
	s += "j/↓      down                " + col1[i] + nextLine
	i += 1
	s += "b/pgup   page up             " + col1[i] + nextLine
	i += 1
	s += "f/pgdn   page down           " + col1[i] + nextLine
	i += 1
	s += "u        ½ page up           " + col1[i] + nextLine
	i += 1
	s += "d        ½ page down         " + col1[i] + nextLine
	i += 1
	s += "esc      back to files       " + col1[i] + nextLine
	i += 1
	s += "                             " + col1[i]

	s = indent(s, 2)

	// Fill up empty cells with spaces for background coloring
	if m.common.width > 0 {
		lines := strings.Split(s, nextLine)
		for i := 0; i < len(lines); i++ {
			l := runewidth.StringWidth(lines[i])
			n := max(m.common.width-l, 0)
			lines[i] += strings.Repeat(" ", n)
		}

		s = strings.Join(lines, nextLine)
	}

	return helpViewStyle(s)
}

func (m *model) footerView() string {
	b := &strings.Builder{}
	m.statusBarView(b)
	return b.String()
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (m *model) statusBarView(b *strings.Builder) {
	showStatusMessage := m.state == pagerStateStatusMessage

	// Scroll percent
	percent := math.Max(minPercent, math.Min(maxPercent, m.viewport.ScrollPercent()))
	scrollPercent := fmt.Sprintf(" %3.f%% ", percent*percentToStringMagnitude)
	var note, helpNote string
	switch {
	case showStatusMessage && m.statusMessage.isError:
		note = errorMessageStyle(" " + m.statusMessage.message + " ")
		helpNote = errorMessageStyle(" ? Help ")
		scrollPercent = errorMessageStyle(scrollPercent)
	case showStatusMessage:
		scrollPercent = statusBarMessageScrollPosStyle(scrollPercent)
		note = statusBarMessageStyle(" " + m.statusMessage.message + " ")
		helpNote = statusBarMessageHelpStyle(" ? Help ")
	default:
		scrollPercent = statusBarNoteStyle(scrollPercent)
		titleText := m.title
		if m.searchTerm != "" && !m.forwardSlashPressed {
			titleText += fmt.Sprintf(" (searching: %s)", m.searchTerm)
		}
		note = statusBarNoteStyle(" " + titleText + " ")
		helpNote = statusBarHelpStyle(" ? Help ")
	}

	note = truncate.StringWithTail(note, uint(max(0, //nolint:gosec
		m.common.width-
			ansi.PrintableRuneWidth(logo)-
			ansi.PrintableRuneWidth(scrollPercent),
	)), ellipsis)

	// Empty space
	padding := max(0,
		m.common.width-
			ansi.PrintableRuneWidth(logo)-
			ansi.PrintableRuneWidth(note)-
			ansi.PrintableRuneWidth(scrollPercent)-
			ansi.PrintableRuneWidth(helpNote),
	)
	emptySpace := strings.Repeat(" ", padding)
	switch {
	case showStatusMessage && m.statusMessage.isError:
		emptySpace = errorMessageStyle(emptySpace)
	case showStatusMessage:
		emptySpace = statusBarMessageStyle(emptySpace)
	default:
		emptySpace = statusBarNoteStyle(emptySpace)
	}

	fmt.Fprintf(b, "%s%s%s%s%s",
		logo,
		note,
		emptySpace,
		scrollPercent,
		helpNote,
	)
	if m.showHelp {
		fmt.Fprint(b, nextLine+m.helpView())
	}
}

// Lightweight version of reflow's indent function.
func indent(s string, n int) string {
	if n <= 0 || s == "" {
		return s
	}
	l := strings.Split(s, nextLine)
	b := strings.Builder{}
	i := strings.Repeat(" ", n)
	for _, v := range l {
		fmt.Fprintf(&b, "%s%s\n", i, v)
	}
	return b.String()
}
