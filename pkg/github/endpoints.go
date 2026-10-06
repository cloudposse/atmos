package github

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Default GitHub.com endpoints, used when no environment override is present.
const (
	defaultGitHubServerURL  = "https://github.com"
	defaultGitHubAPIURL     = "https://api.github.com"
	defaultGitHubUploadURL  = "https://uploads.github.com"
	defaultGitHubServerHost = "github.com"
)

// trailingDot is the FQDN trailing-dot separator normalizeHost/normalizeHostForScheme strip
// from a hostname before comparison, e.g. "ghes.example.com." -> "ghes.example.com".
const trailingDot = "."

// Endpoints describes where a set of GitHub (or GitHub Enterprise Server) resources live:
// the web UI/clone host, the REST API host, and the derived upload host. Two independent
// concerns resolve to their own Endpoints value:
//
//   - RepoEndpoints reads GITHUB_SERVER_URL / GITHUB_API_URL — where the *user's*
//     repositories live (CI provider, imports/vendoring, releases/tags/artifacts API,
//     token host allowlist).
//   - ToolchainEndpoints reads ATMOS_TOOLCHAIN_GITHUB_URL / ATMOS_TOOLCHAIN_GITHUB_API_URL —
//     where toolchain release assets live. This is deliberately separate: toolchain assets
//     (and the aqua-registry mirror, resolved independently by the aqua package) live on
//     public github.com even for GHES users, so the toolchain must not follow the repo vars
//     (doing so would break `atmos toolchain install` on every GHES runner).
//
// Both env vars, on both constructors, default to public GitHub.com so behavior is
// byte-identical to today when unset.
type Endpoints struct {
	// ServerURL is the web/clone host, e.g. "https://github.com" or "https://ghes.example.com".
	ServerURL string
	// APIURL is the REST API base, e.g. "https://api.github.com" or "https://ghes.example.com/api/v3".
	APIURL string
	// UploadURL is the API host used for release asset uploads, e.g. "https://uploads.github.com"
	// on github.com, or "<ServerURL>/api/uploads" on GHES.
	UploadURL string
	// Host is the normalized host of ServerURL (lowercased, no trailing dot, no default port).
	// It may still carry a non-default port (e.g. "ghes.example.com:8443") -- normalizeHost
	// only strips the default 80/443 ports, so a GHES host (or test mock) reachable only on a
	// non-default port keeps that port here and can still match its own URLs via IsHost.
	Host string
}

// RepoEndpoints resolves the endpoints for the user's own repositories from the standard
// GITHUB_SERVER_URL / GITHUB_API_URL environment variables (the same variables GitHub
// Actions exports on both github.com and GitHub Enterprise Server runners). Consumed by
// the CI provider, imports/vendoring raw fetches, the pkg/github API client, the token
// host allowlist, and token-injection host recognition.
func RepoEndpoints() Endpoints {
	defer perf.Track(nil, "github.RepoEndpoints")()

	serverURL := ResolveEndpointURL("GITHUB_SERVER_URL", defaultGitHubServerURL)
	apiURL := ResolveEndpointURL("GITHUB_API_URL", defaultAPIURLFor(serverURL))

	return newEndpoints(serverURL, apiURL)
}

// ToolchainEndpoints resolves the endpoints for toolchain release assets and archives from
// ATMOS_TOOLCHAIN_GITHUB_URL / ATMOS_TOOLCHAIN_GITHUB_API_URL. These are separate from
// RepoEndpoints because aqua-registry tools are hosted on public github.com even when the
// user's own repositories live on a GitHub Enterprise Server. Useful for corporate release
// proxies/mirrors that front public GitHub releases.
func ToolchainEndpoints() Endpoints {
	defer perf.Track(nil, "github.ToolchainEndpoints")()

	serverURL := ResolveEndpointURL("ATMOS_TOOLCHAIN_GITHUB_URL", defaultGitHubServerURL)
	apiURL := ResolveEndpointURL("ATMOS_TOOLCHAIN_GITHUB_API_URL", defaultAPIURLFor(serverURL))

	return newEndpoints(serverURL, apiURL)
}

