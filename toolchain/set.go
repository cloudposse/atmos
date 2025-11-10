package toolchain

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
)

type versionItem struct {
	version      string
	title        string
	desc         string
	releaseNotes string
}

func (i versionItem) Title() string {
	defer perf.Track(nil, "toolchain.versionItem.Title")()

	return i.version
}

func (i versionItem) Description() string {
	defer perf.Track(nil, "toolchain.versionItem.Description")()

	return i.title
}

func (i versionItem) FilterValue() string {
	defer perf.Track(nil, "toolchain.versionItem.FilterValue")()

	return i.version
}

type versionListModel struct {
	list             list.Model
	viewport         viewport.Model
	selected         string
	err              error
	owner            string
	repo             string
	items            []versionItem
	title            string
	currentItemIndex int    // Track the currently displayed item to avoid unnecessary updates
	focused          string // Track which pane has focus: "list" or "viewport"
	scrollSpeed      int    // Scroll speed multiplier (1 = normal, 2 = fast, 3 = very fast, etc.)
}

func (m versionListModel) Init() tea.Cmd {
	defer perf.Track(nil, "toolchain.versionListModel.Init")()

	return nil
}

func (m versionListModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	defer perf.Track(nil, "toolchain.versionListModel.Update")()

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch keypress := msg.String(); keypress {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "tab":
			// Toggle focus between list and viewport
			if m.focused == "list" {
				m.focused = "viewport"
			} else {
				m.focused = "list"
			}
			return m, nil
		case "enter":
			// Enter should work regardless of focus - select the current item
			if m.list.SelectedItem() != nil {
				selectedItem := m.list.SelectedItem().(versionItem)
				m.selected = selectedItem.version
				return m, tea.Quit
			}
		}
	case tea.WindowSizeMsg:
		// Calculate split layout - make version column narrower
		leftWidth := msg.Width / 4              // Version column takes 1/4 of width (smaller)
		rightWidth := msg.Width - leftWidth - 2 // Account for separator only

		// Calculate height accounting for page title and borders - ensure it fits on screen
		contentHeight := msg.Height - 6 // Subtract more for title, borders, and status bar

		// Update list size with more height to show more items, but ensure it fits
		m.list.SetSize(leftWidth-2, contentHeight)

		// Update viewport size to use full available width and fit on screen
		m.viewport.Width = rightWidth - 2 // Account for border
		m.viewport.Height = contentHeight

		// Update release notes for currently selected item
		if m.list.SelectedItem() != nil {
			selectedItem := m.list.SelectedItem().(versionItem)
			currentIndex := m.list.Index()

			// Only update content if the selection actually changed
			if currentIndex != m.currentItemIndex {
				renderedNotes := renderMarkdown(selectedItem.releaseNotes, m.viewport.Width)
				// Set content without width styling to use full available width
				m.viewport.SetContent(renderedNotes)
				m.currentItemIndex = currentIndex
			}
		}
	}

	var cmd tea.Cmd
	// Update list only if it has focus, or for window size changes
	if m.focused == "list" {
		m.list, cmd = m.list.Update(msg)
	} else {
		// Still handle window size for layout, but don't process navigation keys
		m.list, cmd = m.list.Update(tea.WindowSizeMsg{})
	}

	// Update viewport when list selection changes (regardless of focus)
	if m.list.SelectedItem() != nil {
		selectedItem := m.list.SelectedItem().(versionItem)
		currentIndex := m.list.Index()

		// Only update content if the selection actually changed
		if currentIndex != m.currentItemIndex {
			renderedNotes := renderMarkdown(selectedItem.releaseNotes, m.viewport.Width)
			// Set content without width styling to use full available width
			m.viewport.SetContent(renderedNotes)
			m.currentItemIndex = currentIndex
		}
	}

	// Handle viewport scrolling - only if viewport has focus
	var vpCmd tea.Cmd
	if m.focused == "viewport" {
		// Apply custom scroll speed for arrow keys and page up/down
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			switch keyMsg.String() {
			case "up", "k":
				// Scroll up by scroll speed amount
				for i := 0; i < m.scrollSpeed; i++ {
					m.viewport.ScrollUp(1)
				}
				return m, nil
			case "down", "j":
				// Scroll down by scroll speed amount
				for i := 0; i < m.scrollSpeed; i++ {
					m.viewport.ScrollDown(1)
				}
				return m, nil
			case "pgup":
				// Page up by scroll speed amount
				for i := 0; i < m.scrollSpeed; i++ {
					m.viewport.PageUp()
				}
				return m, nil
			case "pgdown":
				// Page down by scroll speed amount
				for i := 0; i < m.scrollSpeed; i++ {
					m.viewport.PageDown()
				}
				return m, nil
			case "home", "g":
				m.viewport.GotoTop()
				return m, nil
			case "end", "G":
				m.viewport.GotoBottom()
				return m, nil
			}
		}
		// For mouse wheel and other events, use normal viewport behavior
		m.viewport, vpCmd = m.viewport.Update(msg)
	} else {
		m.viewport, vpCmd = m.viewport.Update(tea.WindowSizeMsg{}) // Still handle window size for layout
	}

	return m, tea.Batch(cmd, vpCmd)
}

