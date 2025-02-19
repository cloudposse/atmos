package gogetter

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// CustomGitHubDetector intercepts GitHub URLs and transforms them
// into something like git::https://<token>@github.com/... so we can
// do a git-based clone with a token.
type CustomGitHubDetector struct {
	AtmosConfig schema.AtmosConfiguration
}

// Detect implements the getter.Detector interface for go-getter v1.
func (d *CustomGitHubDetector) Detect(src, _ string) (string, bool, error) {
	if len(src) == 0 {
		return "", false, nil
	}

	if !strings.Contains(src, "://") {
		src = "https://" + src
	}

	parsedURL, err := url.Parse(src)
	if err != nil {
		u.LogDebug(fmt.Sprintf("Failed to parse URL %q: %v\n", src, err))
		return "", false, fmt.Errorf("failed to parse URL %q: %w", src, err)
	}

	if strings.ToLower(parsedURL.Host) != "github.com" {
		u.LogDebug(fmt.Sprintf("Host is %q, not 'github.com', skipping token injection\n", parsedURL.Host))
		return "", false, nil
	}

	parts := strings.SplitN(parsedURL.Path, "/", 4)
	if len(parts) < 3 {
		u.LogDebug(fmt.Sprintf("URL path %q doesn't look like /owner/repo\n", parsedURL.Path))
		return "", false, fmt.Errorf("invalid GitHub URL %q", parsedURL.Path)
	}

	atmosGitHubToken := os.Getenv("ATMOS_GITHUB_TOKEN")
	gitHubToken := os.Getenv("GITHUB_TOKEN")

	var usedToken string
	var tokenSource string

	// 1. If ATMOS_GITHUB_TOKEN is set, always use that
	if atmosGitHubToken != "" {
		usedToken = atmosGitHubToken
		tokenSource = "ATMOS_GITHUB_TOKEN"
		u.LogDebug("ATMOS_GITHUB_TOKEN is set\n")
	} else {
		// 2. Otherwise, only inject GITHUB_TOKEN if cfg.Settings.InjectGithubToken == true
		if d.AtmosConfig.Settings.InjectGithubToken && gitHubToken != "" {
			usedToken = gitHubToken
			tokenSource = "GITHUB_TOKEN"
			u.LogTrace("InjectGithubToken=true and GITHUB_TOKEN is set, using it\n")
		} else {
			u.LogTrace("No ATMOS_GITHUB_TOKEN or GITHUB_TOKEN found\n")
		}
	}

	if usedToken != "" {
		user := parsedURL.User.Username()
		pass, _ := parsedURL.User.Password()
		if user == "" && pass == "" {
			u.LogDebug(fmt.Sprintf("Injecting token from %s for %s\n", tokenSource, src))
			parsedURL.User = url.UserPassword("x-access-token", usedToken)
		} else {
			u.LogDebug("Credentials found, skipping token injection\n")
		}
	}

	finalURL := "git::" + parsedURL.String()

	return finalURL, true, nil
}
