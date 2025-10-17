package exec

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/google/go-github/v59/github"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/utils"
)

const (
	versionShowBytesPerKB = 1024
	versionShowBytesPerMB = versionShowBytesPerKB * versionShowBytesPerKB
)

// ExecuteVersionShow displays details for a specific Atmos release.
func ExecuteVersionShow(
	atmosConfig *schema.AtmosConfiguration,
	version string,
	format string,
) error {
	defer perf.Track(atmosConfig, "exec.ExecuteVersionShow")()

	var release *github.RepositoryRelease
	var err error

	// Handle "latest" keyword.
	if strings.ToLower(version) == "latest" {
		release, err = utils.GetGitHubLatestRelease("cloudposse", "atmos")
	} else {
		release, err = utils.GetGitHubReleaseByTag("cloudposse", "atmos", version)
	}

	if err != nil {
		return fmt.Errorf("failed to fetch release: %w", err)
	}

	// Format output based on requested format.
	switch format {
	case "json":
		return outputShowJSON(release)
	case "yaml":
		return outputShowYAML(release)
	case "text":
		outputShowText(release)
		return nil
	default:
		return fmt.Errorf("%w: %s (supported: text, json, yaml)", errUtils.ErrUnsupportedOutputFormat, format)
	}
}

func outputShowJSON(release *github.RepositoryRelease) error {
	type assetInfo struct {
		Name        string `json:"name"`
		Size        int    `json:"size"`
		DownloadURL string `json:"download_url"`
	}

	type releaseDetail struct {
		Tag        string      `json:"tag"`
		Name       string      `json:"name"`
		Published  string      `json:"published"`
		Prerelease bool        `json:"prerelease"`
		Body       string      `json:"body"`
		URL        string      `json:"url"`
		Assets     []assetInfo `json:"assets"`
	}

	var assets []assetInfo
	for _, asset := range release.Assets {
		assets = append(assets, assetInfo{
			Name:        asset.GetName(),
			Size:        asset.GetSize(),
			DownloadURL: asset.GetBrowserDownloadURL(),
		})
	}

	output := releaseDetail{
		Tag:        release.GetTagName(),
		Name:       release.GetName(),
		Published:  release.GetPublishedAt().Format("2006-01-02 15:04:05 MST"),
		Prerelease: release.GetPrerelease(),
		Body:       release.GetBody(),
		URL:        release.GetHTMLURL(),
		Assets:     assets,
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

func outputShowYAML(release *github.RepositoryRelease) error {
	type assetInfo struct {
		Name        string `yaml:"name"`
		Size        int    `yaml:"size"`
		DownloadURL string `yaml:"download_url"`
	}

	type releaseDetail struct {
		Tag        string      `yaml:"tag"`
		Name       string      `yaml:"name"`
		Published  string      `yaml:"published"`
		Prerelease bool        `yaml:"prerelease"`
		Body       string      `yaml:"body"`
		URL        string      `yaml:"url"`
		Assets     []assetInfo `yaml:"assets"`
	}

	var assets []assetInfo
	for _, asset := range release.Assets {
		assets = append(assets, assetInfo{
			Name:        asset.GetName(),
			Size:        asset.GetSize(),
			DownloadURL: asset.GetBrowserDownloadURL(),
		})
	}

	output := releaseDetail{
		Tag:        release.GetTagName(),
		Name:       release.GetName(),
		Published:  release.GetPublishedAt().Format("2006-01-02 15:04:05 MST"),
		Prerelease: release.GetPrerelease(),
		Body:       release.GetBody(),
		URL:        release.GetHTMLURL(),
		Assets:     assets,
	}

	encoder := yaml.NewEncoder(os.Stdout)
	return encoder.Encode(output)
}

func outputShowText(release *github.RepositoryRelease) {
	fmt.Printf("Version: %s\n", release.GetTagName())
	fmt.Printf("Name: %s\n", release.GetName())
	fmt.Printf("Published: %s\n", release.GetPublishedAt().Format("2006-01-02 15:04:05 MST"))

	if release.GetPrerelease() {
		fmt.Println("Type: Pre-release")
	} else {
		fmt.Println("Type: Stable")
	}

	fmt.Printf("URL: %s\n", release.GetHTMLURL())
	fmt.Println()

	// Release notes.
	if body := release.GetBody(); body != "" {
		fmt.Println("Release Notes:")
		fmt.Println("─────────────────────────────────────────────────────────────────")
		fmt.Println(body)
		fmt.Println("─────────────────────────────────────────────────────────────────")
		fmt.Println()
	}

	// Assets.
	if len(release.Assets) > 0 {
		fmt.Println("Assets:")
		for _, asset := range release.Assets {
			sizeMB := float64(asset.GetSize()) / float64(versionShowBytesPerMB)
			fmt.Printf("  - %s (%.2f MB)\n", asset.GetName(), sizeMB)
			fmt.Printf("    %s\n", asset.GetBrowserDownloadURL())
		}
	}
}
