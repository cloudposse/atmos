package toolchain

import (
	"fmt"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/tui/templates/term"
)

// toolSpec holds resolved owner, repo, and key for a tool.
type toolSpec struct {
	owner string
	repo  string
	key   string
}

// resolveToolName resolves a tool name or alias to owner/repo format.
func resolveToolName(toolName string, installer *Installer) (toolSpec, error) {
	owner, repo, err := installer.parseToolSpec(toolName)
	if err != nil {
		return toolSpec{}, fmt.Errorf("invalid tool name: %w", err)
	}
	return toolSpec{
		owner: owner,
		repo:  repo,
		key:   owner + "/" + repo,
	}, nil
}

// validateToolForInteractiveSelection validates that a tool supports interactive version selection.
func validateToolForInteractiveSelection(installer *Installer, owner, repo string) error {
	tool, err := installer.findTool(owner, repo, "")
	if err != nil {
		return fmt.Errorf("failed to find tool configuration: %w", err)
	}

	if tool.Type != "github_release" {
		return fmt.Errorf("%w: interactive version selection is only available for GitHub release type tools", ErrInvalidToolSpec)
	}

	return nil
}

// fetchAndValidateVersions fetches available versions from GitHub and validates the result.
func fetchAndValidateVersions(owner, repo string) ([]versionItem, error) {
	items, err := fetchGitHubVersions(owner, repo)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch versions from GitHub: %w", err)
	}

	if len(items) == 0 {
		return nil, fmt.Errorf("%w: no versions found for %s/%s", ErrNoVersionsFound, owner, repo)
	}

	return items, nil
}

// createVersionListModel creates and configures the interactive version selection UI model.
func createVersionListModel(owner, repo string, items []versionItem, scrollSpeed int) *versionListModel {
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

	m := &versionListModel{
		list:        l,
		viewport:    vp,
		owner:       owner,
		repo:        repo,
		items:       items,
		title:       fmt.Sprintf("Select version for %s/%s", owner, repo),
		focused:     focusList,
		scrollSpeed: scrollSpeed,
	}

	// Set initial viewport content if items exist.
	if len(items) > 0 {
		renderedNotes := renderMarkdown(items[0].releaseNotes, m.viewport.Width)
		m.viewport.SetContent(renderedNotes)
		m.currentItemIndex = 0
	}

	return m
}

// runInteractiveSelection runs the interactive version selection UI and returns the selected version.
// Requires a TTY for interactive input; returns an error in non-TTY environments (CI, piped input).
func runInteractiveSelection(m *versionListModel) (string, error) {
	// Check if we have a TTY for interactive selection.
	if !term.IsTTYSupportForStdin() {
		return "", fmt.Errorf("%w: interactive version selection requires a TTY. Please specify a version explicitly", errUtils.ErrTTYRequired)
	}

	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		return "", fmt.Errorf("failed to run interactive selection: %w", err)
	}

	finalVersionModel := finalModel.(*versionListModel)
	if finalVersionModel.selected == "" {
		return "", fmt.Errorf("%w: no version selected", ErrInvalidToolSpec)
	}

	return finalVersionModel.selected, nil
}
