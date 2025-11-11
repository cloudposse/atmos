package toolchain

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	lipglosstable "github.com/charmbracelet/lipgloss/table"
	"github.com/mattn/go-isatty"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/tui/templates"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	"github.com/cloudposse/atmos/toolchain/registry"
)

// InfoExec handles the core logic for retrieving and formatting tool information.
func InfoExec(toolName, outputFormat string) error {
	defer perf.Track(nil, "toolchain.InfoExec")()

	ctx := context.Background()

	// Create installer inside the function.
	installer := NewInstaller()

	// Parse tool name to get owner/repo.
	owner, repo, err := installer.parseToolSpec(toolName)
	if err != nil {
		return fmt.Errorf("failed to resolve tool '%s': %w", toolName, err)
	}

	// Get installed and configured versions from tool-versions file.
	installedVersions := []string{}
	configuredVersions := []string{}
	defaultVersion := ""
	var version string
	if toolVersions, err := LoadToolVersions(GetToolVersionsFilePath()); err == nil {
		if versions, exists := toolVersions.Tools[toolName]; exists && len(versions) > 0 {
			// Track all configured versions.
			configuredVersions = versions

			// Check which versions are actually installed (have binaries on disk).
			for _, v := range versions {
				if _, err := installer.FindBinaryPath(owner, repo, v); err == nil {
					installedVersions = append(installedVersions, v)
					// First installed version becomes default.
					if defaultVersion == "" {
						defaultVersion = v
					}
				}
			}
			// If no installed versions, but versions exist in config, use last configured as default.
			if defaultVersion == "" && len(versions) > 0 {
				defaultVersion = versions[len(versions)-1]
			}
		}

		// Try to find a version using LookupToolVersionOrLatest.
		_, version, _, _ = LookupToolVersionOrLatest(toolName, toolVersions, installer.GetResolver())
	}

	// If no version found or if it's still "latest", resolve to concrete latest version.
	if version == "" || version == "latest" {
		// Get the actual latest version from the registry.
		reg := NewAquaRegistry()
		latestVersion, err := reg.GetLatestVersion(owner, repo)
		if err == nil {
			version = latestVersion
		} else {
			// If we can't get the latest version, fall back to "latest" literal.
			version = "latest"
		}
	}

	// Find the tool configuration.
	tool, err := installer.findTool(owner, repo, version)
	if err != nil {
		return fmt.Errorf("failed to find tool %s: %w", toolName, err)
	}

	// Get registry metadata to show which registry this came from.
	var registryName string
	reg := NewAquaRegistry()
	if meta, err := reg.GetMetadata(ctx); err == nil {
		registryName = meta.Name
	}

	// Try to get available versions with full metadata from GitHub (with spinner).
	availableVersions, err := fetchGitHubVersionsWithSpinner(owner, repo)
	if err != nil {
		// Log error but don't fail - just show no available versions.
		availableVersions = []versionItem{}
	} else {
		// Show latest 10 versions with full metadata.
		limit := 10
		if len(availableVersions) < limit {
			limit = len(availableVersions)
		}
		availableVersions = availableVersions[:limit]
	}

	// Display output based on format.
	if outputFormat == "yaml" {
		evaluatedYAML, err := getEvaluatedToolYAML(tool, version, installer)
		if err != nil {
			return fmt.Errorf("failed to get evaluated YAML: %w", err)
		}
		data.Write(evaluatedYAML)
	} else {
		// Enhanced table format (default).
		displayToolInfo(&toolContext{
			Name:               toolName,
			Owner:              owner,
			Repo:               repo,
			Tool:               tool,
			Version:            version,
			Installer:          installer,
			Registry:           registryName,
			InstalledVersions:  installedVersions,
			ConfiguredVersions: configuredVersions,
			AvailableVersions:  availableVersions,
			DefaultVersion:     defaultVersion,
		})
	}

	return nil
}

type toolContext struct {
	Name               string
	Owner              string
	Repo               string
	Version            string
	Tool               *registry.Tool
	Installer          *Installer
	Registry           string
	InstalledVersions  []string
	ConfiguredVersions []string
	AvailableVersions  []versionItem // Changed to versionItem for rich metadata
	DefaultVersion     string
}

