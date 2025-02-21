package downloader

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Predefined errors for better reuse and static analysis.
var (
	ErrInvalidGitHubURL = errors.New("invalid GitHub URL")
	ErrURLEmpty         = errors.New("source URL is empty")
	ErrFailedParseURL   = errors.New("failed to parse URL")
)

type EnvProvider func(key string) string

// Logger interface allows logging abstraction for testing.
type Logger interface {
	Debug(any, ...any)
}

// customGitHubDetector detects and modifies GitHub URLs.
type customGitHubDetector struct {
	AtmosConfig *schema.AtmosConfiguration
	GetEnv      EnvProvider
	Log         Logger
}

// NewCustomGitHubDetector initializes a new detector with default dependencies.
func NewCustomGitHubDetector(config *schema.AtmosConfiguration) *customGitHubDetector {
	return &customGitHubDetector{
		AtmosConfig: config,
		GetEnv:      os.Getenv,
		Log:         log.Default(),
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
		d.Log.Debug(fmt.Sprintf("Failed to parse URL %q: %v", src, err))
		return "", false, fmt.Errorf("failed to parse URL %q: %w", src, err)
	}

	if strings.ToLower(parsedURL.Host) != "github.com" {
		d.Log.Debug(fmt.Sprintf("Host is %q, not 'github.com', skipping token injection", parsedURL.Host))
		return "", false, nil
	}

	// Ensure the URL follows the /owner/repo format
	parts := strings.SplitN(parsedURL.Path, "/", 4)
	if len(parts) < 3 {
		d.Log.Debug(fmt.Sprintf("URL path %q doesn't look like /owner/repo", parsedURL.Path))
		return "", false, ErrInvalidGitHubURL
	}

	// Get the authentication token
	usedToken, tokenSource := d.getGitHubToken()
	if usedToken != "" {
		user := parsedURL.User.Username()
		pass, _ := parsedURL.User.Password()

		if user == "" && pass == "" {
			d.Log.Debug(fmt.Sprintf("Injecting token from %s for %s", tokenSource, src))

			parsedURL.User = url.UserPassword("x-access-token", usedToken)
		} else {
			d.Log.Debug("Credentials found in URL, skipping token injection")
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
		d.Log.Debug("Using ATMOS_GITHUB_TOKEN")
		return atmosGitHubToken, "ATMOS_GITHUB_TOKEN"
	}

	if d.AtmosConfig.Settings.InjectGithubToken && gitHubToken != "" {
		d.Log.Debug("Using GITHUB_TOKEN (InjectGithubToken=true)")
		return gitHubToken, "GITHUB_TOKEN"
	}

	d.Log.Debug("No valid GitHub token found")

	return "", ""
}