func (m versionListModel) View() string {
	defer perf.Track(nil, "toolchain.versionListModel.View")()

	if m.err != nil {
		return fmt.Sprintf("Error: %v\n", m.err)
	}

	// Create title
	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("62")).
		Bold(true).
		MarginBottom(1)
	title := titleStyle.Render(m.title)

	// Create split layout
	leftPane := m.list.View()
	rightPane := m.viewport.View()

	// Add border styling with focus indication - just change color, no thick borders
	var leftStyle, rightStyle lipgloss.Style
	if m.focused == "list" {
		leftStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("62"))
		rightStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240"))
	} else {
		leftStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240"))
		rightStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("62"))
	}

	leftPane = leftStyle.Render(leftPane)
	rightPane = rightStyle.Render(rightPane)

	// Combine panes with a separator
	content := lipgloss.JoinHorizontal(lipgloss.Left, leftPane, "  ", rightPane)

	// Add hint about tab navigation and scroll speed
	hintStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Italic(true).
		MarginTop(1)

	scrollSpeedHint := ""
	if m.scrollSpeed > 1 {
		scrollSpeedHint = fmt.Sprintf(" • Scroll speed: %dx", m.scrollSpeed)
	}

	hint := hintStyle.Render(fmt.Sprintf("Press Tab to switch between panes • Press Enter to select • Press q to quit%s", scrollSpeedHint))

	// Combine title, content, and hint
	return lipgloss.JoinVertical(lipgloss.Left, title, content, hint)
}

// SetToolVersion handles the core logic of setting a tool version.
// If version is empty, it will prompt the user interactively (for GitHub release type tools).
func SetToolVersion(toolName, version string, scrollSpeed int) error {
	defer perf.Track(nil, "toolchain.SetToolVersion")()

	// Resolve the tool name to handle aliases
	installer := NewInstaller()
	owner, repo, err := installer.parseToolSpec(toolName)
	if err != nil {
		return fmt.Errorf("invalid tool name: %w", err)
	}
	resolvedKey := owner + "/" + repo

	// If no version provided, fetch available versions and show interactive selection
	if version == "" {
		tool, err := installer.findTool(owner, repo, "")
		if err != nil {
			return fmt.Errorf("failed to find tool configuration: %w", err)
		}

		if tool.Type != "github_release" {
			return fmt.Errorf("%w: interactive version selection is only available for GitHub release type tools", ErrInvalidToolSpec)
		}

		items, err := fetchGitHubVersions(owner, repo)
		if err != nil {
			return fmt.Errorf("failed to fetch versions from GitHub: %w", err)
		}
		if len(items) == 0 {
			return fmt.Errorf("%w: no versions found for %s/%s", ErrNoVersionsFound, owner, repo)
		}

		listItems := make([]list.Item, len(items))
		for i, item := range items {
			listItems[i] = item
		}

		l := list.New(listItems, newCustomDelegate(), 0, 0)
		l.Title = "Select Version"
		l.SetShowHelp(false)
		l.SetShowStatusBar(true)
		l.InfiniteScrolling = true

		vp := viewport.New(0, 0)
		vp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("62"))

		if scrollSpeed < 1 {
			scrollSpeed = 3
		}

		m := versionListModel{
			list:        l,
			viewport:    vp,
			owner:       owner,
			repo:        repo,
			items:       items,
			title:       fmt.Sprintf("Select version for %s/%s", owner, repo),
			focused:     "list",
			scrollSpeed: scrollSpeed,
		}

		if len(items) > 0 {
			renderedNotes := renderMarkdown(items[0].releaseNotes, m.viewport.Width)
			m.viewport.SetContent(renderedNotes)
			m.currentItemIndex = 0
		}

		p := tea.NewProgram(m)
		finalModel, err := p.Run()
		if err != nil {
			return fmt.Errorf("failed to run interactive selection: %w", err)
		}

		finalVersionModel := finalModel.(versionListModel)
		if finalVersionModel.selected == "" {
			return fmt.Errorf("%w: no version selected", ErrInvalidToolSpec)
		}

		version = finalVersionModel.selected
	}

	// Add the tool with the selected version
	err = AddToolToVersions(atmosConfig.Toolchain.VersionsFile, resolvedKey, version)
	if err != nil {
		return fmt.Errorf("failed to set version: %w", err)
	}

	return ui.Successf("Set %s@%s in %s", resolvedKey, version, atmosConfig.Toolchain.VersionsFile)
}

