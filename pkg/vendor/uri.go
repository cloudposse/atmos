package vendor

import (
	"net/url"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/hashicorp/go-getter"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// DoubleSlash is the go-getter subdirectory delimiter.
	doubleSlash = "//"
	// DoubleSlashDot indicates the root of the repository in go-getter.
	doubleSlashDot = "//."
)

// scpURLPattern matches SCP-style Git URLs (e.g., git@github.com:owner/repo.git).
// This pattern is also used by CustomGitDetector.rewriteSCPURL in pkg/downloader/.
var scpURLPattern = regexp.MustCompile(`^(([\w.-]+)@)?([\w.-]+\.[\w.-]+):([\w./-]+)(\.git)?(.*)$`)

// NormalizeURI normalizes vendor source URIs to handle all patterns consistently.
// It converts triple-slash patterns, appends double-slash-dot to Git URLs without
// subdirectory, and skips normalization for special URI types (file, oci, S3, local).
func NormalizeURI(uri string) string {
	defer perf.Track(nil, "vendor.NormalizeURI")()

	// Skip normalization for special URI types.
	if IsFileURI(uri) || IsOCIURI(uri) || IsS3URI(uri) || IsLocalPath(uri) || IsNonGitHTTPURI(uri) {
		return uri
	}

	// Handle triple-slash pattern first.
	if ContainsTripleSlash(uri) {
		uri = normalizeTripleSlash(uri)
	}

	// Add //. to Git URLs without subdirectory.
	if NeedsDoubleSlashDot(uri) {
		uri = AppendDoubleSlashDot(uri)
		log.Debug("Added //. to Git URL without subdirectory", "normalized", uri)
	}

	return uri
}

// normalizeTripleSlash converts triple-slash patterns to appropriate double-slash patterns.
// Uses go-getter's SourceDirSubdir for robust parsing across all Git platforms.
func normalizeTripleSlash(uri string) string {
	// Use go-getter to parse the URI and extract subdirectory.
	// Note: source will include query parameters from the original URI.
	source, subdir := ParseSubdirFromTripleSlash(uri)

	// Separate query parameters from source if present.
	var queryParams string
	if queryPos := strings.Index(source, "?"); queryPos != -1 {
		queryParams = source[queryPos:]
		source = source[:queryPos]
	}

	// Determine the normalized form based on subdirectory.
	if subdir == "" {
		// Root of repository case: convert /// to //.
		normalized := source + doubleSlashDot + queryParams
		log.Debug("Normalized triple-slash to double-slash-dot for repository root",
			"original", uri, "normalized", normalized)
		return normalized
	}
	// Path specified after triple slash: convert /// to //.
	normalized := source + doubleSlash + subdir + queryParams
	log.Debug("Normalized triple-slash to double-slash with path",
		"original", uri, "normalized", normalized)
	return normalized
}

// IsFileURI checks if the URI is a file:// scheme.
func IsFileURI(uri string) bool {
	return strings.HasPrefix(uri, "file://")
}

// IsOCIURI checks if the URI is an OCI registry URI.
func IsOCIURI(uri string) bool {
	return strings.HasPrefix(uri, "oci://")
}

// IsS3URI checks if the URI is an S3 URI.
// Go-getter supports both explicit s3:: prefix and auto-detected .amazonaws.com URLs.
func IsS3URI(uri string) bool {
	return strings.HasPrefix(uri, "s3::") || strings.Contains(uri, ".amazonaws.com/")
}

// HasLocalPathPrefix checks if the URI starts with local path prefixes.
func HasLocalPathPrefix(uri string) bool {
	return strings.HasPrefix(uri, "/") || strings.HasPrefix(uri, "./") || strings.HasPrefix(uri, "../")
}

// HasSchemeSeparator checks if the URI contains a scheme separator.
func HasSchemeSeparator(uri string) bool {
	return strings.Contains(uri, "://") || strings.Contains(uri, "::")
}

// HasSubdirectoryDelimiter checks if the URI contains the go-getter subdirectory delimiter.
func HasSubdirectoryDelimiter(uri string) bool {
	idx := strings.Index(uri, doubleSlash)
	if idx == -1 {
		return false
	}
	// If // is preceded by :, it's a scheme separator (://) not a subdirectory delimiter.
	if idx > 0 && uri[idx-1] == ':' {
		// Check if there's another // after the scheme separator.
		remaining := uri[idx+2:]
		return strings.Contains(remaining, doubleSlash)
	}
	return true
}

// IsLocalPath checks if the URI is a local file system path.
func IsLocalPath(uri string) bool {
	if HasLocalPathPrefix(uri) {
		return true
	}
	if HasSchemeSeparator(uri) {
		return false
	}
	if HasSubdirectoryDelimiter(uri) {
		return false
	}
	if IsGitURI(uri) {
		return false
	}
	if IsDomainLikeURI(uri) {
		return false
	}
	return true
}

