package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

// repoAPIURL is the GitHub API endpoint for this repository's metadata,
// used to keep NOTICE's tagline in sync with the repo's actual GitHub
// description instead of a hand-maintained copy that can drift from it.
const repoAPIURL = "https://api.github.com/repos/cloudposse/atmos"

// fetchRepoDescription retrieves the repository description from the
// GitHub API at apiURL. apiURL is a parameter (rather than always
// repoAPIURL) so tests can point it at an httptest.Server.
func fetchRepoDescription(apiURL string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("build request for %s: %w", apiURL, err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if token := githubToken(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch %s: %w", apiURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch %s: unexpected status %s", apiURL, resp.Status)
	}

	var payload struct {
		Description string `json:"description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("decode response from %s: %w", apiURL, err)
	}
	if payload.Description == "" {
		return "", fmt.Errorf("%s: repository has no description", apiURL)
	}

	return payload.Description, nil
}

// githubToken returns a token to authenticate GitHub API requests with, so
// generation doesn't hit the unauthenticated API's 60 req/hour rate limit.
// GH_TOKEN takes precedence over GITHUB_TOKEN, matching the gh CLI's
// convention. Both are optional: anonymous requests still work for
// occasional local runs.
func githubToken() string {
	if token := os.Getenv("GH_TOKEN"); token != "" {
		return token
	}
	return os.Getenv("GITHUB_TOKEN")
}
