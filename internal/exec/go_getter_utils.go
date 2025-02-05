// https://github.com/hashicorp/go-getter

package exec

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

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
	// Add more validation as needed
	// Validate URI format
	if strings.Contains(uri, "..") {
		return fmt.Errorf("URI cannot contain path traversal sequences")
	}
	if strings.Contains(uri, " ") {
		return fmt.Errorf("URI cannot contain spaces")
	}
	// Validate characters
	if strings.ContainsAny(uri, "<>|&;$") {
		return fmt.Errorf("URI contains invalid characters")
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
	}
	return validSchemes[scheme]
}

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

// RegisterCustomDetectors prepends the custom detector so it runs before
// the built-in ones. Any code that calls go-getter should invoke this.
func RegisterCustomDetectors(atmosConfig schema.AtmosConfiguration) {
	getter.Detectors = append(
		[]getter.Detector{
			&CustomGitHubDetector{AtmosConfig: atmosConfig},
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