// IsDomainLikeURI checks if the URI has a domain-like structure (hostname.domain/path).
func IsDomainLikeURI(uri string) bool {
	dotPos := strings.Index(uri, ".")
	if dotPos <= 0 || dotPos >= len(uri)-1 {
		return false
	}
	afterDot := uri[dotPos+1:]
	slashPos := strings.Index(afterDot, "/")
	return slashPos > 0
}

// IsNonGitHTTPURI checks if the URI is an HTTP/HTTPS URL that doesn't appear to be a Git repository.
func IsNonGitHTTPURI(uri string) bool {
	if !strings.HasPrefix(uri, "http://") && !strings.HasPrefix(uri, "https://") {
		return false
	}
	lowerURI := strings.ToLower(uri)
	archiveExtensions := []string{".tar.gz", ".tgz", ".tar.bz2", ".zip", ".tar", ".gz", ".bz2"}
	for _, ext := range archiveExtensions {
		if strings.Contains(lowerURI, ext) {
			return true
		}
	}
	return false
}

// IsGitURI checks if the URI appears to be a Git repository URL.
// Detection rules:
// 1. Explicit git:: prefix.
// 2. SCP-style URLs (git@github.com:owner/repo.git).
// 3. Known Git hosting platforms (github.com, gitlab.com, bitbucket.org) in host.
// 4. .git extension in path (not in host).
// 5. Azure DevOps _git/ pattern in path.
func IsGitURI(uri string) bool {
	if strings.HasPrefix(uri, "git::") {
		return true
	}

	srcURI, _ := getter.SourceDirSubdir(uri)

	if scpURLPattern.MatchString(srcURI) {
		return true
	}

	parseURI := srcURI
	if !strings.Contains(parseURI, "://") {
		parseURI = "https://" + parseURI
	}

	parsedURL, err := url.Parse(parseURI)
	if err != nil {
		return false
	}

	host := strings.ToLower(parsedURL.Host)
	path := parsedURL.Path

	knownHosts := []string{"github.com", "gitlab.com", "bitbucket.org"}
	for _, knownHost := range knownHosts {
		if host == knownHost || strings.HasSuffix(host, "."+knownHost) {
			return true
		}
	}

	if strings.Contains(path, ".git") {
		return true
	}

	if strings.Contains(path, "/_git/") {
		return true
	}

	return false
}

// HasSubdirectory checks if the URI already has a subdirectory delimiter.
func HasSubdirectory(uri string) bool {
	_, subdir := getter.SourceDirSubdir(uri)
	return subdir != ""
}

// ContainsTripleSlash checks if the URI contains the triple-slash pattern.
func ContainsTripleSlash(uri string) bool {
	return strings.Contains(uri, "///")
}

// ParseSubdirFromTripleSlash extracts source and subdirectory from a triple-slash URI.
func ParseSubdirFromTripleSlash(uri string) (source string, subdir string) {
	source, subdir = getter.SourceDirSubdir(uri)
	subdir = strings.TrimPrefix(subdir, "/")
	return source, subdir
}

// NeedsDoubleSlashDot determines if a URI needs double-slash-dot appended.
func NeedsDoubleSlashDot(uri string) bool {
	if !IsGitURI(uri) {
		return false
	}
	if HasSubdirectory(uri) {
		return false
	}
	if IsFileURI(uri) || IsOCIURI(uri) || IsS3URI(uri) || IsLocalPath(uri) || IsNonGitHTTPURI(uri) {
		return false
	}
	return true
}

// AppendDoubleSlashDot adds double-slash-dot to a URI, handling query parameters correctly.
func AppendDoubleSlashDot(uri string) string {
	queryPos := strings.Index(uri, "?")

	var base, queryPart string
	if queryPos != -1 {
		base = uri[:queryPos]
		queryPart = uri[queryPos:]
	} else {
		base = uri
		queryPart = ""
	}

	base = strings.TrimSuffix(base, doubleSlash)
	return base + doubleSlashDot + queryPart
}

// SanitizeFileName makes a URI safe for use as a filename.
func SanitizeFileName(uri string) string {
	defer perf.Track(nil, "vendor.SanitizeFileName")()

	parsed, err := url.Parse(uri)
	if err != nil {
		return filepath.Base(uri)
	}

	base := filepath.Base(parsed.Path)

	if runtime.GOOS != "windows" {
		return base
	}

	base = strings.Map(func(r rune) rune {
		switch r {
		case '\\', '/', ':', '*', '?', '"', '<', '>', '|':
			return '_'
		default:
			return r
		}
	}, base)

	return base
}
