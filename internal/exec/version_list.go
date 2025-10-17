package exec

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/google/go-github/v59/github"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/utils"
)

const (
	versionListMaxLimit   = 100
	versionListBytesPerKB = 1024
)

// ExecuteVersionList lists Atmos releases from GitHub.
//
//nolint:revive // Function needs these parameters for CLI integration.
func ExecuteVersionList(
	atmosConfig *schema.AtmosConfiguration,
	limit int,
	offset int,
	since string,
	includePrereleases bool,
	format string,
) error {
	defer perf.Track(atmosConfig, "exec.ExecuteVersionList")()

	// Validate limit.
	if limit < 1 || limit > versionListMaxLimit {
		return fmt.Errorf("%w: got %d", errUtils.ErrInvalidLimit, limit)
	}

	// Parse since date if provided.
	var sinceTime *time.Time
	if since != "" {
		t, err := time.Parse("2006-01-02", since)
		if err != nil {
			return fmt.Errorf("invalid date format for --since (expected YYYY-MM-DD): %w", err)
		}
		sinceTime = &t
	}

	// Fetch releases from GitHub.
	opts := utils.GitHubReleasesOptions{
		Owner:              "cloudposse",
		Repo:               "atmos",
		Limit:              limit,
		Offset:             offset,
		IncludePrereleases: includePrereleases,
		Since:              sinceTime,
	}

	releases, err := utils.GetGitHubRepoReleases(opts)
	if err != nil {
		return fmt.Errorf("failed to fetch releases: %w", err)
	}

	// Format output based on requested format.
	switch format {
	case "json":
		return outputJSON(releases)
	case "yaml":
		return outputYAML(releases)
	case "text":
		outputText(releases)
		return nil
	default:
		return fmt.Errorf("%w: %s (supported: text, json, yaml)", errUtils.ErrUnsupportedOutputFormat, format)
	}
}

func outputJSON(releases []*github.RepositoryRelease) error {
	type releaseInfo struct {
		Tag        string `json:"tag"`
		Name       string `json:"name"`
		Published  string `json:"published"`
		Prerelease bool   `json:"prerelease"`
		URL        string `json:"url"`
	}

	var output []releaseInfo
	for _, release := range releases {
		output = append(output, releaseInfo{
			Tag:        release.GetTagName(),
			Name:       release.GetName(),
			Published:  release.GetPublishedAt().Format(time.RFC3339),
			Prerelease: release.GetPrerelease(),
			URL:        release.GetHTMLURL(),
		})
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

func outputYAML(releases []*github.RepositoryRelease) error {
	type releaseInfo struct {
		Tag        string `yaml:"tag"`
		Name       string `yaml:"name"`
		Published  string `yaml:"published"`
		Prerelease bool   `yaml:"prerelease"`
		URL        string `yaml:"url"`
	}

	var output []releaseInfo
	for _, release := range releases {
		output = append(output, releaseInfo{
			Tag:        release.GetTagName(),
			Name:       release.GetName(),
			Published:  release.GetPublishedAt().Format(time.RFC3339),
			Prerelease: release.GetPrerelease(),
			URL:        release.GetHTMLURL(),
		})
	}

	encoder := yaml.NewEncoder(os.Stdout)
	return encoder.Encode(output)
}

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

func outputText(releases []*github.RepositoryRelease) {
	if len(releases) == 0 {
		fmt.Println("No releases found")
		return
	}

	for _, release := range releases {
		prerelease := ""
		if release.GetPrerelease() {
			prerelease = " (pre-release)"
		}
		fmt.Printf("%s - %s - %s%s\n",
			release.GetTagName(),
			release.GetPublishedAt().Format("2006-01-02"),
			getReleaseTitle(release),
			prerelease,
		)
	}
}