// defaultAPIURLFor returns the API URL to fall back to when the caller's API-URL environment
// variable is unset or invalid, given the already-resolved server URL. For the public
// github.com server host this is the public API (api.github.com); for any other (GitHub
// Enterprise Server) host it derives "<serverURL>/api/v3" instead of the public API. Falling
// back to the public API for a GHES server host would be a token-exposure bug: an authenticated
// client built from the resulting Endpoints would send the GHES-scoped token to
// api.github.com. GHES's REST API is conventionally served at "/api/v3" under the same host as
// the web UI, so this default matches GITHUB_SERVER_URL/GITHUB_API_URL as GitHub Actions itself
// exports them on GHES runners.
func defaultAPIURLFor(serverURL string) string {
	defer perf.Track(nil, "github.defaultAPIURLFor")()

	if hostOf(serverURL) == defaultGitHubServerHost {
		return defaultGitHubAPIURL
	}
	return serverURL + "/api/v3"
}

// newEndpoints builds an Endpoints value from already-validated server/API URLs, deriving
// Host and UploadURL.
func newEndpoints(serverURL, apiURL string) Endpoints {
	host := hostOf(serverURL)

	uploadURL := defaultGitHubUploadURL
	if host != defaultGitHubServerHost {
		uploadURL = serverURL + "/api/uploads"
	}

	return Endpoints{
		ServerURL: serverURL,
		APIURL:    apiURL,
		UploadURL: uploadURL,
		Host:      host,
	}
}

// ResolveEndpointURL reads envVar and returns its value trimmed of a trailing slash, or
// fallback when the variable is unset or its value fails to parse as an absolute HTTP(S)
// URL, or when it carries a query string, fragment, or userinfo component. A base URL is only
// ever used as a prefix that owner/repo/ref/path segments are appended to (see e.g. RawURL,
// ReleaseAssetURL, ArchiveURL): a RawQuery would silently vanish once those segments are
// appended after it (net/url's String() places the query after the whole path), a Fragment
// would do the same, and a User component would leak credentials into every URL built from it
// and complicate host comparisons that assume a bare authority. Invalid values are never fatal:
// they are logged at debug level and the caller falls back to the default endpoint, matching
// today's behavior for anyone not opting into GHES. Exported because it is shared by the
// toolchain registries (e.g. the aqua package's RegistryBaseURL) in addition to RepoEndpoints
// and ToolchainEndpoints above.
//
// http:// is accepted here on purpose: the acceptance and unit test suites point these
// endpoints at local httptest/httpmock servers over plain HTTP, and resolution must keep
// working for them. Accepting http here is not itself a credential leak -- the leak would be
// attaching a token to a request built against a non-https endpoint, which is prevented
// centrally by TokenForEndpoints (and newGitHubClientForEndpoints, which calls it), not by
// rejecting http endpoints here.
//
//nolint:forbidigo // Direct env lookup required to resolve GitHub/GHES/toolchain endpoints; mirrors pkg/http/client.go's justification for the same variables.
func ResolveEndpointURL(envVar, fallback string) string {
	defer perf.Track(nil, "github.ResolveEndpointURL")()

	value := os.Getenv(envVar)
	if value == "" {
		return fallback
	}

	trimmed := strings.TrimRight(value, "/")
	parsed, err := url.ParseRequestURI(trimmed)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.RawQuery != "" || parsed.Fragment != "" || parsed.User != nil {
		log.Debug("Invalid GitHub endpoint URL; falling back to default",
			"env", envVar, "value", value, "default", fallback, "error", errors.Join(errUtils.ErrInvalidGitHubEndpointURL, err))
		return fallback
	}

	return trimmed
}

// hostOf returns the normalized host (hostname, plus a non-default port when present) of
// rawURL, or "" if rawURL cannot be parsed. Uses parsed.Host (not Hostname()) so a non-default
// port configured on GITHUB_SERVER_URL/ATMOS_TOOLCHAIN_GITHUB_URL etc. survives into the
// resulting Endpoints.Host: otherwise a GHES host (or test mock) reachable only on a
// non-default port could never match its own URLs via IsHost/IsAPIHost, since normalizeHost
// only strips the default 80/443 ports.
func hostOf(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return normalizeHost(parsed.Host)
}

