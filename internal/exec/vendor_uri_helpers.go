package exec

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/hashicorp/go-getter"
)

// scpURLPattern matches SCP-style Git URLs (e.g., git@github.com:owner/repo.git).
// This pattern is also used by CustomGitDetector.rewriteSCPURL in pkg/downloader/.
var scpURLPattern = regexp.MustCompile(`^(([\w.-]+)@)?([\w.-]+\.[\w.-]+):([\w./-]+)(\.git)?(.*)$`)

// isFileURI checks if the URI is a file:// scheme.
func isFileURI(uri string) bool {
	return strings.HasPrefix(uri, "file://")
}

// isOCIURI checks if the URI is an OCI registry URI.
func isOCIURI(uri string) bool {
	return strings.HasPrefix(uri, "oci://")
}

// IsS3URI checks if the URI is an S3 URI.
// Go-getter supports both explicit s3:: prefix and auto-detected .amazonaws.com URLs.
func isS3URI(uri string) bool {
	return strings.HasPrefix(uri, "s3::") || strings.Contains(uri, ".amazonaws.com/")
}

// hasLocalPathPrefix checks if the URI starts with local path prefixes.
// Examples:
//   - true: "/absolute/path", "./relative/path", "../parent/path"
//   - false: "github.com/repo", "https://example.com", "components/terraform"
func hasLocalPathPrefix(uri string) bool {
	return strings.HasPrefix(uri, "/") || strings.HasPrefix(uri, "./") || strings.HasPrefix(uri, "../")
}

