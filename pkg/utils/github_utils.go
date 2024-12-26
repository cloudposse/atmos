package utils

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"time"

	"github.com/google/go-github/v59/github"
	"golang.org/x/oauth2"
)

// newGitHubClient creates a new GitHub client. If a token is provided, it returns an authenticated client;
// otherwise, it returns an unauthenticated client.
func newGitHubClient(ctx context.Context) *github.Client {
	githubToken := os.Getenv("GITHUB_TOKEN")
	if githubToken == "" {
		return github.NewClient(nil)
	}

	// Token found, create an authenticated client
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: githubToken},
	)
	tc := oauth2.NewClient(ctx, ts)

	return github.NewClient(tc)
}

// GetLatestGitHubRepoRelease returns the latest release tag for a GitHub repository.
func GetLatestGitHubRepoRelease(owner string, repo string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	client := newGitHubClient(ctx)
	opt := &github.ListOptions{Page: 1, PerPage: 1}

	releases, _, err := client.Repositories.ListReleases(ctx, owner, repo, opt)
	if err != nil {
		return "", fmt.Errorf("failed to list releases: %w", err)
	}

	if len(releases) > 0 && releases[0].TagName != nil {
		return *releases[0].TagName, nil
	}

	return "", nil
}

// DownloadFileFromGitHub downloads a file from a GitHub repository using the GitHub API.
func DownloadFileFromGitHub(rawURL string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	owner, repo, filePath, branch, err := ParseGitHubURL(rawURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse GitHub URL: %w", err)
	}

	client := newGitHubClient(ctx)

	// Get the file content
	opt := &github.RepositoryContentGetOptions{Ref: branch}
	fileContent, _, _, err := client.Repositories.GetContents(ctx, owner, repo, filePath, opt)
	if err != nil {
		return nil, fmt.Errorf("failed to get file content from GitHub: %w", err)
	}
	if fileContent == nil {
		return nil, fmt.Errorf("no content returned for the requested file")
	}

	// Decode the base64 encoded content
	content, err := fileContent.GetContent()
	if err != nil {
		return nil, fmt.Errorf("failed to get file content: %w", err)
	}
	data, err := base64.StdEncoding.DecodeString(content)
	if err != nil {
		// fallback to raw content
		data = []byte(content)
	}

	return data, nil
}
