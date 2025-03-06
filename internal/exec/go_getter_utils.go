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

	log "github.com/charmbracelet/log"
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

// CustomGitDetector intercepts GitHub URLs and transforms them into something like git::https://<token>@github.com/ so we can do a git-based clone with a token.
type CustomGitDetector struct {
	AtmosConfig *schema.AtmosConfiguration
	source      string
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
		maskedSrc, _ := u.MaskBasicAuth(src)
		log.Debug("Failed to parse URL", keyURL, maskedSrc, "error", err)
		return "", false, fmt.Errorf("failed to parse URL %q: %w", maskedSrc, err)
	}

	// Normalize the path.
	d.normalizePath(parsedURL)

	// Adjust host check to support GitHub, Bitbucket, GitLab, etc.
	host := strings.ToLower(parsedURL.Host)
	if host != "github.com" && host != "bitbucket.org" && host != "gitlab.com" {
		log.Debug("Skipping token injection for a unsupported host", "host", parsedURL.Host)
	}

	log.Debug("Reading config param", "InjectGithubToken", d.AtmosConfig.Settings.InjectGithubToken)
	// Inject token if available.
	d.injectToken(parsedURL, host)

	// Adjust subdirectory if needed.
	d.adjustSubdir(parsedURL, d.source)

	// Set "depth=1" for a shallow clone if not specified.
	q := parsedURL.Query()
	if _, exists := q["depth"]; !exists {
		q.Set("depth", "1")
	}
	parsedURL.RawQuery = q.Encode()

	finalURL := "git::" + parsedURL.String()
	maskedFinal, err := u.MaskBasicAuth(strings.TrimPrefix(finalURL, "git::"))
	if err != nil {
		log.Debug("Masking failed", "error", err)
	} else {
		log.Debug("Final URL", "final_url", "git::"+maskedFinal)
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

	// Key for logging repeated "url" field.
	keyURL = "url"
)

// ensureScheme checks for an explicit scheme and rewrites SCP-style URLs if needed.
// This version no longer returns an error since it never produces one.
func (d *CustomGitDetector) ensureScheme(src string) string {
	if !strings.Contains(src, "://") {
		if newSrc, rewritten := rewriteSCPURL(src); rewritten {
			maskedOld, _ := u.MaskBasicAuth(src)
			maskedNew, _ := u.MaskBasicAuth(newSrc)
			log.Debug("Rewriting SCP-style SSH URL", "old_url", maskedOld, "new_url", maskedNew)
			return newSrc
		}
		src = "https://" + src
		maskedSrc, _ := u.MaskBasicAuth(src)
		log.Debug("Defaulting to https scheme", keyURL, maskedSrc)
	}
	return src
}

// rewriteSCPURL rewrites SCP-style URLs to a proper SSH URL if they match the expected pattern.
// Returns the rewritten URL and a boolean indicating if rewriting occurred.
func rewriteSCPURL(src string) (string, bool) {
	scpPattern := regexp.MustCompile(`^(([\w.-]+)@)?([\w.-]+\.[\w.-]+):([\w./-]+)(\.git)?(.*)$`)
	if scpPattern.MatchString(src) {
		matches := scpPattern.FindStringSubmatch(src)
		newSrc := "ssh://"
		if matches[matchIndexUser] != "" {
			newSrc += matches[matchIndexUser] // includes username and '@'
		}
		newSrc += matches[matchIndexHost] + "/" + matches[matchIndexPath]
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
		defaultUsername := getDefaultUsername(host)
		parsedURL.User = url.UserPassword(defaultUsername, token)
		maskedURL, _ := u.MaskBasicAuth(parsedURL.String())
		log.Debug("Injected token", "env", tokenSource, keyURL, maskedURL)
	} else {
		log.Debug("No token found for injection")
	}
}

