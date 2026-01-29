// Package imports provides functionality for processing stack imports,
// including support for remote imports from URLs via go-getter.
package imports

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/hashicorp/go-getter"
)

// scpURLPattern matches SCP-style Git URLs (e.g., git@github.com:owner/repo.git).
var scpURLPattern = regexp.MustCompile(`^(([\w.-]+)@)?([\w.-]+\.[\w.-]+):([\w./-]+)(\.git)?(.*)$`)

// versionPattern matches version-like strings (e.g., "v1.0", "1.2.3", "v2").
var versionPattern = regexp.MustCompile(`^[vV]?\d+(\.\d+)*$`)

// IsLocalPath checks if the URI is a local file system path.
// Examples:
//   - Local: "/absolute/path", "./relative/path", "../parent/path", "components/terraform"
//   - Remote: "github.com/owner/repo", "https://example.com", "git.company.com/repo"
func IsLocalPath(uri string) bool {
	defer perf.Track(nil, "imports.IsLocalPath")()

	// Local paths start with /, ./, ../, or are relative paths without scheme.
	// Examples: "/abs/path", "./rel/path", "../parent", "components/terraform".
	if hasLocalPathPrefix(uri) {
		return true
	}

	// If it contains a scheme separator, it's not a local path.
	// Examples: "https://github.com", "git::https://...", "s3::...".
	// This check must come BEFORE the '//' check to avoid false positives from "://".
	if HasSchemeSeparator(uri) {
		return false
	}

	// If it contains the go-getter subdirectory delimiter, it's not a local path.
	// Examples: "github.com/repo//path", "git.company.com/repo//modules".
	if hasSubdirectoryDelimiter(uri) {
		return false
	}

	// If it looks like a Git repository, it's not a local path.
	// Examples: "github.com/owner/repo", "gitlab.com/project", "repo.git", "org/_git/repo" (Azure DevOps).
	if IsGitURI(uri) {
		return false
	}

	// If it has a domain-like structure (hostname.domain/path), it's not a local path.
	// Examples: "git.company.com/repo", "gitea.io/owner/repo".
	if isDomainLikeURI(uri) {
		return false
	}

	// Otherwise, it's likely a relative local path.
	// Examples: "components/terraform", "mixins/context.tf".
	return true
}

// IsRemote returns true if the URI is a remote URL that should be downloaded.
// This is the inverse of IsLocalPath.
func IsRemote(uri string) bool {
	defer perf.Track(nil, "imports.IsRemote")()

	return !IsLocalPath(uri)
}

// HasSchemeSeparator checks if the URI contains a scheme separator.
// Examples:
//   - true: "https://github.com", "git::https://...", "s3::https://..."
//   - false: "github.com/repo", "./local/path", "components/terraform"
func HasSchemeSeparator(uri string) bool {
	defer perf.Track(nil, "imports.HasSchemeSeparator")()

	return strings.Contains(uri, "://") || strings.Contains(uri, "::")
}

// hasLocalPathPrefix checks if the URI starts with local path prefixes.
// Examples:
//   - true: "/absolute/path", "./relative/path", "../parent/path"
//   - false: "github.com/repo", "https://example.com", "components/terraform"
func hasLocalPathPrefix(uri string) bool {
	return strings.HasPrefix(uri, "/") || strings.HasPrefix(uri, "./") || strings.HasPrefix(uri, "../")
}

// hasSubdirectoryDelimiter checks if the URI contains the go-getter subdirectory delimiter.
// Examples:
//   - true: "github.com/repo//path", "git.company.com/repo//modules"
//   - false: "github.com/repo", "https://github.com/repo", "./local/path"
func hasSubdirectoryDelimiter(uri string) bool {
	idx := strings.Index(uri, "//")
	if idx == -1 {
		return false
	}
	// If // is preceded by :, it's a scheme separator (://) not a subdirectory delimiter.
	if idx > 0 && uri[idx-1] == ':' {
		// Check if there's another // after the scheme separator.
		remaining := uri[idx+2:]
		return strings.Contains(remaining, "//")
	}
	return true
}