// fetchGitHubVersions fetches available versions and titles from GitHub releases.
func fetchGitHubVersions(owner, repo string) ([]versionItem, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases?per_page=100", owner, repo)

	resp, err := makeGitHubRequest(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: GitHub API returned status %d", ErrHTTPRequest, resp.StatusCode)
	}

	releases, err := parseReleases(resp.Body)
	if err != nil {
		return nil, err
	}

	items := transformReleases(releases)
	if len(items) == 0 {
		return nil, fmt.Errorf("%w: no non-prerelease versions found for %s/%s", ErrNoVersionsFound, owner, repo)
	}

	return items, nil
}

func makeGitHubRequest(apiURL string) (*http.Response, error) {
	token := viper.GetString("github-token")
	client := &http.Client{}
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch releases from GitHub: %w", err)
	}
	return resp, nil
}

type release struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	Body        string `json:"body"`
	Prerelease  bool   `json:"prerelease"`
	PublishedAt string `json:"published_at"`
}

func parseReleases(r io.Reader) ([]release, error) {
	var releases []release
	if err := json.NewDecoder(r).Decode(&releases); err != nil {
		return nil, fmt.Errorf("failed to parse releases JSON: %w", err)
	}
	return releases, nil
}

func transformReleases(releases []release) []versionItem {
	var items []versionItem
	for _, r := range releases {
		if r.Prerelease {
			continue
		}
		version := strings.TrimPrefix(r.TagName, versionPrefix)
		title := r.Name
		if title == "" {
			title = r.TagName
		}
		notes := formatReleaseNotes(r.Name, r.TagName, r.Body, r.PublishedAt)
		items = append(items, versionItem{
			version:      version,
			title:        title,
			desc:         fmt.Sprintf("Version %s", version),
			releaseNotes: notes,
		})
	}
	return items
}

// formatReleaseNotes formats the release notes for display.
func formatReleaseNotes(name, tagName, body, publishedAt string) string {
	var result strings.Builder

	// Title
	if name != "" && name != tagName {
		result.WriteString(fmt.Sprintf("# %s\n\n", name))
	} else {
		result.WriteString(fmt.Sprintf("# %s\n\n", tagName))
	}

	// Published date
	if publishedAt != "" {
		result.WriteString(fmt.Sprintf("**Published:** %s\n\n", publishedAt))
	}

	// Release notes body
	if body != "" {
		result.WriteString(body)
	} else {
		result.WriteString("No release notes available.")
	}

	return result.String()
}

// renderMarkdown renders markdown content using glamour.
func renderMarkdown(content string, width int) string {
	// Create a glamour renderer with dracula theme but override borders
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dracula"), // Use dracula theme for better contrast
		glamour.WithWordWrap(width),          // Use the actual column width for word wrap
		glamour.WithColorProfile(termenv.ColorProfile()),
		glamour.WithEmoji(),
	)
	if err != nil {
		// If glamour fails, return the raw content
		return content
	}
	defer r.Close()

	output, err := r.Render(content)
	if err != nil {
		// If rendering fails, return the raw content
		return content
	}

	// Remove any border characters from the rendered output
	// This removes box-drawing characters that might create borders
	output = strings.ReplaceAll(output, "┌", "")
	output = strings.ReplaceAll(output, "┐", "")
	output = strings.ReplaceAll(output, "└", "")
	output = strings.ReplaceAll(output, "┘", "")
	output = strings.ReplaceAll(output, "─", "")
	output = strings.ReplaceAll(output, "│", "")
	output = strings.ReplaceAll(output, "├", "")
	output = strings.ReplaceAll(output, "┤", "")
	output = strings.ReplaceAll(output, "┬", "")
	output = strings.ReplaceAll(output, "┴", "")

	return output
}

// Custom list delegate to show "releases" instead of "items".
type customDelegate struct {
	list.DefaultDelegate
}

// ModelI is the minimal list model interface used by customDelegate.
type ModelI interface {
	Items() []list.Item
}

func (d customDelegate) RenderFooter(w int, m ModelI) string {
	defer perf.Track(nil, "toolchain.customDelegate.RenderFooter")()

	return fmt.Sprintf(" %d releases", len(m.Items()))
}

func newCustomDelegate() customDelegate {
	d := list.NewDefaultDelegate()
	return customDelegate{d}
}