// normalizeHost canonicalizes a hostname for allowlist/equality comparison: it lower-cases
// the string, strips a trailing dot (FQDN form), and removes default HTTP/HTTPS ports so
// that "ghes.example.com:443" is treated identically to "ghes.example.com".
//
// The host/port split happens before the trailing dot is trimmed so that a dotted FQDN with
// an explicit port (e.g. "ghes.example.com.:8443") is normalized correctly instead of leaving
// the dot embedded ahead of the port.
//
// This mirrors pkg/http/client.go's normalizeHost. It is duplicated rather than imported
// because pkg/github already imports pkg/http (for GetGitHubTokenFromEnv), and pkg/http
// importing pkg/github back would create an import cycle.
func normalizeHost(host string) string {
	host = strings.ToLower(host)
	// A bare bracketed IPv6 literal ("[::1]") has no port to split, so unbracket it here; the
	// port-bearing form below yields the same unbracketed host from SplitHostPort, keeping
	// "[::1]" and "[::1]:443" equal.
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	}

	if h, port, err := net.SplitHostPort(host); err == nil {
		h = strings.TrimSuffix(h, trailingDot)
		if port == "443" || port == "80" {
			return h
		}
		return net.JoinHostPort(h, port)
	}

	return strings.TrimSuffix(host, trailingDot)
}

// IsHost reports whether host (case-insensitive, with port and trailing dot normalized)
// matches this Endpoints value's own Host. It intentionally does not also match
// "github.com": callers that need to recognize both this endpoint's host and public
// github.com (e.g. because toolchain assets are always public even under GHES) combine
// IsHost with an explicit github.com check.
func (e Endpoints) IsHost(host string) bool {
	defer perf.Track(nil, "github.Endpoints.IsHost")()

	return normalizeHost(host) == e.Host
}

// normalizeHostForScheme is normalizeHost's scheme-aware sibling: it lower-cases the host,
// strips a trailing dot, and removes a port only when it is the *default* port for scheme
// (443 for https, 80 for http) instead of unconditionally stripping both. This matters for any
// caller deciding whether it is safe to attach a bearer token to a request: normalizeHost's
// blanket stripping of both 80 and 443 means "https://host:80" normalizes identically to
// "https://host" (whose default port for https is 443, not 80), so a request explicitly
// targeting port 80 over "https://" would wrongly be treated as the plain configured host and
// could receive the token. Comparing with the port that is actually the default for the
// request's own scheme closes that gap; an explicit non-default port must still match exactly.
func normalizeHostForScheme(host, scheme string) string {
	host = strings.ToLower(host)

	if h, port, err := net.SplitHostPort(host); err == nil {
		h = strings.TrimSuffix(h, trailingDot)
		if port == defaultPortForScheme(scheme) {
			return h
		}
		return net.JoinHostPort(h, port)
	}

	return strings.TrimSuffix(host, trailingDot)
}

// defaultPortForScheme returns the default port for scheme ("443" for https, "80" for http), or
// "" for any other scheme, which never matches a real port and so never causes
// normalizeHostForScheme to strip one.
func defaultPortForScheme(scheme string) string {
	switch strings.ToLower(scheme) {
	case "https":
		return "443"
	case "http":
		return "80"
	default:
		return ""
	}
}

// IsHostForScheme is IsHost's scheme-aware sibling (see normalizeHostForScheme): it strips only
// the default port for scheme instead of unconditionally stripping both 80 and 443, so an
// explicit non-default port (e.g. "host:80" for a "https" request) never matches the bare
// configured host. Callers deciding whether to attach a bearer/OAuth token to a request -- where
// the scheme is already known -- MUST use this instead of IsHost.
func (e Endpoints) IsHostForScheme(host, scheme string) bool {
	defer perf.Track(nil, "github.Endpoints.IsHostForScheme")()

	return normalizeHostForScheme(host, scheme) == normalizeHostForScheme(e.Host, scheme)
}