// isDomainLikeURI checks if the URI has a domain-like structure (hostname.domain/path).
// This excludes paths with version-like patterns (e.g., configs/v1.0/base).
func isDomainLikeURI(uri string) bool {
	// Find the first slash to separate potential host from path.
	slashPos := strings.Index(uri, "/")

	// If no slash, check if the entire string looks like a domain.
	var potentialHost string
	if slashPos == -1 {
		potentialHost = uri
	} else {
		potentialHost = uri[:slashPos]
	}

	// A domain-like host must have a dot and characters on both sides.
	dotPos := strings.Index(potentialHost, ".")
	if dotPos <= 0 || dotPos >= len(potentialHost)-1 {
		return false
	}

	// Check for common TLD-like endings or known domain patterns.
	// This helps distinguish "git.company.com" from "configs/v1.0".
	afterDot := potentialHost[dotPos+1:]

	// Common TLDs and domain suffixes that indicate a real domain.
	knownSuffixes := []string{
		"com", "org", "net", "io", "dev", "co", "edu", "gov", "mil",
		"uk", "de", "fr", "jp", "cn", "au", "ca", "nl", "se", "no",
	}
	for _, suffix := range knownSuffixes {
		if strings.EqualFold(afterDot, suffix) || strings.HasSuffix(strings.ToLower(afterDot), "."+suffix) {
			return true
		}
	}

	// Check for domain-like patterns (e.g., "company.internal", "gitlab.mycompany.com").
	// Must have at least 2 characters after the dot and not look like a version number.
	if len(afterDot) >= 2 {
		// Exclude version-like patterns (e.g., "v1.0", "2.0", "1.2.3").
		if isVersionLike(potentialHost) {
			return false
		}
		// If there's a path after the host and the host has a dot, it's likely a domain.
		if slashPos > 0 {
			return true
		}
	}

	return false
}

// isVersionLike checks if a string looks like a version number (e.g., "v1.0", "1.2.3", "v2").
func isVersionLike(s string) bool {
	return versionPattern.MatchString(s)
}

// IsGitURI checks if the URI appears to be a Git repository URL.
// Detection rules:
// 1. Explicit git:: prefix.
// 2. SCP-style URLs (git@github.com:owner/repo.git).
// 3. Known Git hosting platforms (github.com, gitlab.com, bitbucket.org) in host.
// 4. .git extension in path (not in host).
// 5. Azure DevOps _git/ pattern in path.
func IsGitURI(uri string) bool {
	defer perf.Track(nil, "imports.IsGitURI")()

	// Check for explicit git:: forced getter prefix.
	if strings.HasPrefix(uri, "git::") {
		return true
	}

	// Remove go-getter's subdirectory delimiter for parsing.
	srcURI, _ := getter.SourceDirSubdir(uri)

	// Check for SCP-style URLs (git@github.com:owner/repo.git).
	if scpURLPattern.MatchString(srcURI) {
		return true
	}

	// Use standard library url.Parse for proper URL parsing.
	// Add https:// scheme if missing to help url.Parse identify the host.
	parseURI := srcURI
	if !strings.Contains(parseURI, "://") {
		parseURI = "https://" + parseURI
	}

	parsedURL, err := url.Parse(parseURI)
	if err != nil {
		// If URL parsing fails, it's likely not a valid Git URL.
		return false
	}

	host := strings.ToLower(parsedURL.Host)
	path := parsedURL.Path

	// Check for known Git hosting platforms.
	knownHosts := []string{"github.com", "gitlab.com", "bitbucket.org"}
	for _, knownHost := range knownHosts {
		if host == knownHost || strings.HasSuffix(host, "."+knownHost) {
			return true
		}
	}

	// Check for .git extension in path (not in host).
	if strings.Contains(path, ".git") {
		return true
	}

	// Check for Azure DevOps _git/ pattern.
	if strings.Contains(path, "/_git/") {
		return true
	}

	return false
}

// IsHTTPURI checks if the URI is an HTTP/HTTPS URL.
func IsHTTPURI(uri string) bool {
	defer perf.Track(nil, "imports.IsHTTPURI")()

	return strings.HasPrefix(uri, "http://") || strings.HasPrefix(uri, "https://")
}

// IsS3URI checks if the URI is an S3 URI.
// Go-getter supports both explicit s3:: prefix and auto-detected .amazonaws.com URLs.
func IsS3URI(uri string) bool {
	defer perf.Track(nil, "imports.IsS3URI")()

	return strings.HasPrefix(uri, "s3::") || strings.Contains(uri, ".amazonaws.com/")
}

// IsGCSURI checks if the URI is a Google Cloud Storage URI.
func IsGCSURI(uri string) bool {
	defer perf.Track(nil, "imports.IsGCSURI")()

	return strings.HasPrefix(uri, "gcs::") || strings.HasPrefix(uri, "gcs://")
}
