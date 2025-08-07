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
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type versionItem struct {
	version      string
	title        string
	desc         string
	releaseNotes string
}

func (i versionItem) Title() string       { return i.version }
func (i versionItem) Description() string { return i.title }
func (i versionItem) FilterValue() string { return i.version }

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
	return nil
}

func (m versionListModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
					m.viewport.LineUp(1)
				}
				return m, nil
			case "down", "j":
				// Scroll down by scroll speed amount
				for i := 0; i < m.scrollSpeed; i++ {
					m.viewport.LineDown(1)
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

var setCmd = &cobra.Command{
	Use:   "set <tool> [version]",
	Short: "Set a specific version for a tool in .tool-versions",
	Long: `Set a specific version for a tool in the .tool-versions file.

If no version is provided, this command will fetch available versions from GitHub releases
and present them in an interactive selection (only works for github_release type tools).

Examples:
  toolchain set terraform 1.11.4
  toolchain set hashicorp/terraform 1.11.4
  toolchain set terraform  # Interactive version selection
  toolchain set --file /path/to/.tool-versions kubectl 1.28.0`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath, _ := cmd.Flags().GetString("file")
		if filePath == "" {
			filePath = GetToolVersionsFilePath()
		}

		toolName := args[0]
		var version string
		if len(args) > 1 {
			version = args[1]
		}

		// Resolve the tool name to handle aliases
		installer := NewInstaller()
		owner, repo, err := installer.parseToolSpec(toolName)
		if err != nil {
			return fmt.Errorf("invalid tool name: %w", err)
		}
		resolvedKey := owner + "/" + repo

		// If no version provided, fetch available versions and show interactive selection
		if version == "" {
			// Get tool configuration to check if it's a github_release type
			tool, err := installer.findTool(owner, repo, "")
			if err != nil {
				return fmt.Errorf("failed to find tool configuration: %w", err)
			}

			// Check if this is a GitHub release type tool
			if tool.Type != "github_release" {
				return fmt.Errorf("interactive version selection is only available for GitHub release type tools")
			}

			// Fetch available versions with titles
			items, err := fetchGitHubVersions(owner, repo)
			if err != nil {
				return fmt.Errorf("failed to fetch versions from GitHub: %w", err)
			}

			if len(items) == 0 {
				return fmt.Errorf("no versions found for %s/%s", owner, repo)
			}

			// Convert versionItem slice to list.Item slice
			listItems := make([]list.Item, len(items))
			for i, item := range items {
				listItems[i] = item
			}

			l := list.New(listItems, newCustomDelegate(), 0, 0)
			l.Title = "Select Version" // Add back the list title
			l.SetShowHelp(false)       // Disable default help to show custom footer
			l.SetShowStatusBar(true)   // Show status bar with custom footer
			l.InfiniteScrolling = true // Enable infinite scrolling to show all items

			// Initialize viewport
			vp := viewport.New(0, 0)
			vp.Style = lipgloss.NewStyle().
				Foreground(lipgloss.Color("62")).
				Padding(0, 0) // Remove padding to use full width

			// Get scroll speed from flags
			scrollSpeed, _ := cmd.Flags().GetInt("scroll-speed")
			if scrollSpeed < 1 {
				scrollSpeed = 3 // Default to fast speed
			}

			m := versionListModel{
				list:        l,
				viewport:    vp,
				owner:       owner,
				repo:        repo,
				items:       items,
				title:       fmt.Sprintf("Select version for %s/%s", owner, repo),
				focused:     "list", // Start with list focused
				scrollSpeed: scrollSpeed,
			}

			// Set initial release notes with markdown rendering
			if len(items) > 0 {
				renderedNotes := renderMarkdown(items[0].releaseNotes, m.viewport.Width)
				// Set content without width styling to use full available width
				m.viewport.SetContent(renderedNotes)
				m.currentItemIndex = 0 // Initialize to first item
			}

			p := tea.NewProgram(m)

			finalModel, err := p.Run()
			if err != nil {
				return fmt.Errorf("failed to run interactive selection: %w", err)
			}

			// Cast the final model back to our type to get the selected version
			finalVersionModel := finalModel.(versionListModel)
			if finalVersionModel.selected == "" {
				return fmt.Errorf("no version selected")
			}

			version = finalVersionModel.selected
		}

		// Add the tool with the selected version
		err = AddToolToVersions(filePath, resolvedKey, version)
		if err != nil {
			return fmt.Errorf("failed to set version: %w", err)
		}

		cmd.Printf("✓ Set %s@%s in %s\n", resolvedKey, version, filePath)
		return nil
	},
}

func init() {
	setCmd.Flags().String("file", "", "Path to tool-versions file (defaults to global --tool-versions-file)")
	setCmd.Flags().Int("scroll-speed", 3, "Scroll speed multiplier for viewport (1=normal, 2=fast, 3=very fast, etc.)")
}

// fetchGitHubVersions fetches available versions and titles from GitHub releases
func fetchGitHubVersions(owner, repo string) ([]versionItem, error) {
	// GitHub API endpoint for releases with per_page parameter to get more releases
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases?per_page=100", owner, repo)

	// Get GitHub token for authenticated requests
	token := viper.GetString("github-token")

	// Create HTTP client
	client := &http.Client{}
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add GitHub token if available
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch releases from GitHub: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Parse the JSON response
	var releases []struct {
		TagName     string `json:"tag_name"`
		Name        string `json:"name"`
		Body        string `json:"body"`
		Prerelease  bool   `json:"prerelease"`
		PublishedAt string `json:"published_at"`
	}

	if err := json.Unmarshal(body, &releases); err != nil {
		return nil, fmt.Errorf("failed to parse releases JSON: %w", err)
	}

	// Debug: Print the number of releases fetched
	fmt.Printf("Fetched %d total releases from GitHub\n", len(releases))

	// Extract all non-prerelease versions with their titles and release notes
	var items []versionItem
	for _, release := range releases {
		if !release.Prerelease {
			// Remove 'v' prefix if present
			version := strings.TrimPrefix(release.TagName, "v")

			// Use the release name if available, otherwise use the tag name
			title := release.Name
			if title == "" {
				title = release.TagName
			}

			// Format release notes
			releaseNotes := formatReleaseNotes(release.Name, release.TagName, release.Body, release.PublishedAt)

			items = append(items, versionItem{
				version:      version,
				title:        title,
				desc:         fmt.Sprintf("Version %s", version),
				releaseNotes: releaseNotes,
			})
		}
	}

	// Debug: Print the number of non-prerelease versions
	fmt.Printf("Found %d non-prerelease versions\n", len(items))

	if len(items) == 0 {
		return nil, fmt.Errorf("no non-prerelease versions found for %s/%s", owner, repo)
	}

	return items, nil
}

// formatReleaseNotes formats the release notes for display
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

// renderMarkdown renders markdown content using glamour
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

// Custom list delegate to show "releases" instead of "items"
type customDelegate struct {
	list.DefaultDelegate
}

func (d customDelegate) RenderFooter(w int, m list.Model) string {
	return fmt.Sprintf(" %d releases", len(m.Items()))
}

func newCustomDelegate() customDelegate {
	d := list.NewDefaultDelegate()
	return customDelegate{d}
}