// hasSchemeSeparator checks if the URI contains a scheme separator.
// Examples:
//   - true: "https://github.com", "git::https://...", "s3::https://..."
//   - false: "github.com/repo", "./local/path", "components/terraform"
func hasSchemeSeparator(uri string) bool {
	return strings.Contains(uri, "://") || strings.Contains(uri, "::")
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

// isLocalPath checks if the URI is a local file system path.
// Examples:
//   - Local: "/absolute/path", "./relative/path", "../parent/path", "components/terraform"
//   - Remote: "github.com/owner/repo", "https://example.com", "git.company.com/repo"
func isLocalPath(uri string) bool {
	// Local paths start with /, ./, ../, or are relative paths without scheme
	// Examples: "/abs/path", "./rel/path", "../parent", "components/terraform"
	if hasLocalPathPrefix(uri) {
		return true
	}

	// If it contains a scheme separator, it's not a local path
	// Examples: "https://github.com", "git::https://...", "s3::..."
	// This check must come BEFORE the '//' check to avoid false positives from "://"
	if hasSchemeSeparator(uri) {
		return false
	}

	// If it contains the go-getter subdirectory delimiter, it's not a local path
	// Examples: "github.com/repo//path", "git.company.com/repo//modules"
	if hasSubdirectoryDelimiter(uri) {
		return false
	}

	// If it looks like a Git repository, it's not a local path
	// Examples: "github.com/owner/repo", "gitlab.com/project", "repo.git", "org/_git/repo" (Azure DevOps)
	if isGitURI(uri) {
		return false
	}

	// If it has a domain-like structure (hostname.domain/path), it's not a local path
	// Examples: "git.company.com/repo", "gitea.io/owner/repo"
	if isDomainLikeURI(uri) {
		return false
	}

	// Otherwise, it's likely a relative local path
	// Examples: "components/terraform", "mixins/context.tf"
	return true
}

// isDomainLikeURI checks if the URI has a domain-like structure (hostname.domain/path).
func isDomainLikeURI(uri string) bool {
	dotPos := strings.Index(uri, ".")
	if dotPos <= 0 || dotPos >= len(uri)-1 {
		return false
	}

	// Check if there's a slash after the dot (indicating a domain with path)
	afterDot := uri[dotPos+1:]
	slashPos := strings.Index(afterDot, "/")
	return slashPos > 0
}

// isNonGitHTTPURI checks if the URI is an HTTP/HTTPS URL that doesn't appear to be a Git repository.
func isNonGitHTTPURI(uri string) bool {
	if !strings.HasPrefix(uri, "http://") && !strings.HasPrefix(uri, "https://") {
		return false
	}
	// Check for common archive extensions that indicate it's not a Git repo
	lowerURI := strings.ToLower(uri)
	archiveExtensions := []string{".tar.gz", ".tgz", ".tar.bz2", ".zip", ".tar", ".gz", ".bz2"}
	for _, ext := range archiveExtensions {
		if strings.Contains(lowerURI, ext) {
			return true
		}
	}
	return false
}

// isGitURI checks if the URI appears to be a Git repository URL.
// We cannot use go-getter's Detect() because it only detects specific platforms (GitHub/GitLab/BitBucket)
// and treats everything else as file://. We need broader detection for self-hosted Git, Azure DevOps, etc.
//
// This uses net/url.Parse for proper host/path separation instead of custom string manipulation.
// Detection rules:
// 1. Explicit git:: prefix.
// 2. SCP-style URLs (git@github.com:owner/repo.git).
// 3. Known Git hosting platforms (github.com, gitlab.com, bitbucket.org) in host.
// 4. .git extension in path (not in host).
// 5. Azure DevOps _git/ pattern in path.
func isGitURI(uri string) bool {
	// Check for explicit git:: forced getter prefix
	if strings.HasPrefix(uri, "git::") {
		return true
	}

	// Remove go-getter's subdirectory delimiter for parsing
	srcURI, _ := getter.SourceDirSubdir(uri)

	// Check for SCP-style URLs (git@github.com:owner/repo.git)
	// Use same pattern as CustomGitDetector.rewriteSCPURL
	if scpURLPattern.MatchString(srcURI) {
		return true
	}

	// Use standard library url.Parse for proper URL parsing
	// Add https:// scheme if missing to help url.Parse identify the host
	parseURI := srcURI
	if !strings.Contains(parseURI, "://") {
		parseURI = "https://" + parseURI
	}

	parsedURL, err := url.Parse(parseURI)
	if err != nil {
		// If URL parsing fails, it's likely not a valid Git URL
		return false
	}

	host := strings.ToLower(parsedURL.Host)
	path := parsedURL.Path

	// Check for known Git hosting platforms
	knownHosts := []string{"github.com", "gitlab.com", "bitbucket.org"}
	for _, knownHost := range knownHosts {
		if host == knownHost || strings.HasSuffix(host, "."+knownHost) {
			return true
		}
	}

	// Check for .git extension in path (not in host)
	if strings.Contains(path, ".git") {
		return true
	}

	// Check for Azure DevOps _git/ pattern
	if strings.Contains(path, "/_git/") {
		return true
	}

	return false
}

// hasSubdirectory checks if the URI already has a subdirectory delimiter.
// Uses go-getter's SourceDirSubdir to properly parse the URL.
func hasSubdirectory(uri string) bool {
	// Use go-getter's built-in parser to extract subdirectory
	_, subdir := getter.SourceDirSubdir(uri)
	return subdir != ""
}

// containsTripleSlash checks if the URI contains the triple-slash pattern.
// This is a legacy pattern that needs normalization.
func containsTripleSlash(uri string) bool {
	// Check for literal triple-slash pattern in the URI
	// This is the most reliable way to detect the pattern regardless of platform
	return strings.Contains(uri, "///")
}

// parseSubdirFromTripleSlash extracts source and subdirectory from a triple-slash URI.
// Uses go-getter's SourceDirSubdir for proper parsing.
// Examples:
//   - Input: "github.com/owner/repo.git///?ref=v1.0" → source="github.com/owner/repo.git?ref=v1.0", subdir=""
//   - Input: "github.com/owner/repo.git///path?ref=v1.0" → source="github.com/owner/repo.git?ref=v1.0", subdir="path"
func parseSubdirFromTripleSlash(uri string) (source string, subdir string) {
	source, subdir = getter.SourceDirSubdir(uri)

	// If subdirectory starts with "/", it means triple-slash was used
	// Remove the leading "/" to get the actual subdirectory path
	// Examples: "/" → "", "/path" → "path"
	subdir = strings.TrimPrefix(subdir, "/")

	return source, subdir
}

// needsDoubleSlashDot determines if a URI needs double-slash-dot appended.
func needsDoubleSlashDot(uri string) bool {
	// Only Git URIs need double-slash-dot (e.g., github.com/owner/repo.git needs github.com/owner/repo.git//.)
	if !isGitURI(uri) {
		return false // Not a Git URI, doesn't need //.
	}

	// Already has subdirectory specified, no need to add //.
	if hasSubdirectory(uri) {
		return false
	}

	// These special URI types shouldn't be modified even if they look like Git.
	if isFileURI(uri) || isOCIURI(uri) || isS3URI(uri) || isLocalPath(uri) || isNonGitHTTPURI(uri) {
		return false
	}

	// It's a Git URI without a subdirectory, needs //. appended.
	return true
}

// appendDoubleSlashDot adds double-slash-dot to a URI, handling query parameters correctly.
// Removes any trailing "//" from the base URI before appending "//." to avoid creating "////".
func appendDoubleSlashDot(uri string) string {
	// Find the position of query parameters if they exist
	queryPos := strings.Index(uri, "?")

	var base, queryPart string
	if queryPos != -1 {
		base = uri[:queryPos]
		queryPart = uri[queryPos:]
	} else {
		base = uri
		queryPart = ""
	}

	// Remove trailing "//" if present to avoid creating "////"
	base = strings.TrimSuffix(base, "//")

	// Append //. and query parameters
	return base + "//." + queryPart
}
