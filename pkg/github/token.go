package github

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	execpkg "github.com/cloudposse/atmos/pkg/exec"
	httpClient "github.com/cloudposse/atmos/pkg/http"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// ghCLITimeout is the maximum time to wait for `gh auth token` to respond.
	ghCLITimeout = 5 * time.Second

	// Default GitHub CLI binary used to obtain a token.
	defaultGitHubCLI = "gh"
)

// commander executes the GitHub CLI. It defaults to the standard library
// implementation and is overridable in tests to avoid spawning a real process.
var commander execpkg.CommandExecutor = execpkg.Default()

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

	// No token from any source - GitHub requests will be made anonymously.
	log.Debug("No GitHub token resolved; using anonymous (unauthenticated) GitHub access (subject to rate limits)")
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
// Returns empty string if the CLI is not installed, not authenticated, or disabled.
//
// The CLI binary is configurable via the ATMOS_GITHUB_CLI environment variable
// (defaults to "gh"). Setting it to an empty value disables the fallback, and
// setting it to a nonexistent binary forces the unauthenticated/anonymous path
// (useful for exercising public access).
func getGitHubTokenFromCLI() string {
	defer perf.Track(nil, "github.getGitHubTokenFromCLI")()

	cli := gitHubCLIBinary()
	if cli == "" {
		log.Debug("gh CLI token fallback disabled (ATMOS_GITHUB_CLI is empty)")
		return ""
	}

	// Try to get token from the GitHub CLI with timeout to prevent hanging.
	ctx, cancel := context.WithTimeout(context.Background(), ghCLITimeout)
	defer cancel()
	cmd := commander.CommandContext(ctx, cli, "auth", "token")
	output, err := cmd.Output()
	if err != nil {
		// CLI not installed, not authenticated, or redirected to a missing binary - this is expected.
		log.Debug("GitHub CLI token lookup failed (CLI may not be installed or authenticated)", "cli", cli, "error", err)
		return ""
	}

	token := strings.TrimSpace(string(output))
	if token == "" {
		return ""
	}

	return token
}

// gitHubCLIBinary returns the GitHub CLI binary name to use for token lookups.
// It honors the ATMOS_GITHUB_CLI environment variable and defaults to "gh".
// An explicitly empty value disables the CLI fallback.
func gitHubCLIBinary() string {
	cli, set := os.LookupEnv("ATMOS_GITHUB_CLI")
	if !set {
		return defaultGitHubCLI
	}
	return strings.TrimSpace(cli)
}
