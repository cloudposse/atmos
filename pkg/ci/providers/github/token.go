package github

import (
	"os"

	errUtils "github.com/cloudposse/atmos/errors"
	pkgGitHub "github.com/cloudposse/atmos/pkg/github"
	"github.com/cloudposse/atmos/pkg/perf"
)

// GetCIGitHubToken retrieves a GitHub token for CI operations (commit statuses, artifacts).
// It checks ATMOS_CI_GITHUB_TOKEN first, then falls back to the standard GetGitHubToken chain.
// This allows CI operations to use a dedicated token while Terraform uses a different one.
//
// Detection order:
//  1. ATMOS_CI_GITHUB_TOKEN environment variable (CI-specific override)
//  2. Standard GetGitHubToken chain (--github-token flag, ATMOS_GITHUB_TOKEN, GITHUB_TOKEN, GH_TOKEN, gh auth token)
//
// Returns the token if found, or an empty string if no token is available.
func GetCIGitHubToken() string {
	defer perf.Track(nil, "github.GetCIGitHubToken")()

	if token := os.Getenv("ATMOS_CI_GITHUB_TOKEN"); token != "" {
		return token
	}

	return pkgGitHub.GetGitHubToken()
}

// GetCIGitHubTokenOrError retrieves a CI GitHub token or returns an error if none is found.
// Use this when authentication is required for CI operations.
func GetCIGitHubTokenOrError() (string, error) {
	defer perf.Track(nil, "github.GetCIGitHubTokenOrError")()

	token := GetCIGitHubToken()
	if token == "" {
		return "", errUtils.ErrGitHubTokenNotFound
	}

	return token, nil
}
