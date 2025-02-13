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
	l.Debug(fmt.Sprintf("CustomGitDetector.Detect(%q, %q)", src, d.source))

	if len(src) == 0 {
		return "", false, nil
	}

	// We need this block because many SCP-style URLs aren’t valid according to Go’s URL parser.
	// SCP-style URLs omit an explicit scheme (like "ssh://" or "https://") and use a colon
	// to separate the host from the path. Go’s URL parser expects a scheme, so without one,
	// it fails to parse these URLs correctly.
	// Below, we check if the URL doesn’t contain a scheme. If so, we attempt to detect an SCP-style URL:
	// e.g. "git@github.com:cloudposse/terraform-null-label.git?ref=..."
	// If the URL matches this pattern, we rewrite it to a proper SSH URL.
	// Otherwise, we default to prepending "https://".
	if !strings.Contains(src, "://") {
		// Check for SCP-style SSH URL (e.g. "git@github.com:cloudposse/terraform-null-label.git?ref=...")
		// This regex supports any host with a dot (e.g. github.com, bitbucket.org, gitlab.com)
		scpPattern := regexp.MustCompile(`^(([\w.-]+)@)?([\w.-]+\.[\w.-]+):([\w./-]+)(\.git)?(.*)$`)
		if scpPattern.MatchString(src) {
			matches := scpPattern.FindStringSubmatch(src)
			// Build proper SSH URL: "ssh://[username@]host/repoPath[.git][additional]"
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
			l.Debug(fmt.Sprintf("Rewriting SCP-style SSH URL to proper SSH URL: %s -> %s", src, newSrc))
			src = newSrc
		} else {
			src = "https://" + src
			l.Debug(fmt.Sprintf("Defaulting to https scheme, url is %q:", src))
		}
	}

	l.Debug(fmt.Sprintf("url = %q:", src))

	parsedURL, err := url.Parse(src)
	if err != nil {
		l.Debug(fmt.Sprintf("Failed to parse URL %q: %v", src, err))
		return "", false, fmt.Errorf("failed to parse URL %q: %w", src, err)
	}

	// If the URL uses the SSH scheme, check for an active SSH agent.
	if parsedURL.Scheme == "ssh" && os.Getenv("SSH_AUTH_SOCK") == "" {
		return "", false, fmt.Errorf("SSH URL detected but no SSH agent appears to be active. Please ensure your SSH key is loaded (e.g. run 'eval $(ssh-agent -s)' and 'ssh-add ~/.ssh/id_ed25519')")
	}

	// Adjust host check to support GitHub, Bitbucket, GitLab, etc.
	host := strings.ToLower(parsedURL.Host)
	if host != "github.com" && host != "bitbucket.org" && host != "gitlab.com" {
		l.Debug(fmt.Sprintf("Host is %q, not recognized for token injection", parsedURL.Host))
		// For unrecognized hosts, simply return without injecting tokens.
		return "", false, nil
	}

	// TBC: should we support more tokens for Bitbucket and GitLab at all? Any other hosts?
	// Any other git-enabled hosts to be added?
	var token, tokenSource string
	switch host {
	case "github.com":
		token = os.Getenv("ATMOS_GITHUB_TOKEN")
		if token == "" && d.AtmosConfig.Settings.InjectGithubToken {
			token = os.Getenv("GITHUB_TOKEN")
			tokenSource = "GITHUB_TOKEN"
		} else {
			tokenSource = "ATMOS_GITHUB_TOKEN"
		}
	case "bitbucket.org":
		token = os.Getenv("ATMOS_BITBUCKET_TOKEN")
		if token == "" {
			token = os.Getenv("BITBUCKET_TOKEN")
			tokenSource = "BITBUCKET_TOKEN"
		} else {
			tokenSource = "ATMOS_BITBUCKET_TOKEN"
		}
	case "gitlab.com":
		token = os.Getenv("ATMOS_GITLAB_TOKEN")
		if token == "" {
			token = os.Getenv("GITLAB_TOKEN")
			tokenSource = "GITLAB_TOKEN"
		} else {
			tokenSource = "ATMOS_GITLAB_TOKEN"
		}
	}

	if token != "" {
		// Inject token only if no credentials are already provided.
		if parsedURL.User == nil || parsedURL.User.Username() == "" {
			l.Debug(fmt.Sprintf("Injecting token from %s for %s", tokenSource, src))
			parsedURL.User = url.UserPassword("x-access-token", token)
		} else {
			l.Debug("Credentials already provided, skipping token injection")
		}
	}

	//  check if the user typed something like
	// "github.com/org/repo.git" with NO subdir and, if so, appends '//.'.
	if !strings.Contains(d.source, "//") {
		// means user typed something like "github.com/org/repo.git" with NO subdir
		parts := strings.SplitN(parsedURL.Path, "/", 4)
		if strings.HasSuffix(parsedURL.Path, ".git") || len(parts) == 3 {
			l.Debug("Detected top-level repo with no subdir: appending '//.'")
			parsedURL.Path = parsedURL.Path + "//."
		}
	}

	// Set "depth=1" for a shallow clone if not specified.
	// In Go-Getter, "depth" controls how many revisions are cloned:
	// - `depth=1` fetches only the latest commit (faster, less bandwidth).
	// - `depth=` (empty) performs a full clone (default Git behavior).
	// - `depth=N` clones the last N revisions.
	q := parsedURL.Query()
	if _, exists := q["depth"]; !exists {
		q.Set("depth", "1")
	}
	parsedURL.RawQuery = q.Encode()

	finalURL := "git::" + parsedURL.String()

	return finalURL, true, nil
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
