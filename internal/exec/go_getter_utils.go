// https://github.com/hashicorp/go-getter

package exec

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	l "github.com/charmbracelet/log"
	"github.com/google/uuid"
	"github.com/hashicorp/go-getter"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ValidateURI validates URIs
func ValidateURI(uri string) error {
	if uri == "" {
		return fmt.Errorf("URI cannot be empty")
	}
	// Maximum length check
	if len(uri) > 2048 {
		return fmt.Errorf("URI exceeds maximum length of 2048 characters")
	}
	// Validate URI format
	if strings.Contains(uri, "..") {
		return fmt.Errorf("URI cannot contain path traversal sequences")
	}
	if strings.Contains(uri, " ") {
		return fmt.Errorf("URI cannot contain spaces")
	}
	// Validate scheme-specific format
	if strings.HasPrefix(uri, "oci://") {
		if !strings.Contains(uri[6:], "/") {
			return fmt.Errorf("invalid OCI URI format")
		}
	} else if strings.Contains(uri, "://") {
		scheme := strings.Split(uri, "://")[0]
		if !IsValidScheme(scheme) {
			return fmt.Errorf("unsupported URI scheme: %s", scheme)
		}
	}
	return nil
}

// IsValidScheme checks if the URL scheme is valid
func IsValidScheme(scheme string) bool {
	validSchemes := map[string]bool{
		"http":       true,
		"https":      true,
		"git":        true,
		"ssh":        true,
		"git::https": true,
		"git::ssh":   true,
	}
	return validSchemes[scheme]
}

// CustomGitDetector intercepts Git URLs (for GitHub, Bitbucket, GitLab, etc.)
// and transforms them into a proper URL for cloning, optionally injecting tokens.
type CustomGitDetector struct {
	AtmosConfig schema.AtmosConfiguration
	source      string
}

// Detect implements the getter.Detector interface for go-getter v1.
func (d *CustomGitDetector) Detect(src, _ string) (string, bool, error) {
	l.Debug("CustomGitDetector.Detect called")
	if len(src) == 0 {
		return "", false, nil
	}

	// Rewrite SCP-style URLs or default to https scheme.
	var err error
	src, err = rewriteURL(src)
	if err != nil {
		return "", false, err
	}

	// Parse the URL and normalize its path.
	parsedURL, err := parseAndNormalizeURL(src)
	if err != nil {
		return "", false, err
	}

	// Adjust host check to support GitHub, Bitbucket, GitLab, etc.
	host := strings.ToLower(parsedURL.Host)
	if host != "github.com" && host != "bitbucket.org" && host != "gitlab.com" {
		l.Debug("Skipping token injection for a unsupported host", "host", parsedURL.Host)
	}

	l.Debug("Reading config param", "InjectGithubToken", d.AtmosConfig.Settings.InjectGithubToken)

	// Inject token if available.
	injectToken(parsedURL, host, d.AtmosConfig.Settings.InjectGithubToken)

	// Normalize d.source for Windows path separators and append subdir if needed.
	appendSubdirIfNeeded(parsedURL, d.source)

	// Set "depth=1" for a shallow clone if not specified.
	setShallowCloneDepth(parsedURL)

	finalURL := "git::" + parsedURL.String()
	urlForMasking := strings.TrimPrefix(finalURL, "git::")
	maskedFinal, err := u.MaskBasicAuth(urlForMasking)
	if err != nil {
		l.Debug("Masking failed", "error", err)
	} else {
		l.Debug("Final URL", "final_url", "git::"+maskedFinal)
	}

	return finalURL, true, nil
}

