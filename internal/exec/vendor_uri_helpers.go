package exec

// This file contains thin wrappers that delegate to pkg/vendor/uri.go.
// The actual implementation lives in pkg/vendor/ as the single shared code path.
// These wrappers exist so that existing tests in vendor_uri_helpers_test.go continue to work.

import (
	"github.com/cloudposse/atmos/pkg/vendor"
)

// isFileURI checks if the URI is a file:// scheme.
func isFileURI(uri string) bool {
	return vendor.IsFileURI(uri)
}

// isOCIURI checks if the URI is an OCI registry URI.
func isOCIURI(uri string) bool {
	return vendor.IsOCIURI(uri)
}

// isS3URI checks if the URI is an S3 URI.
func isS3URI(uri string) bool {
	return vendor.IsS3URI(uri)
}

// hasLocalPathPrefix checks if the URI starts with local path prefixes.
func hasLocalPathPrefix(uri string) bool {
	return vendor.HasLocalPathPrefix(uri)
}

// hasSchemeSeparator checks if the URI contains a scheme separator.
func hasSchemeSeparator(uri string) bool {
	return vendor.HasSchemeSeparator(uri)
}

// hasSubdirectoryDelimiter checks if the URI contains the go-getter subdirectory delimiter.
func hasSubdirectoryDelimiter(uri string) bool {
	return vendor.HasSubdirectoryDelimiter(uri)
}

// isLocalPath checks if the URI is a local file system path.
func isLocalPath(uri string) bool {
	return vendor.IsLocalPath(uri)
}

// isDomainLikeURI checks if the URI has a domain-like structure (hostname.domain/path).
func isDomainLikeURI(uri string) bool {
	return vendor.IsDomainLikeURI(uri)
}

// isNonGitHTTPURI checks if the URI is an HTTP/HTTPS URL that doesn't appear to be a Git repository.
func isNonGitHTTPURI(uri string) bool {
	return vendor.IsNonGitHTTPURI(uri)
}

// isGitURI checks if the URI appears to be a Git repository URL.
func isGitURI(uri string) bool {
	return vendor.IsGitURI(uri)
}

// hasSubdirectory checks if the URI already has a subdirectory delimiter.
func hasSubdirectory(uri string) bool {
	return vendor.HasSubdirectory(uri)
}

// containsTripleSlash checks if the URI contains the triple-slash pattern.
func containsTripleSlash(uri string) bool {
	return vendor.ContainsTripleSlash(uri)
}

// parseSubdirFromTripleSlash extracts source and subdirectory from a triple-slash URI.
func parseSubdirFromTripleSlash(uri string) (source string, subdir string) {
	return vendor.ParseSubdirFromTripleSlash(uri)
}

// needsDoubleSlashDot determines if a URI needs double-slash-dot appended.
func needsDoubleSlashDot(uri string) bool {
	return vendor.NeedsDoubleSlashDot(uri)
}

// appendDoubleSlashDot adds double-slash-dot to a URI, handling query parameters correctly.
func appendDoubleSlashDot(uri string) string {
	return vendor.AppendDoubleSlashDot(uri)
}
