package github

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"

	httpClient "github.com/cloudposse/atmos/pkg/http"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// ghCLITimeout is the maximum time to wait for `gh auth token` to respond.
	ghCLITimeout = 5 * time.Second
)

// ErrGitHubTokenRequired indicates that a GitHub token is required but not found.
var ErrGitHubTokenRequired = errors.New("GitHub token required")

// GetGitHubToken retrieves a GitHub token using multiple fallback strategies.
// The token is required for operations that need authentication (e.g., downloading PR artifacts).
//
// Detection order:
//  1. --github-token CLI flag (via viper, for toolchain commands)
//  2. ATMOS_GITHUB_TOKEN environment variable
//  3. GITHUB_TOKEN environment variable
//  4. `gh auth token` command output (if GitHub CLI is installed)
//
// Returns the token if found, or an empty string if no token is available.
// Use GetGitHubTokenOrError if you need to require authentication.
func GetGitHubToken() string {
	defer perf.Track(nil, "github.GetGitHubToken")()

	// First, try the standard Atmos token detection (CLI flag + env vars).
	if token := httpClient.GetGitHubTokenFromEnv(); token != "" {
		return token
	}

	// Fall back to GitHub CLI if installed.
	if token := getGitHubTokenFromCLI(); token != "" {
		log.Debug("Using GitHub token from gh CLI")
		return token
	}

	return ""
}

// GetGitHubTokenOrError retrieves a GitHub token or returns an error if none is found.
// Use this when authentication is required (e.g., downloading PR artifacts).
func GetGitHubTokenOrError() (string, error) {
	defer perf.Track(nil, "github.GetGitHubTokenOrError")()

	token := GetGitHubToken()
	if token == "" {
		return "", ErrGitHubTokenRequired
	}
	return token, nil
}

// getGitHubTokenFromCLI attempts to get a token from the GitHub CLI.
// Returns empty string if gh is not installed or not authenticated.
func getGitHubTokenFromCLI() string {
	defer perf.Track(nil, "github.getGitHubTokenFromCLI")()

	// Try to get token from gh CLI with timeout to prevent hanging.
	ctx, cancel := context.WithTimeout(context.Background(), ghCLITimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gh", "auth", "token")
	output, err := cmd.Output()
	if err != nil {
		// gh not installed or not authenticated - this is expected.
		log.Debug("gh auth token failed (gh CLI may not be installed or authenticated)", "error", err)
		return ""
	}

	token := strings.TrimSpace(string(output))
	if token == "" {
		return ""
	}

	return token
}