// displayToolInfo displays tool information using structured table UI.
func displayToolInfo(ctx *toolContext) {
	defer perf.Track(nil, "toolchain.displayToolInfo")()

	// Get terminal width.
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width == 0 {
		width = 120 // Fallback width.
	}

	// Display tool header.
	displayToolHeader(ctx)

	// Display installed versions table.
	if len(ctx.InstalledVersions) > 0 {
		_ = ui.Writeln("")
		_ = ui.Writeln("Installed Versions:")
		displayVersionsTable(ctx.InstalledVersions, ctx.DefaultVersion, width, true)
	} else {
		_ = ui.Writeln("")
		_ = ui.Writeln("No versions installed")
	}

	// Display available versions with rich metadata.
	if len(ctx.AvailableVersions) > 0 {
		_ = ui.Writeln("")
		_ = ui.Writeln("Available Versions (latest 10):")
		displayVersionsWithMetadata(ctx.AvailableVersions, ctx.InstalledVersions, ctx.ConfiguredVersions, width)
	}

	// Display helpful hints.
	_ = ui.Writeln("")
	_ = ui.Hintf("Use `atmos toolchain install %s@<version>` to install", ctx.Name)
}

// displayToolHeader displays tool metadata in a clean format.
func displayToolHeader(ctx *toolContext) {
	// Tool name and registry.
	toolInfo := fmt.Sprintf("%s/%s", ctx.Owner, ctx.Repo)
	if ctx.Registry != "" {
		toolInfo += fmt.Sprintf(" (registry: %s)", ctx.Registry)
	}
	_ = ui.Writeln(toolInfo)

	// Tool type.
	_ = ui.Writef("Type: %s", ctx.Tool.Type)
	_ = ui.Writeln("")

	// Repository link.
	if ctx.Tool.RepoOwner != "" && ctx.Tool.RepoName != "" {
		_ = ui.Writef("Repository: https://github.com/%s/%s", ctx.Tool.RepoOwner, ctx.Tool.RepoName)
		_ = ui.Writeln("")
	}
}

// versionRow represents a row in the versions table.
type versionRow struct {
	status      string
	version     string
	isDefault   bool
	isInstalled bool
}

// displayVersionsTable displays a table of versions using Charm Bracelet table UI.
func displayVersionsTable(versions []string, defaultVersion string, terminalWidth int, showInstalled bool) {
	defer perf.Track(nil, "toolchain.displayVersionsTable")()

	// Build row data.
	var rows []versionRow
	statusWidth := 2 // For checkmark character.
	versionWidth := len("VERSION")

	for _, v := range versions {
		row := versionRow{
			version:     v,
			isDefault:   v == defaultVersion,
			isInstalled: showInstalled, // All versions in installed list are installed.
		}

		// Set status indicator.
		if row.isDefault {
			row.status = theme.Styles.Checkmark.String() // Checkmark for default.
		} else if row.isInstalled {
			row.status = theme.Styles.Checkmark.String() // Checkmark for installed.
		} else {
			row.status = " " // No indicator for available-only.
		}

		// Update column widths (account for " (default)" suffix).
		versionLen := len(v)
		if row.isDefault {
			versionLen += len(" (default)")
		}
		if versionLen > versionWidth {
			versionWidth = versionLen
		}

		rows = append(rows, row)
	}

	// Add padding.
	statusWidth += 4
	versionWidth += 4

	// Calculate total width needed.
	totalNeededWidth := statusWidth + versionWidth

	// If screen is narrow, truncate version column.
	if totalNeededWidth > terminalWidth {
		excess := totalNeededWidth - terminalWidth
		versionReduce := min(excess, versionWidth-12)
		versionWidth -= versionReduce
	}

	// Create table columns.
	columns := []table.Column{
		{Title: " ", Width: statusWidth}, // Status column.
		{Title: "VERSION", Width: versionWidth},
	}

	// Convert rows to table format.
	var tableRows []table.Row
	for _, row := range rows {
		suffix := ""
		if row.isDefault {
			suffix = " (default)"
		}
		tableRows = append(tableRows, table.Row{
			row.status,
			row.version + suffix,
		})
	}

	// Create and configure table.
	t := table.New(
		table.WithColumns(columns),
		table.WithRows(tableRows),
		table.WithFocused(false),
		table.WithHeight(len(tableRows)+1), // +1 for header row.
	)

	// Apply theme styles.
	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(theme.ColorBorder)).
		BorderBottom(true).
		Bold(true)
	s.Cell = s.Cell.PaddingLeft(1).PaddingRight(1)
	s.Selected = s.Cell

	t.SetStyles(s)

	// Render and print.
	_ = ui.Writeln(t.View())
}

