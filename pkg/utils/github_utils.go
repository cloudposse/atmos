package utils

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/google/go-github/v59/github"
)

// GetLatestGitHubRepoRelease returns the latest release tag for a GitHub repository
func GetLatestGitHubRepoRelease(owner string, repo string) (string, error) {
	opt := &github.ListOptions{Page: 1, PerPage: 1}
	client := github.NewClient(nil)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()

	releases, _, err := client.Repositories.ListReleases(ctx, owner, repo, opt)
	if err != nil {
		return "", err
	}

	if len(releases) > 0 {
		latestRelease := releases[0]
		latestReleaseTag := *latestRelease.TagName
		return latestReleaseTag, nil
	}

	return "", nil
}

// ParseGitHubURL parses a GitHub URL and returns the owner, repo, file path and branch
func DownloadFileFromGitHub(rawURL string) ([]byte, error) {
	owner, repo, filePath, branch, err := ParseGitHubURL(rawURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse GitHub URL: %w", err)
	}

	githubToken := os.Getenv("GITHUB_TOKEN")
	if githubToken == "" {
		return nil, fmt.Errorf("GITHUB_TOKEN is not set")
	}

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s?ref=%s", owner, repo, filePath, branch)
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+githubToken)
	req.Header.Set("Accept", "application/vnd.github.v3.raw")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to perform request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download file: %s", resp.Status)
	}

	return io.ReadAll(resp.Body)
}
