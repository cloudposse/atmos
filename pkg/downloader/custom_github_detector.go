package downloader

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	log "github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Predefined errors for better reuse and static analysis.
var (
	ErrInvalidGitHubURL = errors.New("invalid GitHub URL")
	ErrURLEmpty         = errors.New("source URL is empty")
	ErrFailedParseURL   = errors.New("failed to parse URL")
)

type EnvProvider func(key string) string

// customGitHubDetector detects and modifies GitHub URLs.
type customGitHubDetector struct {
	AtmosConfig *schema.AtmosConfiguration
	GetEnv      EnvProvider
}

// NewCustomGitHubDetector initializes a new detector with default dependencies.
func NewCustomGitHubDetector(config *schema.AtmosConfiguration) *customGitHubDetector {
	return &customGitHubDetector{
		AtmosConfig: config,
		GetEnv:      os.Getenv,
	}
}

// Detect checks if a URL is a GitHub repo and injects authentication if needed.
func (d *customGitHubDetector) Detect(src, _ string) (string, bool, error) {
	if src == "" {
		return "", false, ErrURLEmpty
	}

	if !strings.Contains(src, "://") {
		src = "https://" + src
	}

	parsedURL, err := url.Parse(src)
	if err != nil {
		log.Debug("Failed to parse URL", "source", src, "error", err)
		return "", false, fmt.Errorf("failed to parse URL %q: %w", src, err)
	}

	if strings.ToLower(parsedURL.Host) != "github.com" {
		log.Debug("Host is not 'github.com', skipping token injection", "host", parsedURL.Host)
		return "", false, nil
	}

	// Ensure the URL follows the /owner/repo format
	parts := strings.SplitN(parsedURL.Path, "/", 4)
	if len(parts) < 3 {
		log.Debug("URL path doesn't look like /owner/repo", "url path", parsedURL.Path)
		return "", false, ErrInvalidGitHubURL
	}

	// Get the authentication token
	usedToken, tokenSource := d.getGitHubToken()
	if usedToken != "" {
		user := parsedURL.User.Username()
		pass, _ := parsedURL.User.Password()

		if user == "" && pass == "" {
			log.Debug("Injecting token", "from", tokenSource, "to", src)

			parsedURL.User = url.UserPassword("x-access-token", usedToken)
		} else {
			log.Debug("Credentials found in URL, skipping token injection")
		}
	}

	finalURL := "git::" + parsedURL.String()

	return finalURL, true, nil
}

// getGitHubToken selects the appropriate GitHub token based on the configuration.
func (d *customGitHubDetector) getGitHubToken() (string, string) {
	atmosGitHubToken := d.GetEnv("ATMOS_GITHUB_TOKEN")
	gitHubToken := d.GetEnv("GITHUB_TOKEN")

	if atmosGitHubToken != "" {
		log.Debug("Using ATMOS_GITHUB_TOKEN")
		return atmosGitHubToken, "ATMOS_GITHUB_TOKEN"
	}

	if d.AtmosConfig.Settings.InjectGithubToken && gitHubToken != "" {
		log.Debug("Using GITHUB_TOKEN (InjectGithubToken=true)")
		return gitHubToken, "GITHUB_TOKEN"
	}

	log.Debug("No valid GitHub token found for injection")

	return "", ""
}
