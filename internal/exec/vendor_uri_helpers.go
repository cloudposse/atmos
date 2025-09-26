package exec

import (
	"strings"
)

// isFileURI checks if the URI is a file:// scheme.
func isFileURI(uri string) bool {
	return strings.HasPrefix(uri, "file://")
}

// isOCIURI checks if the URI is an OCI registry URI.
func isOCIURI(uri string) bool {
	return strings.HasPrefix(uri, "oci://")
}

// isS3URI checks if the URI is an S3 URI.
func isS3URI(uri string) bool {
	return strings.HasPrefix(uri, "s3::")
}

// isLocalPath checks if the URI is a local file system path.
func isLocalPath(uri string) bool {
	// Local paths start with /, ./, ../, or are relative paths without scheme
	if strings.HasPrefix(uri, "/") || strings.HasPrefix(uri, "./") || strings.HasPrefix(uri, "../") {
		return true
	}
	// If it contains a scheme separator, it's not a local path
	if strings.Contains(uri, "://") || strings.Contains(uri, "::") {
		return false
	}
	// If it looks like a Git host, it's not a local path
	if strings.Contains(uri, "github.com") || strings.Contains(uri, "gitlab.com") ||
		strings.Contains(uri, "bitbucket.org") || strings.Contains(uri, ".git") {
		return false
	}
	// Otherwise, it's likely a relative local path
	return true
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
func isGitURI(uri string) bool {
	// Check for explicit git:: prefix
	if strings.HasPrefix(uri, "git::") {
		return true
	}

	// Check for common Git hosting platforms
	gitHosts := []string{"github.com", "gitlab.com", "bitbucket.org", "git."}
	lowerURI := strings.ToLower(uri)
	for _, host := range gitHosts {
		if strings.Contains(lowerURI, host) {
			return true
		}
	}

	// Check for .git suffix
	if strings.Contains(uri, ".git") {
		return true
	}

	return false
}

// hasSubdirectory checks if the URI already has a subdirectory delimiter.
func hasSubdirectory(uri string) bool {
	// Look for the // delimiter that's not part of a scheme (like https://)
	// First, remove any scheme prefix to avoid false positives
	workingURI := uri

	// Handle git:: prefix specially - it can have an underlying scheme
	workingURI = strings.TrimPrefix(workingURI, "git::")

	// Remove common scheme prefixes
	schemes := []string{"https://", "http://", "ssh://", "git+https://", "git+ssh://"}
	for _, scheme := range schemes {
		if strings.HasPrefix(workingURI, scheme) {
			workingURI = strings.TrimPrefix(workingURI, scheme)
			break
		}
	}

	// Now check if there's a // in the remaining part
	return strings.Contains(workingURI, "//")
}

// containsTripleSlash checks if the URI contains the triple-slash pattern for Git repos.
func containsTripleSlash(uri string) bool {
	return strings.Contains(uri, ".git///") || strings.Contains(uri, ".com///") || strings.Contains(uri, ".org///")
}

// extractURIParts splits a Git URI into base and suffix parts around the triple-slash.
func extractURIParts(uri string, pattern string) (base string, suffix string, found bool) {
	pos := strings.Index(uri, pattern)
	if pos == -1 {
		return "", "", false
	}

	patternLen := len(pattern)
	base = uri[:pos+patternLen-3] // Keep up to and including .git or .com/.org
	suffix = uri[pos+patternLen:] // Everything after ///
	return base, suffix, true
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
