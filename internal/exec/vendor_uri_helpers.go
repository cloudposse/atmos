package exec

import (
	"strings"

	"github.com/hashicorp/go-getter"
)

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
	return strings.Contains(uri, "//")
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

// parseGitHostAndPath separates a URI into host and path components.
func parseGitHostAndPath(uri string) (host string, path string) {
	// Remove scheme if present to get the actual host/path
	workingURI := uri
	if idx := strings.Index(uri, "://"); idx != -1 {
		workingURI = uri[idx+3:]
	}

	// Split on first / to separate host from path
	parts := strings.SplitN(workingURI, "/", 2)
	host = strings.ToLower(parts[0])
	if len(parts) > 1 {
		path = "/" + parts[1]
	}
	return host, path
}

// isKnownGitHost checks if the host is a known Git hosting platform.
func isKnownGitHost(host string) bool {
	gitHosts := []string{"github.com", "gitlab.com", "bitbucket.org"}
	for _, gitHost := range gitHosts {
		if host == gitHost || strings.HasSuffix(host, "."+gitHost) {
			return true
		}
	}
	return false
}

// hasGitExtension checks if the path has a .git extension in the correct position.
func hasGitExtension(path string) bool {
	idx := strings.Index(path, ".git")
	if idx == -1 {
		return false
	}
	afterGit := path[idx+4:]
	return afterGit == "" || strings.HasPrefix(afterGit, "/") || strings.HasPrefix(afterGit, "?")
}

// isGitURI checks if the URI appears to be a Git repository URL.
// Properly parses URLs to check host and path separately, avoiding false positives.
func isGitURI(uri string) bool {
	// Check for explicit git:: prefix
	if strings.HasPrefix(uri, "git::") {
		return true
	}

	// Parse the URI to separate host from path
	host, path := parseGitHostAndPath(uri)

	// Check if host is a known Git hosting platform
	if isKnownGitHost(host) {
		return true
	}

	// Check path for .git extension
	if hasGitExtension(path) {
		return true
	}

	// Check path for Azure DevOps _git/ pattern
	return strings.Contains(path, "/_git/")
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
	// Skip if not a Git URI
	if !isGitURI(uri) {
		return false
	}

	// Skip if it already has a subdirectory
	if hasSubdirectory(uri) {
		return false
	}

	// Skip special URI types that don't need modification
	if isFileURI(uri) || isOCIURI(uri) || isS3URI(uri) || isLocalPath(uri) || isNonGitHTTPURI(uri) {
		return false
	}

	return true
}

// appendDoubleSlashDot adds double-slash-dot to a URI, handling query parameters correctly.
func appendDoubleSlashDot(uri string) string {
	// Find the position of query parameters if they exist
	queryPos := strings.Index(uri, "?")

	if queryPos != -1 {
		// Insert //. before the query parameters
		return uri[:queryPos] + "//." + uri[queryPos:]
	}

	// No query parameters, just append
	return uri + "//."
}