// rewriteURL rewrites SCP-style URLs into proper SSH URLs or prepends "https://" if no scheme is found.
func rewriteURL(src string) (string, error) {
	// We need this block because many SCP-style URLs aren’t valid according to Go’s URL parser.
	// SCP-style URLs omit an explicit scheme (like "ssh://" or "https://") and use a colon
	// to separate the host from the path. Go’s URL parser expects a scheme, so without one,
	// it fails to parse these URLs correctly.
	// Below, we check if the URL doesn’t contain a scheme. If so, we attempt to detect an SCP-style URL:
	// e.g. "git@github.com:cloudposse/terraform-null-label.git?ref=..."
	// If the URL matches this pattern, we rewrite it to a proper SSH URL.
	// Otherwise, we default to prepending "https://".
	if !strings.Contains(src, "://") {
		scpPattern := regexp.MustCompile(`^(([\w.-]+)@)?([\w.-]+\.[\w.-]+):([\w./-]+)(\.git)?(.*)$`)
		if scpPattern.MatchString(src) {
			matches := scpPattern.FindStringSubmatch(src)
			newSrc := "ssh://"
			if matches[1] != "" {
				newSrc += matches[1] // includes username and '@'
			}
			newSrc += matches[3] + "/" + matches[4]
			if matches[5] != "" {
				newSrc += matches[5]
			}
			if matches[6] != "" {
				newSrc += matches[6]
			}
			maskedOld, _ := u.MaskBasicAuth(src)
			maskedNew, _ := u.MaskBasicAuth(newSrc)
			l.Debug("Rewriting SCP-style SSH URL", "old_url", maskedOld, "new_url", maskedNew)
			return newSrc, nil
		}
		src = "https://" + src
		maskedSrc, _ := u.MaskBasicAuth(src)
		l.Debug("Defaulting to https scheme", "url", maskedSrc)
	}
	return src, nil
}

// parseAndNormalizeURL parses the URL and normalizes Windows path separators and URL-encoded backslashes.
func parseAndNormalizeURL(src string) (*url.URL, error) {
	parsedURL, err := url.Parse(src)
	if err != nil {
		maskedSrc, _ := u.MaskBasicAuth(src)
		l.Debug("Failed to parse URL", "url", maskedSrc, "error", err)
		return nil, fmt.Errorf("failed to parse URL %q: %w", maskedSrc, err)
	}
	unescapedPath, err := url.PathUnescape(parsedURL.Path)
	if err == nil {
		parsedURL.Path = filepath.ToSlash(unescapedPath)
	} else {
		parsedURL.Path = filepath.ToSlash(parsedURL.Path)
	}
	return parsedURL, nil
}

// injectToken injects authentication token into the URL if available.
func injectToken(parsedURL *url.URL, host string, injectGithub bool) {
	var token, tokenSource string
	switch host {
	case "github.com":
		if injectGithub {
			tokenSource = "ATMOS_GITHUB_TOKEN"
			token = os.Getenv(tokenSource)
			if token == "" {
				tokenSource = "GITHUB_TOKEN"
				token = os.Getenv(tokenSource)
			}
		} else {
			tokenSource = "GITHUB_TOKEN"
			token = os.Getenv(tokenSource)
		}
	case "bitbucket.org":
		tokenSource = "BITBUCKET_TOKEN"
		token = os.Getenv(tokenSource)
		if token == "" {
			tokenSource = "ATMOS_BITBUCKET_TOKEN"
			token = os.Getenv(tokenSource)
		}
	case "gitlab.com":
		tokenSource = "GITLAB_TOKEN"
		token = os.Getenv(tokenSource)
		if token == "" {
			tokenSource = "ATMOS_GITLAB_TOKEN"
			token = os.Getenv(tokenSource)
		}
	}
	if token != "" {
		var defaultUsername string
		switch host {
		case "github.com":
			defaultUsername = "x-access-token"
		case "gitlab.com":
			defaultUsername = "oauth2"
		case "bitbucket.org":
			defaultUsername = os.Getenv("ATMOS_BITBUCKET_USERNAME")
			if defaultUsername == "" {
				defaultUsername = os.Getenv("BITBUCKET_USERNAME")
				if defaultUsername == "" {
					defaultUsername = "x-token-auth"
				}
			}
			l.Debug("Using Bitbucket username", "username", defaultUsername)
		default:
			defaultUsername = "x-access-token"
		}
		parsedURL.User = url.UserPassword(defaultUsername, token)
		maskedURL, _ := u.MaskBasicAuth(parsedURL.String())
		l.Debug("Injected token", "env", tokenSource, "url", maskedURL)
	} else {
		l.Debug("No token found for injection")
	}
}