// IsAPIHostForScheme is IsAPIHost's scheme-aware sibling (see normalizeHostForScheme). Callers
// deciding whether to attach a bearer/OAuth token to a request MUST use this instead of
// IsAPIHost.
func (e Endpoints) IsAPIHostForScheme(host, scheme string) bool {
	defer perf.Track(nil, "github.Endpoints.IsAPIHostForScheme")()

	return normalizeHostForScheme(host, scheme) == normalizeHostForScheme(hostOf(e.APIURL), scheme)
}

// IsUploadHostForScheme is IsUploadHost's scheme-aware sibling (see normalizeHostForScheme).
// Callers deciding whether to attach a bearer/OAuth token to a request MUST use this instead of
// IsUploadHost.
func (e Endpoints) IsUploadHostForScheme(host, scheme string) bool {
	defer perf.Track(nil, "github.Endpoints.IsUploadHostForScheme")()

	return normalizeHostForScheme(host, scheme) == normalizeHostForScheme(hostOf(e.UploadURL), scheme)
}

// Hostname returns e.Host with any port stripped, e.g. "ghes.example.com:8443" becomes
// "ghes.example.com". Host deliberately keeps a non-default port (see its doc comment) so
// IsHost can match a GHES instance reachable only on a non-default port; callers that need to
// compare against a portless value instead -- e.g. the host captured from an SCP-style Git
// remote (git@host:org/repo.git), which carries no port of its own -- use this instead of
// comparing against Host directly.
func (e Endpoints) Hostname() string {
	defer perf.Track(nil, "github.Endpoints.Hostname")()

	if h, _, err := net.SplitHostPort(e.Host); err == nil {
		return h
	}
	return e.Host
}

// IsAPIHost reports whether host (case-insensitive, with port and trailing dot normalized)
// matches this Endpoints value's API host (derived from APIURL). This can differ from IsHost
// (ServerURL's host) for a corporate mirror that fronts the web/clone host and the API host
// separately, e.g. ATMOS_TOOLCHAIN_GITHUB_URL and ATMOS_TOOLCHAIN_GITHUB_API_URL pointing at
// different hosts. Callers that authenticate requests sent to APIURL (rather than ServerURL)
// should check this in addition to IsHost.
func (e Endpoints) IsAPIHost(host string) bool {
	defer perf.Track(nil, "github.Endpoints.IsAPIHost")()

	return normalizeHost(host) == hostOf(e.APIURL)
}

// IsUploadHost reports whether host (case-insensitive, with port and trailing dot normalized)
// matches this Endpoints value's upload host (derived from UploadURL). This can differ from
// both IsHost and IsAPIHost, e.g. on public github.com uploads go to the separate
// uploads.github.com host. Callers that authenticate requests sent for release-asset uploads
// should check this in addition to IsHost/IsAPIHost.
func (e Endpoints) IsUploadHost(host string) bool {
	defer perf.Track(nil, "github.Endpoints.IsUploadHost")()

	return normalizeHost(host) == hostOf(e.UploadURL)
}

// isDefaultGitHubCom reports whether these endpoints point at public GitHub.com.
func (e Endpoints) isDefaultGitHubCom() bool {
	return e.Host == defaultGitHubServerHost
}

// AllowsToken reports whether it is safe to attach a bearer/OAuth token to a request against
// this Endpoints value's API host: only when APIURL resolves to an https URL. Sending a token
// over plain HTTP would put it on the wire in cleartext. ResolveEndpointURL intentionally
// accepts http:// (the acceptance/unit test suites point endpoints at local httptest/httpmock
// servers over plain HTTP), so those endpoints must still resolve -- they simply must never be
// paired with a token.
func (e Endpoints) AllowsToken() bool {
	defer perf.Track(nil, "github.Endpoints.AllowsToken")()

	parsed, err := url.Parse(e.APIURL)
	return err == nil && strings.EqualFold(parsed.Scheme, "https")
}

