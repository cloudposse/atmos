package version

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/google/go-github/v59/github"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/tui/templates"
	log "github.com/cloudposse/atmos/pkg/logger"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/cloudposse/atmos/pkg/version"
)

const (
	bytesPerKB         = 1024
	bytesPerMB         = bytesPerKB * bytesPerKB
	minWidth           = 40
	tableBorderPadding = 8 // Account for column padding (2 chars per column * 4 columns).
	versionPrefix      = "v"
	emptyIndicator     = " "
)

var (
	// Table styling.
	currentVersionStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10")) // Green.
	headerStyle         = lipgloss.NewStyle().Bold(true)
	dateStyle           = lipgloss.NewStyle().Foreground(lipgloss.Color("8")) // Gray.
)

// extractFirstHeading extracts the first meaningful heading from markdown text.
// It looks for <summary> tags first, then H1/H2 headings.
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

// getReleaseTitle returns a meaningful title for the release.
// If the title matches the tag, it extracts the first heading from release notes.
func getReleaseTitle(release *github.RepositoryRelease) string {
	title := release.GetName()
	tag := release.GetTagName()

	// If title is the same as tag (or empty), try to extract from release notes.
	if title == tag || title == "" {
		if body := release.GetBody(); body != "" {
			if heading := extractFirstHeading(body); heading != "" {
				return heading
			}
		}
		return tag
	}

	return title
}

// isCurrentVersion checks if a version tag matches the current Atmos version.
func isCurrentVersion(tag string) bool {
	currentVersion := version.Version
	// Normalize versions for comparison (handle with/without 'v' prefix).
	normalizedTag := strings.TrimPrefix(tag, versionPrefix)
	normalizedCurrent := strings.TrimPrefix(currentVersion, versionPrefix)
	return normalizedTag == normalizedCurrent
}

// addCurrentVersionIfMissing adds the current version to the top of the list if it's not already present.
func addCurrentVersionIfMissing(releases []*github.RepositoryRelease) []*github.RepositoryRelease {
	currentVersion := version.Version
	if currentVersion == "" {
		return releases
	}

	// Normalize current version for comparison.
	normalizedCurrent := strings.TrimPrefix(currentVersion, versionPrefix)

	// Check if current version is already in the list.
	for _, release := range releases {
		normalizedTag := strings.TrimPrefix(release.GetTagName(), versionPrefix)
		if normalizedTag == normalizedCurrent {
			return releases // Already in the list.
		}
	}

	// Current version not found - add it synthetically at the top.
	currentTag := currentVersion
	if !strings.HasPrefix(currentTag, versionPrefix) {
		currentTag = versionPrefix + currentTag
	}

	syntheticRelease := &github.RepositoryRelease{
		TagName:     github.String(currentTag),
		Name:        github.String(currentTag),
		Body:        github.String("Current installed version"),
		PublishedAt: &github.Timestamp{Time: time.Now()},
		Prerelease:  github.Bool(false),
		HTMLURL:     github.String(""),
	}

	// Prepend to the list.
	return append([]*github.RepositoryRelease{syntheticRelease}, releases...)
}

// filterAssetsByPlatform filters release assets to only include those matching the current OS and architecture.
func filterAssetsByPlatform(assets []*github.ReleaseAsset) []*github.ReleaseAsset {
	currentOS := runtime.GOOS
	currentArch := runtime.GOARCH

	// Map Go OS/arch to common naming patterns in release assets.
	osPatterns := map[string][]string{
		"darwin":  {"darwin", "macos", "osx"},
		"linux":   {"linux"},
		"windows": {"windows", "win32", "win64"},
	}

	archPatterns := map[string][]string{
		"amd64": {"amd64", "x86_64", "x64"},
		"arm64": {"arm64", "aarch64"},
		"386":   {"386", "i386", "x86"},
		"arm":   {"arm", "armv7"},
	}

	var filtered []*github.ReleaseAsset
	for _, asset := range assets {
		name := strings.ToLower(asset.GetName())

		// Check if asset matches current OS.
		osMatch := false
		for _, pattern := range osPatterns[currentOS] {
			if strings.Contains(name, pattern) {
				osMatch = true
				break
			}
		}

		// Check if asset matches current architecture.
		archMatch := false
		for _, pattern := range archPatterns[currentArch] {
			if strings.Contains(name, pattern) {
				archMatch = true
				break
			}
		}

		// Include asset if it matches both OS and architecture.
		if osMatch && archMatch {
			filtered = append(filtered, asset)
		}
	}

	return filtered
}

// renderMarkdownInline renders inline markdown with proper ANSI colors preserved.
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

