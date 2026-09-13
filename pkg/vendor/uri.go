package vendor

import (
	"net/url"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
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

// IsFileURI checks if the URI uses the file scheme. This matches on the
// "file:" prefix rather than parsing the URI, so malformed file URIs (e.g. a
// bad IPv6 host) are still classified as file URIs and rejected with a
// proper parse error downstream instead of falling through to a doomed
// remote-source download attempt.
func IsFileURI(uri string) bool {
	return strings.HasPrefix(uri, "file:")
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

// directoryArchiveExtensions lists the go-getter decompressor extensions that
// unpack to a directory of files (possibly just one), as opposed to the
// single-compressed-file formats (.gz, .bz2, .xz, .zst alone) that unpack to
// exactly one file. Longer extensions are listed before their suffixes (e.g.
// "tar.gz" before "gz" would matter if this were used for prefix matching;
// HasSuffix below doesn't require that ordering, but it documents intent).
var directoryArchiveExtensions = []string{
	".tar.gz", ".tgz", ".tar.bz2", ".tbz2", ".tar.xz", ".txz",
	".tar.zst", ".tzst", ".tar", ".zip",
}

// archiveQueryParam is go-getter's query parameter that explicitly overrides
// extension-based archive detection. Its own client.go resolves it with a
// plain `c.Decompressors[archiveV]` map lookup after this rewrite:
// any value strconv.ParseBool parses as false ("false", "0", "f", "F",
// "FALSE", "False", etc.) is rewritten to the sentinel "-" before the lookup
// (`if b, err := strconv.ParseBool(archiveV); err == nil && !b { archiveV =
// "-" }`), while a ParseBool-true value ("true", "1", "t", etc.) is left as
// that literal string. Neither "-" nor "true"/"1"/"t" is ever a key in
// Decompressors, so both boolean spellings resolve to no decompressor and
// go-getter downloads the source as a plain file. Only a value ParseBool
// can't parse at all -- an explicit archive type like "zip" -- reaches the
// map lookup as-is, and only forces unarchiving if it names one of
// go-getter's *directory* archive types (see directoryArchiveExtensions);
// the single-file codec keys ("bz2", "gz", "xz", "zst") and any unrecognized
// value also resolve to no decompressor. An empty value (`?archive=`, as
// opposed to the parameter being entirely absent) is treated the same as
// absent -- go-getter falls through to extension-based detection rather than
// forcing unarchiving. See
// https://pkg.go.dev/github.com/hashicorp/go-getter#hdr-Archiving.
const archiveQueryParam = "archive"

// IsArchiveURI checks whether go-getter will unpack this source into a
// directory tree (a tarball or zip), rather than staging it as a single file.
// It honors go-getter's explicit `archive` query parameter override before
// falling back to extension-based detection on the path suffix, and strips
// both any go-getter subdirectory (`//...`) suffix and the query string
// first: a URI like `https://example.com/archive.zip//nested/dir` must still
// be recognized as an archive by its source extension, not misclassified by
// its subdirectory path, and `?archive=zip`/`?archive=false` must override
// extension detection either direction (forcing unarchiving even with no
// recognized extension, or disabling it for a recognized one) rather than
// being silently dropped along with the rest of the query string.
func IsArchiveURI(uri string) bool {
	source, _ := getter.SourceDirSubdir(uri)

	path := source
	var rawQuery string
	if idx := strings.IndexByte(path, '?'); idx != -1 {
		rawQuery = path[idx+1:]
		path = path[:idx]
	}

	if archive, ok := archiveQueryOverride(rawQuery); ok {
		return archive
	}

	lowerPath := strings.ToLower(path)
	for _, ext := range directoryArchiveExtensions {
		if strings.HasSuffix(lowerPath, ext) {
			return true
		}
	}
	return false
}

// archiveQueryOverride parses go-getter's explicit `archive` query parameter
// override out of rawQuery. It returns ok == false whenever there is no
// override to apply -- rawQuery is empty or unparseable, the archive
// parameter is absent, or its value is empty (`?archive=`, which go-getter
// treats the same as absent: see client.go's `archiveV != ""` check
// upstream) -- signaling the caller to fall back to extension-based
// detection instead.
func archiveQueryOverride(rawQuery string) (archive bool, ok bool) {
	if rawQuery == "" {
		return false, false
	}
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return false, false
	}
	raw, present := values[archiveQueryParam]
	if !present || len(raw) == 0 || raw[0] == "" {
		return false, false
	}
	// Parse with strconv.ParseBool, exactly like go-getter itself does. A
	// value ParseBool recognizes -- whether a false spelling ("0", "f") or a
	// true spelling ("1", "t") -- never resolves to a real Decompressors key
	// in go-getter's client.go (a false value is rewritten to the sentinel
	// "-"; a true value is left as the literal string "true"/"1"/"t"), so
	// go-getter downloads the source as a plain file either way.
	if _, err := strconv.ParseBool(raw[0]); err == nil {
		return false, true
	}
	// A value ParseBool can't parse at all (e.g. an explicit archive type
	// like "zip") reaches go-getter's Decompressors map lookup unchanged, so
	// it only forces unarchiving when it names one of go-getter's directory
	// archive types -- not a single-file codec key ("bz2", "gz", "xz",
	// "zst") or an unrecognized value, both of which also miss the lookup
	// and download as a plain file.
	return isDirectoryArchiveType(raw[0]), true
}

// isDirectoryArchiveType reports whether typ (without a leading dot, e.g.
// "zip" or "tar.gz") is one of go-getter's directory-archive Decompressors
// keys (github.com/hashicorp/go-getter@v1.8.6 decompress.go), as opposed to
// a single-file codec key ("bz2", "gz", "xz", "zst") or a value that isn't a
// Decompressors key at all. Matching is exact-case, mirroring go-getter's
// own `c.Decompressors[archiveV]` map lookup -- no case normalization.
func isDirectoryArchiveType(typ string) bool {
	for _, ext := range directoryArchiveExtensions {
		if strings.TrimPrefix(ext, ".") == typ {
			return true
		}
	}
	return false
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
// It detects archive extensions and known-host file download/raw content URL patterns.
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
	// Detect known-host file download and raw content URLs.
	// These are HTTP URLs on known Git hosts that point to downloadable files,
	// not Git repositories, and should not have //. appended.
	return isKnownHostFileURL(lowerURI)
}

// knownHostFilePattern defines a pattern pair for detecting file download URLs on known Git hosts.
type knownHostFilePattern struct {
	host string // Host substring to match (empty = match any host).
	path string // Path substring to match.
}

// knownHostFilePatterns lists URL patterns that indicate file downloads (not Git repos)
// on popular Git hosting platforms.
var knownHostFilePatterns = []knownHostFilePattern{
	{host: "", path: "/releases/download/"},       // GitHub release assets.
	{host: "raw.githubusercontent.com", path: ""}, // GitHub raw content via subdomain.
	{host: "github.com", path: "/raw/"},           // GitHub raw content via path.
	{host: "gitlab.com", path: "/-/raw/"},         // GitLab raw content.
	{host: "gitlab.com", path: "/-/archive/"},     // GitLab archive downloads.
	{host: "bitbucket.org", path: "/downloads/"},  // Bitbucket file downloads.
}

// isKnownHostFileURL checks if the URL matches known file download or raw content
// patterns on popular Git hosting platforms.
func isKnownHostFileURL(lowerURI string) bool {
	for _, p := range knownHostFilePatterns {
		hostMatch := p.host == "" || strings.Contains(lowerURI, p.host)
		pathMatch := p.path == "" || strings.Contains(lowerURI, p.path)
		if hostMatch && pathMatch {
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
