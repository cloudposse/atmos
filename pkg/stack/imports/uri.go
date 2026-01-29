// Package imports provides functionality for processing stack imports,
// including support for remote imports from URLs via go-getter.
package imports

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/hashicorp/go-getter"
)

// scpURLPattern matches SCP-style Git URLs (e.g., git@github.com:owner/repo.git).
var scpURLPattern = regexp.MustCompile(`^(([\w.-]+)@)?([\w.-]+\.[\w.-]+):([\w./-]+)(\.git)?(.*)$`)

// IsLocalPath checks if the URI is a local file system path.
// Examples:
//   - Local: "/absolute/path", "./relative/path", "../parent/path", "components/terraform"
//   - Remote: "github.com/owner/repo", "https://example.com", "git.company.com/repo"
func IsLocalPath(uri string) bool {
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
	return !IsLocalPath(uri)
}

// HasSchemeSeparator checks if the URI contains a scheme separator.
// Examples:
//   - true: "https://github.com", "git::https://...", "s3::https://..."
//   - false: "github.com/repo", "./local/path", "components/terraform"
func HasSchemeSeparator(uri string) bool {
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
func isDomainLikeURI(uri string) bool {
	dotPos := strings.Index(uri, ".")
	if dotPos <= 0 || dotPos >= len(uri)-1 {
		return false
	}

	// Check if there's a slash after the dot (indicating a domain with path).
	afterDot := uri[dotPos+1:]
	slashPos := strings.Index(afterDot, "/")
	return slashPos > 0
}

// IsGitURI checks if the URI appears to be a Git repository URL.
// Detection rules:
// 1. Explicit git:: prefix.
// 2. SCP-style URLs (git@github.com:owner/repo.git).
// 3. Known Git hosting platforms (github.com, gitlab.com, bitbucket.org) in host.
// 4. .git extension in path (not in host).
// 5. Azure DevOps _git/ pattern in path.
func IsGitURI(uri string) bool {
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
	return strings.HasPrefix(uri, "http://") || strings.HasPrefix(uri, "https://")
}

// IsS3URI checks if the URI is an S3 URI.
// Go-getter supports both explicit s3:: prefix and auto-detected .amazonaws.com URLs.
func IsS3URI(uri string) bool {
	return strings.HasPrefix(uri, "s3::") || strings.Contains(uri, ".amazonaws.com/")
}

// IsGCSURI checks if the URI is a Google Cloud Storage URI.
func IsGCSURI(uri string) bool {
	return strings.HasPrefix(uri, "gcs::") || strings.HasPrefix(uri, "gcs://")
}