// displayVersionsWithMetadata displays versions with full GitHub release metadata in table format.
// Matches the look and feel of `atmos version list` exactly.
func displayVersionsWithMetadata(versions []versionItem, installedVersions []string, configuredVersions []string, terminalWidth int) {
	defer perf.Track(nil, "toolchain.displayVersionsWithMetadata")()

	if len(versions) == 0 {
		return
	}

	// Styling to match atmos version list exactly.
	installedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10")) // Green for installed.
	configuredStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8")) // Gray for configured.
	emptyIndicator := " "
	const tableBorderPadding = 8 // Account for column padding.

	// Build table rows.
	var rows [][]string
	for _, v := range versions {
		// Determine status indicator.
		indicator := emptyIndicator
		if isVersionInstalled(v.version, installedVersions) {
			indicator = installedStyle.Render("●") // Green dot for installed.
		} else if isVersionConfigured(v.version, configuredVersions) {
			indicator = configuredStyle.Render("●") // Gray dot for configured.
		}

		// Extract date from release notes if available.
		date := extractPublishedDate(v.releaseNotes)

		// Extract meaningful title from release body.
		title := extractMeaningfulTitle(v.releaseNotes, v.title, v.version)

		// Render markdown inline in title (preserve ANSI codes).
		title = renderMarkdownInline(title)

		rows = append(rows, []string{indicator, v.version, date, title})
	}

	// Get terminal width - use exactly what's detected.
	detectedWidth := templates.GetTerminalWidth()
	tableWidth := detectedWidth - tableBorderPadding

	// Create table with lipgloss/table to match version list exactly.
	// Use lipgloss/table for auto column width calculation.
	t, err := createVersionsTable(rows, tableWidth)
	if err != nil {
		_ = ui.Writeln(err.Error())
		return
	}

	// Render and print to stderr (UI output).
	_ = ui.Writeln(t.String())
}

// createVersionsTable creates a styled table for version listing (matching atmos version list).
func createVersionsTable(rows [][]string, tableWidth int) (*lipglosstable.Table, error) {
	defer perf.Track(nil, "toolchain.createVersionsTable")()

	// Styling to match atmos version list exactly.
	headerStyle := lipgloss.NewStyle().Bold(true)
	dateStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8")) // Gray for date.

	// Create table with lipgloss/table - only border under header.
	t := lipglosstable.New().
		Headers("", "VERSION", "DATE", "TITLE").
		Rows(rows...).
		BorderHeader(true).                                               // Show border under header.
		BorderTop(false).                                                 // No top border.
		BorderBottom(false).                                              // No bottom border.
		BorderLeft(false).                                                // No left border.
		BorderRight(false).                                               // No right border.
		BorderRow(false).                                                 // No row separators.
		BorderColumn(false).                                              // No column separators.
		BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("8"))). // Gray border.
		StyleFunc(func(row, col int) lipgloss.Style {
			switch {
			case row == lipglosstable.HeaderRow:
				return headerStyle.Padding(0, 1)
			case col == 2: // Date column.
				return dateStyle.Padding(0, 1)
			default:
				return lipgloss.NewStyle().Padding(0, 1)
			}
		}).
		Width(tableWidth)

	return t, nil
}

// renderMarkdownInline renders inline markdown with proper ANSI colors preserved.
// This matches the implementation from cmd/version/formatters.go.
func renderMarkdownInline(text string) string {
	// Use Glamour to render markdown inline with colors.
	renderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(0),
	)
	if err != nil {
		// Fallback: just remove backticks if rendering fails.
		return strings.ReplaceAll(text, "`", "")
	}

	rendered, err := renderer.Render(text)
	if err != nil {
		// Fallback: just remove backticks if rendering fails.
		return strings.ReplaceAll(text, "`", "")
	}

	// Remove newlines but preserve ANSI color codes.
	rendered = strings.TrimSpace(rendered)
	rendered = strings.ReplaceAll(rendered, "\n", " ")

	return rendered
}

