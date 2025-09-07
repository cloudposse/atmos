package toolchain

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/spf13/viper"
)

// GitHubAPI defines the interface for GitHub API operations.
type GitHubAPI interface {
	FetchReleases(owner, repo string, limit int) ([]string, error)
}

// GitHubAPIClient implements GitHubAPI with real HTTP calls.
type GitHubAPIClient struct {
	client  *http.Client
	baseURL string
}

// NewGitHubAPIClient creates a new GitHub API client.
func NewGitHubAPIClient() *GitHubAPIClient {
	return &GitHubAPIClient{
		client:  &http.Client{},
		baseURL: "https://api.github.com",
	}
}

// NewGitHubAPIClientWithBaseURL creates a new GitHub API client with a custom base URL (for testing).
func NewGitHubAPIClientWithBaseURL(baseURL string) *GitHubAPIClient {
	return &GitHubAPIClient{
		client:  &http.Client{},
		baseURL: baseURL,
	}
}

// FetchReleases fetches all available versions from GitHub API.
func (g *GitHubAPIClient) FetchReleases(owner, repo string, limit int) ([]string, error) {
	// GitHub API endpoint for releases with per_page parameter
	apiURL := fmt.Sprintf("%s/repos/%s/%s/releases?per_page=%d", g.baseURL, owner, repo, limit)

	// Get GitHub token for authenticated requests
	token := viper.GetString("github-token")

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add GitHub token if available
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := g.client.Do(req)
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
		TagName    string `json:"tag_name"`
		Prerelease bool   `json:"prerelease"`
	}

	if err := json.Unmarshal(body, &releases); err != nil {
		return nil, fmt.Errorf("failed to parse releases JSON: %w", err)
	}

	// Extract all non-prerelease versions
	var versions []string
	for _, release := range releases {
		if !release.Prerelease {
			// Remove 'v' prefix if present
			version := strings.TrimPrefix(release.TagName, "v")
			versions = append(versions, version)
		}
	}

	if len(versions) == 0 {
		return nil, fmt.Errorf("no non-prerelease versions found for %s/%s", owner, repo)
	}

	return versions, nil
}

// Global GitHub API client instance.
var defaultGitHubAPI GitHubAPI = NewGitHubAPIClient()

// SetGitHubAPI sets the global GitHub API client (for testing).
func SetGitHubAPI(api GitHubAPI) {
	defaultGitHubAPI = api
}

// ResetGitHubAPI resets the global GitHub API client to the default.
func ResetGitHubAPI() {
	defaultGitHubAPI = NewGitHubAPIClient()
}

// fetchAllGitHubVersions is the public function that uses the global GitHub API client.
func fetchAllGitHubVersions(owner, repo string, limit int) ([]string, error) {
	return defaultGitHubAPI.FetchReleases(owner, repo, limit)
}