// resolveToken returns the token and its source based on the host.
func (d *CustomGitDetector) resolveToken(host string) (string, string) {
	var token, tokenSource string
	switch host {
	case "github.com":
		if d.AtmosConfig.Settings.InjectGithubToken {
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
	return token, tokenSource
}

// getDefaultUsername returns the default username for token injection based on the host.
func getDefaultUsername(host string) string {
	switch host {
	case "github.com":
		return "x-access-token"
	case "gitlab.com":
		return "oauth2"
	case "bitbucket.org":
		defaultUsername := os.Getenv("ATMOS_BITBUCKET_USERNAME")
		if defaultUsername == "" {
			defaultUsername = os.Getenv("BITBUCKET_USERNAME")
			if defaultUsername == "" {
				return "x-token-auth"
			}
		}
		log.Debug("Using Bitbucket username", "username", defaultUsername)
		return defaultUsername
	default:
		return "x-access-token"
	}
}

// adjustSubdir appends "//." to the path if no subdirectory is specified.
func (d *CustomGitDetector) adjustSubdir(parsedURL *url.URL, source string) {
	normalizedSource := filepath.ToSlash(source)
	if normalizedSource != "" && !strings.Contains(normalizedSource, "//") {
		parts := strings.SplitN(parsedURL.Path, "/", 4)
		if strings.HasSuffix(parsedURL.Path, ".git") || len(parts) == 3 {
			maskedSrc, _ := u.MaskBasicAuth(source)
			log.Debug("Detected top-level repo with no subdir: appending '//.'", keyURL, maskedSrc)
			parsedURL.Path += "//."
		}
	}
}

// RegisterCustomDetectors prepends the custom detector so it runs before
// the built-in ones. Any code that calls go-getter should invoke this.
func RegisterCustomDetectors(atmosConfig *schema.AtmosConfiguration, source string) {
	getter.Detectors = append(
		[]getter.Detector{
			&CustomGitDetector{AtmosConfig: atmosConfig, source: source},
		},
		getter.Detectors...,
	)
}

// GoGetterGet downloads packages (files and folders) from different sources using `go-getter` and saves them into the destination.
func GoGetterGet(
	atmosConfig *schema.AtmosConfiguration,
	src string,
	dest string,
	clientMode getter.ClientMode,
	timeout time.Duration,
) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Register custom detectors, passing the original `src` to the CustomGitDetector.
	// go-getter typically strips subdirectories before calling the detector, so the
	// unaltered source is needed to identify whether a top-level repository or a
	// subdirectory was specified (e.g., for appending "//." only when no subdir is present).
	RegisterCustomDetectors(atmosConfig, src)

	client := &getter.Client{
		Ctx: ctx,
		Src: src,
		// Destination where the files will be stored. This will create the directory if it doesn't exist
		Dst:  dest,
		Mode: clientMode,
		Getters: map[string]getter.Getter{
			// Overriding 'git'
			"git":   &CustomGitGetter{},
			"file":  &getter.FileGetter{},
			"hg":    &getter.HgGetter{},
			"http":  &getter.HttpGetter{},
			"https": &getter.HttpGetter{},
			// "s3": &getter.S3Getter{}, // add as needed
			// "gcs": &getter.GCSGetter{},
		},
	}
	if err := client.Get(); err != nil {
		return err
	}

	return nil
}

// CustomGitGetter is a custom getter for git (git::) that removes symlinks.
type CustomGitGetter struct {
	getter.GitGetter
}

// Get implements the custom getter logic removing symlinks.
func (c *CustomGitGetter) Get(dst string, url *url.URL) error {
	// Normal clone
	if err := c.GitGetter.Get(dst, url); err != nil {
		return err
	}
	// Remove symlinks
	return removeSymlinks(dst)
}

// removeSymlinks walks the directory and removes any symlinks
// it encounters.
func removeSymlinks(root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			log.Debug("Removing symlink", "path", path)
			// Symlinks are removed for the entire repo, regardless if there are any subfolders specified
			return os.Remove(path)
		}
		return nil
	})
}

// DownloadDetectFormatAndParseFile downloads a remote file, detects the format of the file (JSON, YAML, HCL) and parses the file into a Go type.
func DownloadDetectFormatAndParseFile(atmosConfig *schema.AtmosConfiguration, file string) (any, error) {
	tempDir := os.TempDir()
	f := filepath.Join(tempDir, uuid.New().String())

	if err := GoGetterGet(atmosConfig, file, f, getter.ClientModeFile, 30*time.Second); err != nil {
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