// appendSubdirIfNeeded appends '//.' to the URL path if d.source is provided and indicates no subdirectory.
func appendSubdirIfNeeded(parsedURL *url.URL, source string) {
	normalizedSource := filepath.ToSlash(source)
	if normalizedSource != "" && !strings.Contains(normalizedSource, "//") {
		parts := strings.SplitN(parsedURL.Path, "/", 4)
		if strings.HasSuffix(parsedURL.Path, ".git") || len(parts) == 3 {
			maskedSrc, _ := u.MaskBasicAuth(parsedURL.String())
			l.Debug("Detected top-level repo with no subdir: appending '//.'", "url", maskedSrc)
			parsedURL.Path += "//."
		}
	}
}

// setShallowCloneDepth sets "depth=1" in the URL query if not already specified.
func setShallowCloneDepth(parsedURL *url.URL) {
	q := parsedURL.Query()
	if _, exists := q["depth"]; !exists {
		q.Set("depth", "1")
	}
	parsedURL.RawQuery = q.Encode()
}

// RegisterCustomDetectors prepends the custom detector so it runs before
// the built-in ones. Any code that calls go-getter should invoke this.
func RegisterCustomDetectors(atmosConfig schema.AtmosConfiguration) {
	getter.Detectors = append(
		[]getter.Detector{
			&CustomGitDetector{AtmosConfig: atmosConfig},
		},
		getter.Detectors...,
	)
}

// GoGetterGet downloads packages (files and folders) from different sources using `go-getter` and saves them into the destination
func GoGetterGet(
	atmosConfig schema.AtmosConfiguration,
	src string,
	dest string,
	clientMode getter.ClientMode,
	timeout time.Duration,
) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Register custom detectors
	RegisterCustomDetectors(atmosConfig)

	client := &getter.Client{
		Ctx: ctx,
		Src: src,
		// Destination where the files will be stored. This will create the directory if it doesn't exist
		Dst:  dest,
		Mode: clientMode,
	}

	if err := client.Get(); err != nil {
		return err
	}

	return nil
}

// DownloadDetectFormatAndParseFile downloads a remote file, detects the format of the file (JSON, YAML, HCL) and parses the file into a Go type
func DownloadDetectFormatAndParseFile(atmosConfig schema.AtmosConfiguration, file string) (any, error) {
	tempDir := os.TempDir()
	f := filepath.Join(tempDir, uuid.New().String())

	if err := GoGetterGet(atmosConfig, file, f, getter.ClientModeFile, time.Second*30); err != nil {
		return nil, fmt.Errorf("failed to download the file '%s': %w", file, err)
	}

	res, err := u.DetectFormatAndParseFile(f)
	if err != nil {
		return nil, fmt.Errorf("failed to parse file '%s': %w", file, err)
	}

	return res, nil
}

/*
Supported schemes:

file, dir, tar, zip
http, https
git, hg
s3, gcs
oci
scp, sftp
Shortcuts like github.com, bitbucket.org

- File-related Schemes:
file - Local filesystem paths
dir - Local directories
tar - Tar files, potentially compressed (tar.gz, tar.bz2, etc.)
zip - Zip files

- HTTP/HTTPS:
http - HTTP URLs
https - HTTPS URLs

- Git:
git - Git repositories, which can be accessed via HTTPS or SSH

- Mercurial:
hg - Mercurial repositories, accessed via HTTP/S or SSH

- Amazon S3:
s3 - Amazon S3 bucket URLs

- Google Cloud Storage:
gcs - Google Cloud Storage URLs

- OCI:
oci - Open Container Initiative (OCI) images

- Other Protocols:
scp - Secure Copy Protocol for SSH-based transfers
sftp - SSH File Transfer Protocol

- GitHub/Bitbucket/Other Shortcuts:
github.com - Direct GitHub repository shortcuts
bitbucket.org - Direct Bitbucket repository shortcuts

- Composite Schemes:
go-getter allows for composite schemes, where multiple operations can be combined. For example:
git::https://github.com/user/repo - Forces the use of git over an HTTPS URL.
tar::http://example.com/archive.tar.gz - Treats the HTTP resource as a tarball.

*/