// createVersionTable creates a styled table for version listing.
func createVersionTable(rows [][]string) (*table.Table, error) {
	// Get terminal width - use exactly what's detected.
	detectedWidth := templates.GetTerminalWidth()

	// Check if terminal is too narrow.
	if detectedWidth < minWidth {
		return nil, fmt.Errorf("%w: detected %d chars, minimum required %d chars", errUtils.ErrTerminalTooNarrow, detectedWidth, minWidth)
	}

	// Account for table borders and padding.
	tableWidth := detectedWidth - tableBorderPadding

	log.Debug("Terminal width detection", "detectedWidth", detectedWidth, "tableWidth", tableWidth)

	// Create table with lipgloss - only border under header.
	return table.New().
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
			case row == table.HeaderRow:
				return headerStyle.Padding(0, 1)
			case col == 2: // Date column.
				return dateStyle.Padding(0, 1)
			default:
				return lipgloss.NewStyle().Padding(0, 1)
			}
		}).
		Width(tableWidth), nil
}

// formatReleaseListText outputs releases as a formatted table using lipgloss/table.
func formatReleaseListText(releases []*github.RepositoryRelease) error {
	// Add current version if it's not in the list.
	releases = addCurrentVersionIfMissing(releases)

	if len(releases) == 0 {
		fmt.Println("No releases found")
		return nil
	}

	// Build table rows.
	var rows [][]string
	for _, release := range releases {
		tag := release.GetTagName()
		date := release.GetPublishedAt().Format("2006-01-02")
		title := getReleaseTitle(release)

		// Render markdown in title.
		title = renderMarkdownInline(title)

		// Add indicator for current version.
		indicator := emptyIndicator
		if isCurrentVersion(tag) {
			indicator = currentVersionStyle.Render("●")
		}

		// Add prerelease indicator.
		if release.GetPrerelease() {
			title += " (pre-release)"
		}

		rows = append(rows, []string{indicator, tag, date, title})
	}

	t, err := createVersionTable(rows)
	if err != nil {
		return err
	}
	fmt.Println(t)
	return nil
}

