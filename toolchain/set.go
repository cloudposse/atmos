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

const (
	// VersionListUIReservedHeight is the number of lines reserved for title, borders, and status bar.
	versionListUIReservedHeight = 6

	// FocusList indicates the list pane has focus in the version selector UI.
	focusList = "list"

	// FocusViewport indicates the viewport pane has focus in the version selector UI.
	focusViewport = "viewport"
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

func (m *versionListModel) Init() tea.Cmd {
	defer perf.Track(nil, "toolchain.versionListModel.Init")()

	return nil
}

func (m *versionListModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	defer perf.Track(nil, "toolchain.versionListModel.Update")()

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if model, cmd, handled := m.handleKeyMsg(msg); handled {
			return model, cmd
		}
	case tea.WindowSizeMsg:
		m.handleWindowSizeMsg(msg)
	}

	cmd := m.updateListComponent(msg)
	m.updateViewportContent()
	vpCmd := m.updateViewportComponent(msg)

	return m, tea.Batch(cmd, vpCmd)
}

// handleKeyMsg handles keyboard input and returns (model, cmd, handled).
func (m *versionListModel) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit, true
	case "tab":
		m.toggleFocus()
		return m, nil, true
	case "enter":
		if m.list.SelectedItem() != nil {
			selectedItem := m.list.SelectedItem().(versionItem)
			m.selected = selectedItem.version
			return m, tea.Quit, true
		}
	}
	return m, nil, false
}

// toggleFocus switches focus between list and viewport.
func (m *versionListModel) toggleFocus() {
	if m.focused == focusList {
		m.focused = focusViewport
	} else {
		m.focused = focusList
	}
}

// handleWindowSizeMsg handles window resize events.
func (m *versionListModel) handleWindowSizeMsg(msg tea.WindowSizeMsg) {
	leftWidth := msg.Width / 4
	rightWidth := msg.Width - leftWidth - 2
	contentHeight := msg.Height - versionListUIReservedHeight

	m.list.SetSize(leftWidth-2, contentHeight)
	m.viewport.Width = rightWidth - 2
	m.viewport.Height = contentHeight
	m.updateViewportContent()
}

// updateListComponent updates the list component based on focus.
func (m *versionListModel) updateListComponent(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	if m.focused == focusList {
		m.list, cmd = m.list.Update(msg)
	} else {
		m.list, cmd = m.list.Update(tea.WindowSizeMsg{})
	}
	return cmd
}

// updateViewportContent updates viewport content when selection changes.
func (m *versionListModel) updateViewportContent() {
	if m.list.SelectedItem() == nil {
		return
	}
	selectedItem := m.list.SelectedItem().(versionItem)
	currentIndex := m.list.Index()
	if currentIndex != m.currentItemIndex {
		renderedNotes := renderMarkdown(selectedItem.releaseNotes, m.viewport.Width)
		m.viewport.SetContent(renderedNotes)
		m.currentItemIndex = currentIndex
	}
}

// updateViewportComponent handles viewport scrolling and updates.
func (m *versionListModel) updateViewportComponent(msg tea.Msg) tea.Cmd {
	if m.focused != "viewport" {
		var vpCmd tea.Cmd
		m.viewport, vpCmd = m.viewport.Update(tea.WindowSizeMsg{})
		return vpCmd
	}

	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if m.handleViewportKeyMsg(keyMsg) {
			return nil
		}
	}

	var vpCmd tea.Cmd
	m.viewport, vpCmd = m.viewport.Update(msg)
	return vpCmd
}

// handleViewportKeyMsg handles keyboard input for viewport scrolling.
// Returns true if the key was handled.
func (m *versionListModel) handleViewportKeyMsg(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "up", "k":
		for i := 0; i < m.scrollSpeed; i++ {
			m.viewport.ScrollUp(1)
		}
		return true
	case "down", "j":
		for i := 0; i < m.scrollSpeed; i++ {
			m.viewport.ScrollDown(1)
		}
		return true
	case "pgup":
		for i := 0; i < m.scrollSpeed; i++ {
			m.viewport.PageUp()
		}
		return true
	case "pgdown":
		for i := 0; i < m.scrollSpeed; i++ {
			m.viewport.PageDown()
		}
		return true
	case "home", "g":
		m.viewport.GotoTop()
		return true
	case "end", "G":
		m.viewport.GotoBottom()
		return true
	}
	return false
}

func (m *versionListModel) View() string {
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
	if m.focused == focusList {
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

	// Resolve the tool name to handle aliases.
	installer := NewInstaller()
	spec, err := resolveToolName(toolName, installer)
	if err != nil {
		return err
	}

	// If no version provided, fetch available versions and show interactive selection.
	if version == "" {
		// Validate tool supports interactive selection.
		if err := validateToolForInteractiveSelection(installer, spec.owner, spec.repo); err != nil {
			return err
		}

		// Fetch and validate available versions.
		items, err := fetchAndValidateVersions(spec.owner, spec.repo)
		if err != nil {
			return err
		}

		// Create and configure the interactive UI model.
		m := createVersionListModel(spec.owner, spec.repo, items, scrollSpeed)

		// Run the interactive selection and get the chosen version.
		version, err = runInteractiveSelection(m)
		if err != nil {
			return err
		}
	}

	// Add the tool with the selected version.
	filePath := GetToolVersionsFilePath()
	err = AddToolToVersions(filePath, spec.key, version)
	if err != nil {
		return fmt.Errorf("failed to set version: %w", err)
	}

	return ui.Successf("Set %s@%s in %s", spec.key, version, filePath)
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

func (d *customDelegate) RenderFooter(w int, m ModelI) string {
	defer perf.Track(nil, "toolchain.customDelegate.RenderFooter")()

	return fmt.Sprintf(" %d releases", len(m.Items()))
}

func newCustomDelegate() *customDelegate {
	d := list.NewDefaultDelegate()
	return &customDelegate{d}
}
