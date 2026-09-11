package httpmock

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

const (
	// Default pagination size, matching GitHub's own default per_page.
	defaultPerPage = 30
	// Number of "/"-separated segments in /{owner}/{repo}/releases/download/{tag}/{asset}
	// and /{owner}/{repo}/archive/refs/tags/{tag}.tar.gz.
	downloadPathSegments = 6
	// Index of the release tag segment in a download path's "/"-split segments.
	downloadTagIndex = 4
	// Index of the release asset segment in a download path's "/"-split segments.
	downloadAssetIndex = 5
)

// ReleaseSpec describes one fake GitHub release served by the mock's
// /api/v3/repos/{owner}/{repo}/releases[/latest] endpoints.
type ReleaseSpec struct {
	TagName    string
	Prerelease bool
	Draft      bool
}

// releaseJSON is the subset of GitHub's release JSON shape that go-github (pkg/github) and
// the aqua registry's hand-rolled JSON decoding (pkg/toolchain/registry/aqua/version.go) read:
// tag_name, prerelease, draft.
type releaseJSON struct {
	TagName    string `json:"tag_name"`
	Name       string `json:"name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
}

// tagJSON is the subset of GitHub's tag JSON shape aqua's getLatestTag reads: name.
type tagJSON struct {
	Name string `json:"name"`
}

// RegisterRelease appends one fake release for owner/repo, in the order GitHub's "most recent
// first" release list is normally returned in: register newest first for GetLatestVersion/
// GetLatestRelease-style "first non-draft, non-prerelease wins" semantics to behave as expected.
func (m *GitHubMockServer) RegisterRelease(owner, repo string, spec ReleaseSpec) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := owner + "/" + repo
	m.releases[key] = append(m.releases[key], spec)
}

// RegisterTag appends one fake tag name for owner/repo.
func (m *GitHubMockServer) RegisterTag(owner, repo, name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := owner + "/" + repo
	m.tags[key] = append(m.tags[key], name)
}

// apiRepoRoute identifies which /api/v3/repos/{owner}/{repo}/{resource}[/...] handler serves
// a request, or that none does.
type apiRepoRoute int

const (
	apiRepoRouteNone apiRepoRoute = iota
	apiRepoRouteLatestRelease
	apiRepoRouteReleasesList
	apiRepoRouteTagsList
)

// classifyAPIRepoRoute maps a request's "/"-split path segments (as returned for
// /api/v3/repos/{owner}/{repo}/{resource}[/...]) to the handler that serves it. Requires an
// exact segment count per endpoint (rather than e.g. "at least 4") so malformed paths like
// "/releases/123" or "/tags/extra" 404 instead of silently matching a collection route, which
// would hide an incorrect GitHub endpoint construction in the code under test.
func classifyAPIRepoRoute(parts []string) apiRepoRoute {
	resource := parts[2]
	switch {
	case resource == "releases" && len(parts) == 4 && parts[3] == "latest":
		return apiRepoRouteLatestRelease
	case resource == "releases" && len(parts) == 3:
		return apiRepoRouteReleasesList
	case resource == "tags" && len(parts) == 3:
		return apiRepoRouteTagsList
	default:
		return apiRepoRouteNone
	}
}

// tryAPIRepos handles GET /api/v3/repos/{owner}/{repo}/releases[/latest] and
// /api/v3/repos/{owner}/{repo}/tags. Returns false (unhandled) for any other path so the
// caller can continue trying other routes.
func (m *GitHubMockServer) tryAPIRepos(w http.ResponseWriter, r *http.Request) bool {
	const prefix = "/api/v3/repos/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		return false
	}

	parts := strings.Split(strings.TrimPrefix(r.URL.Path, prefix), "/")
	if len(parts) < 3 {
		http.NotFound(w, r)
		return true
	}
	owner, repo := parts[0], parts[1]
	key := owner + "/" + repo

	switch classifyAPIRepoRoute(parts) {
	case apiRepoRouteLatestRelease:
		m.writeLatestRelease(w, r, key)
	case apiRepoRouteReleasesList:
		m.writeReleasesList(w, r, key)
	case apiRepoRouteTagsList:
		m.writeTagsList(w, r, key)
	default:
		http.NotFound(w, r)
	}
	return true
}

// writeLatestRelease serves the first registered non-draft, non-prerelease release for key,
// matching GitHub's semantics for GET .../releases/latest (which excludes drafts/prereleases).
func (m *GitHubMockServer) writeLatestRelease(w http.ResponseWriter, r *http.Request, key string) {
	m.mu.Lock()
	releases := m.releases[key]
	m.mu.Unlock()

	for _, rel := range releases {
		if !rel.Draft && !rel.Prerelease {
			writeJSON(w, releaseJSON{TagName: rel.TagName, Name: rel.TagName})
			return
		}
	}
	http.NotFound(w, r)
}

// writeReleasesList serves a page of the registered releases for key, honoring the `page`
// and `per_page` query params GitHub's API accepts, and sets a Link: rel="next" header when
// more pages remain -- exercising the same pagination-following code
// (pkg/toolchain/registry/aqua/version.go parseNextLink) that talks to the real API.
func (m *GitHubMockServer) writeReleasesList(w http.ResponseWriter, r *http.Request, key string) {
	m.mu.Lock()
	releases := m.releases[key]
	m.mu.Unlock()

	page := queryInt(r, "page", 1)
	perPage := queryInt(r, "per_page", defaultPerPage)

	// Clamp page to the available page count before computing start/end: a page value beyond
	// that range multiplied by perPage can otherwise overflow int and produce a negative start,
	// which would panic on the releases[start:end] slice below.
	totalPages := (len(releases) + perPage - 1) / perPage
	if totalPages < 1 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}

	start := (page - 1) * perPage
	end := start + perPage
	if start > len(releases) {
		start = len(releases)
	}
	if end > len(releases) {
		end = len(releases)
	}

	if end < len(releases) {
		nextURL := fmt.Sprintf("%s%s?page=%d&per_page=%d", m.Server.URL, r.URL.Path, page+1, perPage)
		w.Header().Set("Link", fmt.Sprintf(`<%s>; rel="next"`, nextURL))
	}

	pageItems := releases[start:end]
	out := make([]releaseJSON, len(pageItems))
	for i, rel := range pageItems {
		out[i] = releaseJSON{TagName: rel.TagName, Name: rel.TagName, Draft: rel.Draft, Prerelease: rel.Prerelease}
	}
	writeJSON(w, out)
}

// writeTagsList serves the registered tags for key, honoring `per_page` (aqua's getLatestTag
// requests per_page=1 and reads only tags[0]).
func (m *GitHubMockServer) writeTagsList(w http.ResponseWriter, r *http.Request, key string) {
	m.mu.Lock()
	tags := m.tags[key]
	m.mu.Unlock()

	perPage := queryInt(r, "per_page", defaultPerPage)
	if perPage < len(tags) {
		tags = tags[:perPage]
	}

	out := make([]tagJSON, len(tags))
	for i, name := range tags {
		out[i] = tagJSON{Name: name}
	}
	writeJSON(w, out)
}

// archiveSuffix is the required trailing extension of an archive download path
// (/{owner}/{repo}/archive/refs/tags/{tag}.tar.gz); without it the path is malformed and must
// not be served, even if every other segment matches.
const archiveSuffix = ".tar.gz"

// tryReleaseDownload handles GET /{owner}/{repo}/releases/download/{tag}/{asset} and
// GET /{owner}/{repo}/archive/refs/tags/{tag}.tar.gz -- the web-host (not API-host) shapes
// pkg/github.Endpoints.ReleaseAssetURL/ArchiveURL build. Returns false (unhandled) for any
// other path.
func (m *GitHubMockServer) tryReleaseDownload(w http.ResponseWriter, r *http.Request) bool {
	segments := strings.Split(strings.Trim(r.URL.Path, "/"), "/")

	if len(segments) == downloadPathSegments && segments[2] == "releases" && segments[3] == "download" {
		return m.tryReleaseAssetDownload(w, r, segments)
	}
	if isArchiveDownloadPath(segments) {
		return m.tryArchiveDownload(w, r, segments)
	}
	return false
}

// isArchiveDownloadPath reports whether segments is a well-formed
// /{owner}/{repo}/archive/refs/tags/{tag}.tar.gz path.
func isArchiveDownloadPath(segments []string) bool {
	return len(segments) == downloadPathSegments &&
		segments[2] == "archive" && segments[3] == "refs" && segments[4] == "tags" &&
		strings.HasSuffix(segments[downloadAssetIndex], archiveSuffix)
}

// tryReleaseAssetDownload serves a registered release asset for a path already matched as
// /{owner}/{repo}/releases/download/{tag}/{asset}, always reporting true (handled): either the
// asset bytes, or a 404 when unregistered.
func (m *GitHubMockServer) tryReleaseAssetDownload(w http.ResponseWriter, r *http.Request, segments []string) bool {
	owner, repo, tag, asset := segments[0], segments[1], segments[downloadTagIndex], segments[downloadAssetIndex]
	key := strings.Join([]string{owner, repo, tag, asset}, "/")
	m.mu.Lock()
	data, ok := m.assets[key]
	m.mu.Unlock()
	if !ok {
		http.NotFound(w, r)
		return true
	}
	writeBytes(w, data, "")
	return true
}

// tryArchiveDownload serves a registered tag archive for a path already matched by
// isArchiveDownloadPath, always reporting true (handled): either the archive bytes, or a 404
// when unregistered.
func (m *GitHubMockServer) tryArchiveDownload(w http.ResponseWriter, r *http.Request, segments []string) bool {
	owner, repo := segments[0], segments[1]
	tag := strings.TrimSuffix(segments[downloadAssetIndex], archiveSuffix)
	key := strings.Join([]string{owner, repo, tag}, "/")
	m.mu.Lock()
	data, ok := m.archives[key]
	m.mu.Unlock()
	if !ok {
		http.NotFound(w, r)
		return true
	}
	writeBytes(w, data, "application/gzip")
	return true
}

// RegisterReleaseAsset registers data to be served at
// /{owner}/{repo}/releases/download/{tag}/{asset}.
func (m *GitHubMockServer) RegisterReleaseAsset(owner, repo, tag, asset string, data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := strings.Join([]string{owner, repo, tag, asset}, "/")
	m.assets[key] = data
}

// RegisterArchive registers data to be served at
// /{owner}/{repo}/archive/refs/tags/{tag}.tar.gz.
func (m *GitHubMockServer) RegisterArchive(owner, repo, tag string, data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := strings.Join([]string{owner, repo, tag}, "/")
	m.archives[key] = data
}

// RegisterRawFile registers content to be served at /raw/{owner}/{repo}/{ref}/{path}, the
// GitHub Enterprise Server raw-content shape pkg/github.Endpoints.RawURL builds when
// GITHUB_SERVER_URL resolves to a non-default (non-github.com) host. Unlike the legacy
// RegisterFile suffix match, this is an exact match on owner/repo/ref/path.
func (m *GitHubMockServer) RegisterRawFile(owner, repo, ref, path, content string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := strings.Join([]string{owner, repo, ref, strings.TrimPrefix(path, "/")}, "/")
	m.rawFiles[key] = content
}

// tryRaw handles GET /raw/{owner}/{repo}/{ref}/{path} via an exact RegisterRawFile match.
// Returns false (unhandled, not 404) on a miss so the caller falls through to the legacy
// suffix-matched file map, preserving backward compatibility for callers that only ever
// called RegisterFile.
func (m *GitHubMockServer) tryRaw(w http.ResponseWriter, r *http.Request) bool {
	const prefix = "/raw/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		return false
	}

	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, prefix), "/", 4) //nolint:mnd // owner/repo/ref/path.
	if len(parts) < 3 {
		return false
	}
	owner, repo, ref := parts[0], parts[1], parts[2]
	path := ""
	if len(parts) == 4 { //nolint:mnd // owner/repo/ref/path.
		path = parts[3]
	}
	key := strings.Join([]string{owner, repo, ref, path}, "/")

	m.mu.Lock()
	content, ok := m.rawFiles[key]
	m.mu.Unlock()
	if !ok {
		return false
	}
	writeBytes(w, []byte(content), "")
	return true
}

// queryInt reads an integer query parameter, falling back to def when absent or unparseable.
func queryInt(r *http.Request, name string, def int) int {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return def
	}
	return v
}

// writeJSON writes v as a JSON response body with a 200 status.
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(v)
}

// writeBytes writes data as a 200 response body. When contentType is non-empty it is set as
// the Content-Type header; otherwise the default (application/octet-stream, via
// http.DetectContentType-free plain write) is left to the client to interpret -- fine for a
// fake binary/archive asset a test only checks for existence, never executes or parses.
func writeBytes(w http.ResponseWriter, data []byte, contentType string) {
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data) //nolint:gosec // Test mock serving registered fixture bytes, not user input.
}