// formatReleaseListJSON outputs releases as JSON.
func formatReleaseListJSON(releases []*github.RepositoryRelease) error {
	// Add current version if it's not in the list.
	releases = addCurrentVersionIfMissing(releases)

	type releaseInfo struct {
		Tag        string `json:"tag"`
		Name       string `json:"name"`
		Title      string `json:"title"`
		Published  string `json:"published"`
		Prerelease bool   `json:"prerelease"`
		Current    bool   `json:"current"`
		URL        string `json:"url"`
	}

	var output []releaseInfo
	for _, release := range releases {
		output = append(output, releaseInfo{
			Tag:        release.GetTagName(),
			Name:       release.GetName(),
			Title:      getReleaseTitle(release),
			Published:  release.GetPublishedAt().Format(time.RFC3339),
			Prerelease: release.GetPrerelease(),
			Current:    isCurrentVersion(release.GetTagName()),
			URL:        release.GetHTMLURL(),
		})
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

// formatReleaseListYAML outputs releases as YAML.
func formatReleaseListYAML(releases []*github.RepositoryRelease) error {
	// Add current version if it's not in the list.
	releases = addCurrentVersionIfMissing(releases)

	type releaseInfo struct {
		Tag        string `yaml:"tag"`
		Name       string `yaml:"name"`
		Title      string `yaml:"title"`
		Published  string `yaml:"published"`
		Prerelease bool   `yaml:"prerelease"`
		Current    bool   `yaml:"current"`
		URL        string `yaml:"url"`
	}

	var output []releaseInfo
	for _, release := range releases {
		output = append(output, releaseInfo{
			Tag:        release.GetTagName(),
			Name:       release.GetName(),
			Title:      getReleaseTitle(release),
			Published:  release.GetPublishedAt().Format(time.RFC3339),
			Prerelease: release.GetPrerelease(),
			Current:    isCurrentVersion(release.GetTagName()),
			URL:        release.GetHTMLURL(),
		})
	}

	encoder := yaml.NewEncoder(os.Stdout)
	return encoder.Encode(output)
}

// formatReleaseDetailText outputs a single release in text format.
func formatReleaseDetailText(release *github.RepositoryRelease) {
	fmt.Printf("Version: %s\n", release.GetTagName())
	fmt.Printf("Name: %s\n", release.GetName())
	fmt.Printf("Published: %s\n", release.GetPublishedAt().Format("2006-01-02 15:04:05 MST"))

	if release.GetPrerelease() {
		fmt.Println("Type: Pre-release")
	} else {
		fmt.Println("Type: Stable")
	}

	if isCurrentVersion(release.GetTagName()) {
		fmt.Println(currentVersionStyle.Render("Current: ● Yes (installed)"))
	}

	fmt.Printf("URL: %s\n", release.GetHTMLURL())
	fmt.Println()

	// Release notes (rendered as markdown).
	if body := release.GetBody(); body != "" {
		fmt.Println("Release Notes:")
		fmt.Println("─────────────────────────────────────────────────────────────────")
		u.PrintfMarkdown("%s", body)
		fmt.Println("─────────────────────────────────────────────────────────────────")
		fmt.Println()
	}

	// Assets (filtered by current OS and architecture).
	filteredAssets := filterAssetsByPlatform(release.Assets)
	if len(filteredAssets) > 0 {
		fmt.Printf("\nAssets for %s/%s:\n", runtime.GOOS, runtime.GOARCH)
		for _, asset := range filteredAssets {
			sizeMB := float64(asset.GetSize()) / float64(bytesPerMB)
			// Style the filename without the size, then show size in muted color.
			filename := asset.GetName()
			sizeText := fmt.Sprintf("(%.2f MB)", sizeMB)

			fmt.Printf("  %s %s\n", filename, lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(sizeText))

			// Render the URL as a link.
			linkStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Underline(true)
			fmt.Printf("  %s\n", linkStyle.Render(asset.GetBrowserDownloadURL()))
		}
	} else if len(release.Assets) > 0 {
		fmt.Printf("\nNo assets found for %s/%s\n", runtime.GOOS, runtime.GOARCH)
	}
}

// formatReleaseDetailJSON outputs a single release in JSON format.
func formatReleaseDetailJSON(release *github.RepositoryRelease) error {
	type assetInfo struct {
		Name        string `json:"name"`
		Size        int    `json:"size"`
		DownloadURL string `json:"download_url"`
	}

	type releaseDetail struct {
		Tag        string      `json:"tag"`
		Name       string      `json:"name"`
		Title      string      `json:"title"`
		Published  string      `json:"published"`
		Prerelease bool        `json:"prerelease"`
		Current    bool        `json:"current"`
		Body       string      `json:"body"`
		URL        string      `json:"url"`
		Assets     []assetInfo `json:"assets"`
	}

	var assets []assetInfo
	filteredAssets := filterAssetsByPlatform(release.Assets)
	for _, asset := range filteredAssets {
		assets = append(assets, assetInfo{
			Name:        asset.GetName(),
			Size:        asset.GetSize(),
			DownloadURL: asset.GetBrowserDownloadURL(),
		})
	}

	output := releaseDetail{
		Tag:        release.GetTagName(),
		Name:       release.GetName(),
		Title:      getReleaseTitle(release),
		Published:  release.GetPublishedAt().Format("2006-01-02 15:04:05 MST"),
		Prerelease: release.GetPrerelease(),
		Current:    isCurrentVersion(release.GetTagName()),
		Body:       release.GetBody(),
		URL:        release.GetHTMLURL(),
		Assets:     assets,
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

// formatReleaseDetailYAML outputs a single release in YAML format.
func formatReleaseDetailYAML(release *github.RepositoryRelease) error {
	type assetInfo struct {
		Name        string `yaml:"name"`
		Size        int    `yaml:"size"`
		DownloadURL string `yaml:"download_url"`
	}

	type releaseDetail struct {
		Tag        string      `yaml:"tag"`
		Name       string      `yaml:"name"`
		Title      string      `yaml:"title"`
		Published  string      `yaml:"published"`
		Prerelease bool        `yaml:"prerelease"`
		Current    bool        `yaml:"current"`
		Body       string      `yaml:"body"`
		URL        string      `yaml:"url"`
		Assets     []assetInfo `yaml:"assets"`
	}

	var assets []assetInfo
	filteredAssets := filterAssetsByPlatform(release.Assets)
	for _, asset := range filteredAssets {
		assets = append(assets, assetInfo{
			Name:        asset.GetName(),
			Size:        asset.GetSize(),
			DownloadURL: asset.GetBrowserDownloadURL(),
		})
	}

	output := releaseDetail{
		Tag:        release.GetTagName(),
		Name:       release.GetName(),
		Title:      getReleaseTitle(release),
		Published:  release.GetPublishedAt().Format("2006-01-02 15:04:05 MST"),
		Prerelease: release.GetPrerelease(),
		Current:    isCurrentVersion(release.GetTagName()),
		Body:       release.GetBody(),
		URL:        release.GetHTMLURL(),
		Assets:     assets,
	}

	encoder := yaml.NewEncoder(os.Stdout)
	return encoder.Encode(output)
}
