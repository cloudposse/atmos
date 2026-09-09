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

	// This mirrors defaultAquaRegistryBaseURL in pkg/toolchain/registry/aqua/aqua.go. It is
	// duplicated here (rather than imported) because pkg/toolchain/registry/aqua already
	// imports this package, and importing it back would create a cycle. Keep both literals in
	// sync if the upstream registry moves.
	defaultAquaRegistryURL = "https://raw.githubusercontent.com/aquaproj/aqua-registry/main"
)

// Endpoints describes where a set of GitHub (or GitHub Enterprise Server) resources live:
// the web UI/clone host, the REST API host, and the derived upload host. Two independent
// concerns resolve to their own Endpoints value:
//
//   - RepoEndpoints reads GITHUB_SERVER_URL / GITHUB_API_URL — where the *user's*
//     repositories live (CI provider, imports/vendoring, releases/tags/artifacts API,
//     token host allowlist).
//   - ToolchainEndpoints reads ATMOS_TOOLCHAIN_GITHUB_URL / ATMOS_TOOLCHAIN_GITHUB_API_URL —
//     where toolchain release assets and the aqua-registry mirror live. These are
//     deliberately separate: aqua-registry tools live on public github.com even for GHES
//     users, so the toolchain must not follow the repo vars (doing so would break
//     `atmos toolchain install` on every GHES runner).
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
	// Host is the normalized hostname of ServerURL (lowercased, no trailing dot, no default port).
	Host string
}

// RepoEndpoints resolves the endpoints for the user's own repositories from the standard
// GITHUB_SERVER_URL / GITHUB_API_URL environment variables (the same variables GitHub
// Actions exports on both github.com and GitHub Enterprise Server runners). Consumed by
// the CI provider, imports/vendoring raw fetches, the pkg/github API client, the token
// host allowlist, and token-injection host recognition.
func RepoEndpoints() Endpoints {
	defer perf.Track(nil, "github.RepoEndpoints")()

	serverURL := resolveEndpointURL("GITHUB_SERVER_URL", defaultGitHubServerURL)
	apiURL := resolveEndpointURL("GITHUB_API_URL", defaultGitHubAPIURL)

	return newEndpoints(serverURL, apiURL)
}

// ToolchainEndpoints resolves the endpoints for toolchain release assets and archives from
// ATMOS_TOOLCHAIN_GITHUB_URL / ATMOS_TOOLCHAIN_GITHUB_API_URL. These are separate from
// RepoEndpoints because aqua-registry tools are hosted on public github.com even when the
// user's own repositories live on a GitHub Enterprise Server. Useful for corporate release
// proxies/mirrors that front public GitHub releases.
func ToolchainEndpoints() Endpoints {
	defer perf.Track(nil, "github.ToolchainEndpoints")()

	serverURL := resolveEndpointURL("ATMOS_TOOLCHAIN_GITHUB_URL", defaultGitHubServerURL)
	apiURL := resolveEndpointURL("ATMOS_TOOLCHAIN_GITHUB_API_URL", defaultGitHubAPIURL)

	return newEndpoints(serverURL, apiURL)
}

// AquaRegistryURL resolves the base URL of the aqua-registry raw content mirror from
// ATMOS_TOOLCHAIN_AQUA_REGISTRY_URL, defaulting to the upstream aquaproj/aqua-registry
// repository on raw.githubusercontent.com. It serves the top-level registry.yaml index and
// the per-package pkgs/<name>/registry.yaml files.
func AquaRegistryURL() string {
	defer perf.Track(nil, "github.AquaRegistryURL")()

	return resolveEndpointURL("ATMOS_TOOLCHAIN_AQUA_REGISTRY_URL", defaultAquaRegistryURL)
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

// resolveEndpointURL reads envVar and returns its value trimmed of a trailing slash, or
// fallback when the variable is unset or its value fails to parse as an absolute HTTP(S)
// URL. Invalid values are never fatal: they are logged at debug level and the caller falls
// back to the default endpoint, matching today's behavior for anyone not opting into GHES.
//
//nolint:forbidigo // Direct env lookup required to resolve GitHub/GHES/toolchain endpoints; mirrors pkg/http/client.go's justification for the same variables.
func resolveEndpointURL(envVar, fallback string) string {
	defer perf.Track(nil, "github.resolveEndpointURL")()

	value := os.Getenv(envVar)
	if value == "" {
		return fallback
	}

	trimmed := strings.TrimRight(value, "/")
	parsed, err := url.ParseRequestURI(trimmed)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		log.Debug("Invalid GitHub endpoint URL; falling back to default",
			"env", envVar, "value", value, "default", fallback, "error", errors.Join(errUtils.ErrInvalidGitHubEndpointURL, err))
		return fallback
	}

	return trimmed
}

// hostOf returns the normalized hostname of rawURL, or "" if rawURL cannot be parsed.
func hostOf(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return normalizeHost(parsed.Hostname())
}

// normalizeHost canonicalizes a hostname for allowlist/equality comparison: it lower-cases
// the string, strips a trailing dot (FQDN form), and removes default HTTP/HTTPS ports so
// that "ghes.example.com:443" is treated identically to "ghes.example.com".
//
// This mirrors pkg/http/client.go's normalizeHost. It is duplicated rather than imported
// because pkg/github already imports pkg/http (for GetGitHubTokenFromEnv), and pkg/http
// importing pkg/github back would create an import cycle.
func normalizeHost(host string) string {
	host = strings.ToLower(host)
	host = strings.TrimSuffix(host, ".")
	if h, port, err := net.SplitHostPort(host); err == nil && (port == "443" || port == "80") {
		host = strings.TrimSuffix(h, ".")
	}
	return host
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

// isDefaultGitHubCom reports whether these endpoints point at public GitHub.com.
func (e Endpoints) isDefaultGitHubCom() bool {
	return e.Host == defaultGitHubServerHost
}

// RawURL builds the URL for fetching a file's raw content at ref from owner/repo.
// On github.com this is raw.githubusercontent.com; on GitHub Enterprise Server, raw
// content is served from the same host under /raw/ instead of a separate subdomain.
// Path may be empty (returns the ref root) and any leading slash is stripped.
func (e Endpoints) RawURL(owner, repo, ref, path string) string {
	defer perf.Track(nil, "github.Endpoints.RawURL")()

	path = strings.TrimPrefix(path, "/")

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
func (e Endpoints) ReleaseAssetURL(owner, repo, tag, asset string) string {
	defer perf.Track(nil, "github.Endpoints.ReleaseAssetURL")()

	return fmt.Sprintf("%s/%s/%s/releases/download/%s/%s", e.ServerURL, owner, repo, tag, asset)
}

// ArchiveURL builds the URL for downloading a tag's source archive
// (`/<owner>/<repo>/archive/refs/tags/<tag>.tar.gz`), identical on github.com and GHES.
func (e Endpoints) ArchiveURL(owner, repo, tag string) string {
	defer perf.Track(nil, "github.Endpoints.ArchiveURL")()

	return fmt.Sprintf("%s/%s/%s/archive/refs/tags/%s.tar.gz", e.ServerURL, owner, repo, tag)
}
