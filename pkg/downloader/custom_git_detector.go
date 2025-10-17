package downloader

import (
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

const schemeSeparator = "://"

// CustomGitDetector intercepts Git URLs (for GitHub, Bitbucket, GitLab, etc.)
// and transforms them into a proper URL for cloning, optionally injecting tokens.
type CustomGitDetector struct {
	atmosConfig *schema.AtmosConfiguration
	source      string
}

func NewCustomGitDetector(atmosConfig *schema.AtmosConfiguration, source string) *CustomGitDetector {
	return &CustomGitDetector{
		atmosConfig: atmosConfig,
		source:      source,
	}
}

// Detect implements the getter.Detector interface for go-getter v1.
func (d *CustomGitDetector) Detect(src, _ string) (string, bool, error) {
	log.Debug("CustomGitDetector.Detect called")

	if len(src) == 0 {
		return "", false, nil
	}

	// Ensure the URL has an explicit scheme.
	src = d.ensureScheme(src)

	// Parse the URL to extract the host and path.
	parsedURL, err := url.Parse(src)
	if err != nil {
		maskedSrc, _ := maskBasicAuth(src)
		log.Debug("Failed to parse URL", keyURL, maskedSrc, "error", err)
		return "", false, fmt.Errorf("%w: %q: %w", errUtils.ErrParseURL, maskedSrc, err)
	}

	// If no host is detected, this is likely a local file path.
	// Skip custom processing so that go getter handles it as is.
	if parsedURL.Host == "" {
		log.Debug("No host detected in URL, skipping custom git detection", keyURL, src)
		return "", false, nil
	}

	// Normalize the path.
	d.normalizePath(parsedURL)

	// Adjust host check to support GitHub, Bitbucket, GitLab, etc.
	host := strings.ToLower(parsedURL.Host)
	if host != hostGitHub && host != hostBitbucket && host != hostGitLab {
		log.Debug("Skipping token injection for an unsupported host", "host", parsedURL.Host)
		return "", false, nil
	}

	log.Debug("Reading config param", "InjectGithubToken", d.atmosConfig.Settings.InjectGithubToken)
	// Inject token if available.
	d.injectToken(parsedURL, host)

	// Note: URI normalization (including adding //.) is now handled by normalizeVendorURI
	// in the vendor processing pipeline, so we don't need to adjust subdirectory here

	// Set "depth=1" for a shallow clone if not specified.
	q := parsedURL.Query()
	if _, exists := q["depth"]; !exists {
		q.Set("depth", "1")
	}
	parsedURL.RawQuery = q.Encode()

	finalURL := "git::" + parsedURL.String()
	maskedFinal, err := maskBasicAuth(strings.TrimPrefix(finalURL, "git::"))
	if err != nil {
		log.Debug("Masking failed", "error", err)
	} else {
		log.Debug("Final transformation", "url", "git::"+maskedFinal)
	}

	return finalURL, true, nil
}

const (
	// Named constants for regex match indices.
	matchIndexUser   = 1
	matchIndexHost   = 3
	matchIndexPath   = 4
	matchIndexSuffix = 5
	matchIndexExtra  = 6

	keyURL = "url"

	hostGitHub    = "github.com"
	hostGitLab    = "gitlab.com"
	hostBitbucket = "bitbucket.org"
)

const GitPrefix = "git::"

// ensureScheme checks for an explicit scheme and rewrites SCP-style URLs if needed.
// Also removes any existing "git::" prefix (required for the dry-run mode to operate correctly).
func (d *CustomGitDetector) ensureScheme(src string) string {
	// Strip any existing "git::" prefix
	src = strings.TrimPrefix(src, GitPrefix)

	if !strings.Contains(src, schemeSeparator) {
		if newSrc, rewritten := rewriteSCPURL(src); rewritten {
			maskedOld, _ := maskBasicAuth(src)
			maskedNew, _ := maskBasicAuth(newSrc)
			log.Debug("Rewriting SCP-style SSH URL", "old_url", maskedOld, "new_url", maskedNew)
			return newSrc
		}
		src = "https://" + src
		maskedSrc, _ := maskBasicAuth(src)
		log.Debug("Defaulting to https scheme", keyURL, maskedSrc)
	}
	return src
}

func rewriteSCPURL(src string) (string, bool) {
	scpPattern := regexp.MustCompile(`^(([\w.-]+)@)?([\w.-]+\.[\w.-]+):([\w./-]+)(\.git)?(.*)$`)
	if scpPattern.MatchString(src) {
		matches := scpPattern.FindStringSubmatch(src)
		newSrc := "ssh://"
		user := matches[matchIndexUser] // This includes the "@" if present.
		host := matches[matchIndexHost]
		// Only for SSH vendoring (i.e. when rewriting an SCP URL), inject default username (git) for known hosts.
		if user == "" && (strings.EqualFold(host, hostGitHub) ||
			strings.EqualFold(host, hostGitLab) ||
			strings.EqualFold(host, hostBitbucket)) {
			user = "git@"
		}
		newSrc += user + host + "/" + matches[matchIndexPath]
		if matches[matchIndexSuffix] != "" {
			newSrc += matches[matchIndexSuffix]
		}
		if matches[matchIndexExtra] != "" {
			newSrc += matches[matchIndexExtra]
		}
		return newSrc, true
	}
	return "", false
}

// normalizePath converts the URL path to use forward slashes.
func (d *CustomGitDetector) normalizePath(parsedURL *url.URL) {
	unescapedPath, err := url.PathUnescape(parsedURL.Path)
	if err == nil {
		parsedURL.Path = filepath.ToSlash(unescapedPath)
	} else {
		parsedURL.Path = filepath.ToSlash(parsedURL.Path)
	}
}

// injectToken injects a token into the URL if available.
func (d *CustomGitDetector) injectToken(parsedURL *url.URL, host string) {
	token, tokenSource := d.resolveToken(host)
	if token != "" {
		defaultUsername := d.getDefaultUsername(host)
		parsedURL.User = url.UserPassword(defaultUsername, token)
		maskedURL, _ := maskBasicAuth(parsedURL.String())
		log.Debug("Injected token", "env", tokenSource, keyURL, maskedURL)
	} else {
		log.Debug("No token found for injection")
	}
}

// resolveToken returns the token and its source based on the host.
// It prefers ATMOS_* prefixed tokens but falls back to standard tokens if not set.
func (d *CustomGitDetector) resolveToken(host string) (string, string) {
	switch host {
	case hostGitHub:
		// Try ATMOS_GITHUB_TOKEN first, fall back to GITHUB_TOKEN
		if d.atmosConfig.Settings.AtmosGithubToken != "" {
			return d.atmosConfig.Settings.AtmosGithubToken, "ATMOS_GITHUB_TOKEN"
		}
		return d.atmosConfig.Settings.GithubToken, "GITHUB_TOKEN"
	case hostBitbucket:
		// Try ATMOS_BITBUCKET_TOKEN first, fall back to BITBUCKET_TOKEN
		if d.atmosConfig.Settings.AtmosBitbucketToken != "" {
			return d.atmosConfig.Settings.AtmosBitbucketToken, "ATMOS_BITBUCKET_TOKEN"
		}
		return d.atmosConfig.Settings.BitbucketToken, "BITBUCKET_TOKEN"
	case hostGitLab:
		// Try ATMOS_GITLAB_TOKEN first, fall back to GITLAB_TOKEN
		if d.atmosConfig.Settings.AtmosGitlabToken != "" {
			return d.atmosConfig.Settings.AtmosGitlabToken, "ATMOS_GITLAB_TOKEN"
		}
		return d.atmosConfig.Settings.GitlabToken, "GITLAB_TOKEN"
	}
	return "", ""
}

// getDefaultUsername returns the default username for token injection based on the host.
func (d *CustomGitDetector) getDefaultUsername(host string) string {
	switch host {
	case hostGitHub:
		return "x-access-token"
	case hostGitLab:
		return "oauth2"
	case hostBitbucket:
		defaultUsername := d.atmosConfig.Settings.BitbucketUsername
		if defaultUsername == "" {
			return "x-token-auth"
		}
		return defaultUsername
	default:
		return "x-access-token"
	}
}