// isVersionConfigured checks if a version is in the configured list (but not necessarily installed).
func isVersionConfigured(version string, configuredVersions []string) bool {
	// Normalize version for comparison (strip "v" prefix if present).
	normalizedVersion := strings.TrimPrefix(version, "v")
	for _, cv := range configuredVersions {
		normalizedCV := strings.TrimPrefix(cv, "v")
		if normalizedCV == normalizedVersion {
			return true
		}
	}
	return false
}

// extractPublishedDate extracts the published date from formatted release notes.
func extractPublishedDate(releaseNotes string) string {
	// Release notes are formatted as "# Title\n\n**Published:** DATE\n\n..."
	lines := strings.Split(releaseNotes, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "**Published:**") {
			// Extract date (format: "**Published:** 2024-01-15T12:00:00Z").
			parts := strings.SplitN(line, "**Published:**", 2)
			if len(parts) == 2 {
				dateStr := strings.TrimSpace(parts[1])
				// Parse and format as YYYY-MM-DD.
				if len(dateStr) >= 10 {
					return dateStr[:10]
				}
			}
		}
	}
	return ""
}

// extractMeaningfulTitle extracts a meaningful title from release notes.
// This matches the logic from cmd/version/formatters.go for consistency.
func extractMeaningfulTitle(releaseNotes, fallbackTitle, version string) string {
	// The releaseNotes are formatted as:
	// # Title
	// **Published:** DATE
	// <actual body content>
	//
	// We need to extract the body content and look for meaningful headings.

	// Split by "**Published:**" to get to the body content.
	parts := strings.SplitN(releaseNotes, "**Published:**", 2)
	var body string
	if len(parts) == 2 {
		// Skip the date line and get the rest.
		bodyParts := strings.SplitN(parts[1], "\n\n", 2)
		if len(bodyParts) == 2 {
			body = bodyParts[1]
		}
	}

	// If we have body content, try to extract a meaningful title.
	if body != "" {
		if title := extractFirstHeading(body); title != "" {
			return title
		}
	}

	// If fallback title is different from version, use it.
	if fallbackTitle != "" && fallbackTitle != version && fallbackTitle != "v"+version {
		return fallbackTitle
	}

	// Last resort: return the version.
	return version
}

// extractFirstHeading extracts the first meaningful heading from markdown text.
// It looks for <summary> tags first, then H1/H2 headings.
// This matches the logic from cmd/version/formatters.go for consistency.
func extractFirstHeading(markdown string) string {
	// Try to extract from <summary> tag first (common in GitHub releases).
	summaryRe := regexp.MustCompile(`<summary>(.+?)</summary>`)
	if matches := summaryRe.FindStringSubmatch(markdown); len(matches) > 1 {
		// Clean up the summary text (remove markdown, whitespace, etc.).
		summary := strings.TrimSpace(matches[1])
		// Remove any trailing author/PR references like "@user (#123)".
		summary = regexp.MustCompile(`\s+@\S+\s+\(#\d+\)$`).ReplaceAllString(summary, "")
		if summary != "" {
			return summary
		}
	}

	// Fall back to first H1 or H2 heading.
	headingRe := regexp.MustCompile(`(?m)^#{1,2}\s+(.+)$`)
	if matches := headingRe.FindStringSubmatch(markdown); len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}

	return ""
}

// isVersionInstalled checks if a version is in the installed list.
func isVersionInstalled(version string, installedVersions []string) bool {
	// Normalize version for comparison (strip "v" prefix if present).
	normalizedVersion := strings.TrimPrefix(version, "v")
	for _, iv := range installedVersions {
		normalizedIV := strings.TrimPrefix(iv, "v")
		if normalizedIV == normalizedVersion {
			return true
		}
	}
	return false
}

// versionFetchModel is the bubbletea model for fetching versions with a spinner.
type versionFetchModel struct {
	spinner  spinner.Model
	versions []versionItem
	err      error
	done     bool
	owner    string
	repo     string
}

func (m *versionFetchModel) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		fetchVersionsCmd(m.owner, m.repo),
	)
}