// TokenForEndpoints returns token unchanged when endpoints.AllowsToken() (its API host is
// https), or "" otherwise, logging at debug level so a non-https endpoint that withholds a
// configured token is diagnosable rather than silently degrading to anonymous access. This
// centralizes the "never send a token over http" rule for every caller that builds an
// authenticated GitHub request or client from RepoEndpoints()/ToolchainEndpoints().
func TokenForEndpoints(endpoints Endpoints, token string) string {
	defer perf.Track(nil, "github.TokenForEndpoints")()

	if token == "" || endpoints.AllowsToken() {
		return token
	}

	log.Debug("GitHub API endpoint is not https; sending unauthenticated requests",
		"apiURL", endpoints.APIURL)
	return ""
}

// escapePathSegments percent-encodes each "/"-separated segment of path independently and
// rejoins them with "/", so a reserved character (e.g. "#", "?") supplied within one logical
// path segment is escaped as data rather than truncating or reinterpreting the URL, while a
// genuine multi-segment path (e.g. "dir/file.yaml") still produces one URL segment per
// directory component as intended.
func escapePathSegments(path string) string {
	if path == "" {
		return ""
	}
	segments := strings.Split(path, "/")
	for i, seg := range segments {
		segments[i] = url.PathEscape(seg)
	}
	return strings.Join(segments, "/")
}

// RawURL builds the URL for fetching a file's raw content at ref from owner/repo.
// On github.com this is raw.githubusercontent.com; on GitHub Enterprise Server, raw
// content is served from the same host under /raw/ instead of a separate subdomain.
// Path may be empty (returns the ref root) and any leading slash is stripped.
//
// Owner, repo, and ref are percent-encoded as single opaque path segments -- a "/" occurring
// within one of them (e.g. a malicious or malformed ref) is escaped as data ("%2F") rather
// than reinterpreted as an additional path separator. Path is a genuine multi-segment file
// path, so each "/"-separated segment is escaped independently instead.
func (e Endpoints) RawURL(owner, repo, ref, path string) string {
	defer perf.Track(nil, "github.Endpoints.RawURL")()

	owner = url.PathEscape(owner)
	repo = url.PathEscape(repo)
	ref = url.PathEscape(ref)
	path = escapePathSegments(strings.TrimPrefix(path, "/"))

	if e.isDefaultGitHubCom() {
		if path == "" {
			return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s", owner, repo, ref)
		}
		return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/%s", owner, repo, ref, path)
	}

	if path == "" {
		return fmt.Sprintf("%s/raw/%s/%s/%s", e.ServerURL, owner, repo, ref)
	}
	return fmt.Sprintf("%s/raw/%s/%s/%s/%s", e.ServerURL, owner, repo, ref, path)
}

// ReleaseAssetURL builds the URL for downloading a release asset. The path shape
// (`/<owner>/<repo>/releases/download/<tag>/<asset>`) is identical on github.com and GHES.
// Each component is percent-encoded as a single opaque path segment.
func (e Endpoints) ReleaseAssetURL(owner, repo, tag, asset string) string {
	defer perf.Track(nil, "github.Endpoints.ReleaseAssetURL")()

	return fmt.Sprintf("%s/%s/%s/releases/download/%s/%s",
		e.ServerURL, url.PathEscape(owner), url.PathEscape(repo), url.PathEscape(tag), url.PathEscape(asset))
}

// ArchiveURL builds the URL for downloading a tag's source archive
// (`/<owner>/<repo>/archive/refs/tags/<tag>.tar.gz`), identical on github.com and GHES. Each
// component is percent-encoded as a single opaque path segment.
func (e Endpoints) ArchiveURL(owner, repo, tag string) string {
	defer perf.Track(nil, "github.Endpoints.ArchiveURL")()

	return fmt.Sprintf("%s/%s/%s/archive/refs/tags/%s.tar.gz",
		e.ServerURL, url.PathEscape(owner), url.PathEscape(repo), url.PathEscape(tag))
}