func (m *versionFetchModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m, tea.Quit
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case []versionItem:
		m.versions = msg
		m.done = true
		return m, tea.Quit
	case error:
		m.err = msg
		m.done = true
		return m, tea.Quit
	default:
		return m, nil
	}
}

func (m *versionFetchModel) View() string {
	if m.done {
		return ""
	}
	return m.spinner.View() + " Fetching versions from GitHub..."
}

// fetchVersionsCmd returns a command that fetches versions from GitHub.
func fetchVersionsCmd(owner, repo string) tea.Cmd {
	return func() tea.Msg {
		versions, err := fetchGitHubVersions(owner, repo)
		if err != nil {
			return err
		}
		return versions
	}
}

// fetchGitHubVersionsWithSpinner fetches versions with a spinner if TTY is available.
func fetchGitHubVersionsWithSpinner(owner, repo string) ([]versionItem, error) {
	defer perf.Track(nil, "toolchain.fetchGitHubVersionsWithSpinner")()

	// Check if we have a TTY for the spinner.
	//nolint:nestif // Spinner logic requires nested conditions for TTY check.
	if isatty.IsTerminal(os.Stderr.Fd()) || isatty.IsCygwinTerminal(os.Stderr.Fd()) {
		// Create spinner model.
		s := spinner.New()
		s.Spinner = spinner.Dot

		// Fetch versions with spinner.
		m := &versionFetchModel{spinner: s, owner: owner, repo: repo}
		p := tea.NewProgram(m, tea.WithOutput(os.Stderr))

		// Run the spinner.
		finalModel, err := p.Run()
		if err != nil {
			return nil, fmt.Errorf("spinner execution failed: %w", err)
		}

		// Check for nil model.
		if finalModel == nil {
			return nil, fmt.Errorf("%w: spinner completed but returned nil model during version fetch", errUtils.ErrSpinnerReturnedNilModel)
		}

		// Get the final model with type assertion safety.
		final, ok := finalModel.(*versionFetchModel)
		if !ok {
			return nil, fmt.Errorf("%w: got %T", errUtils.ErrSpinnerUnexpectedModelType, finalModel)
		}

		if final.err != nil {
			return nil, fmt.Errorf("failed to fetch versions: %w", final.err)
		}

		return final.versions, nil
	}

	// No TTY - fetch without spinner.
	return fetchGitHubVersions(owner, repo)
}

// getEvaluatedToolYAML returns the tool configuration as YAML with all templates evaluated.
func getEvaluatedToolYAML(tool *registry.Tool, version string, installer *Installer) (string, error) {
	// Build the asset URL to evaluate templates (if Asset is set).
	assetURL := ""
	var err error
	if tool.Asset != "" {
		assetURL, err = installer.buildAssetURL(tool, version)
		if err != nil {
			return "", fmt.Errorf("failed to build asset URL: %w", err)
		}
	}

	// Create a copy of the tool with evaluated templates.
	evaluatedTool := struct {
		Type         string              `yaml:"type"`
		RepoOwner    string              `yaml:"repo_owner"`
		RepoName     string              `yaml:"repo_name"`
		Asset        string              `yaml:"asset"`
		URL          string              `yaml:"url,omitempty"`
		Format       string              `yaml:"format,omitempty"`
		BinaryName   string              `yaml:"binary_name,omitempty"`
		Files        []registry.File     `yaml:"files,omitempty"`
		Overrides    []registry.Override `yaml:"overrides,omitempty"`
		Version      string              `yaml:"version"`
		ProcessedURL string              `yaml:"processed_url,omitempty"`
	}{
		Type:         tool.Type,
		RepoOwner:    tool.RepoOwner,
		RepoName:     tool.RepoName,
		Asset:        assetURL, // Use evaluated URL instead of template
		URL:          tool.URL,
		Format:       tool.Format,
		BinaryName:   tool.BinaryName,
		Files:        tool.Files,
		Overrides:    tool.Overrides,
		Version:      version,
		ProcessedURL: assetURL,
	}

	// Marshal to YAML.
	data, err := yaml.Marshal(evaluatedTool)
	if err != nil {
		return "", fmt.Errorf("failed to marshal tool to YAML: %w", err)
	}

	return string(data), nil
}
