// Package github implements the CI cache Backend against the GitHub Actions
// cache (Cache Service v2). It speaks the Twirp JSON API at ACTIONS_RESULTS_URL
// (authenticated with ACTIONS_RUNTIME_TOKEN) for save/restore, uploads/downloads
// content through the returned Azure Blob SAS URLs, and uses the public REST
// caches API for list/delete.
//
// Save/restore of content require the Actions runtime token and results URL,
// which are only present inside a GitHub Actions runner; outside a runner those
// operations return errUtils.ErrCacheUnavailable. List/delete are pure cache
// administration over the public REST API and work anywhere a GitHub token and
// the repository (owner/repo) can be resolved, so NewBackend constructs an
// admin-capable backend even outside a runner.
package github

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/cache"
	"github.com/cloudposse/atmos/pkg/git"
	ghtoken "github.com/cloudposse/atmos/pkg/github"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// BackendName is the registered backend type name.
	backendName = "github/actions"

	// CacheServicePath is the Twirp service path for cache operations.
	cacheServicePath = "twirp/github.actions.results.api.v1.CacheService"

	// CacheVersionSalt is hashed to produce the immutable "version" that GitHub
	// uses to namespace cache content formats. A single salt for all Atmos
	// caches lets restore-key prefix matching work across entries.
	cacheVersionSalt = "atmos-cache-v1"

	// HTTPTimeout bounds Twirp/REST calls. Blob transfers use a longer timeout.
	httpTimeout = 30 * time.Second

	// BlobTimeout bounds blob upload/download.
	blobTimeout = 10 * time.Minute

	// GithubPaginationLimit is the max items per REST page.
	githubPaginationLimit = 100
)

// wrapErr wraps a cause with a static sentinel error, preserving both chains.
func wrapErr(sentinel, cause error) error {
	return fmt.Errorf("%w: %w", sentinel, cause)
}

// cacheClient is the Twirp client surface, extracted for testability.
type cacheClient interface {
	CreateCacheEntry(ctx context.Context, req *createCacheEntryRequest) (*createCacheEntryResponse, error)
	FinalizeCacheEntryUpload(ctx context.Context, req *finalizeCacheEntryRequest) (*finalizeCacheEntryResponse, error)
	GetCacheEntryDownloadURL(ctx context.Context, req *getCacheEntryDownloadURLRequest) (*getCacheEntryDownloadURLResponse, error)
}

type createCacheEntryRequest struct {
	Key     string `json:"key"`
	Version string `json:"version"`
}

type createCacheEntryResponse struct {
	OK              bool   `json:"ok"`
	SignedUploadURL string `json:"signed_upload_url"`
}

type finalizeCacheEntryRequest struct {
	Key       string `json:"key"`
	SizeBytes int64  `json:"size_bytes"`
	Version   string `json:"version"`
}

type finalizeCacheEntryResponse struct {
	OK      bool   `json:"ok"`
	EntryID string `json:"entry_id"`
}

type getCacheEntryDownloadURLRequest struct {
	Key         string   `json:"key"`
	RestoreKeys []string `json:"restore_keys,omitempty"`
	Version     string   `json:"version"`
}

type getCacheEntryDownloadURLResponse struct {
	OK                bool   `json:"ok"`
	SignedDownloadURL string `json:"signed_download_url"`
	MatchedKey        string `json:"matched_key"`
}

// githubCache mirrors a cache entry from the REST caches API.
type githubCache struct {
	ID          int64     `json:"id"`
	Key         string    `json:"key"`
	Version     string    `json:"version"`
	SizeInBytes int64     `json:"size_in_bytes"`
	CreatedAt   time.Time `json:"created_at"`
}

type listCachesResponse struct {
	TotalCount   int           `json:"total_count"`
	ActionsCache []githubCache `json:"actions_caches"`
}

// Backend implements cache.Backend using the GitHub Actions cache.
type Backend struct {
	client     cacheClient
	blobClient *http.Client
	restClient *http.Client
	baseURL    string
	owner      string
	repo       string
	version    string
}

// NewBackend constructs the GitHub Actions cache backend.
//
// List/delete (cache administration) work anywhere: they use the public REST
// caches API authenticated with an ordinary GitHub token and the repository's
// owner/repo (resolved from GITHUB_REPOSITORY or the local git remote). Save and
// restore of content additionally require the Actions runtime token and results
// URL (ACTIONS_RUNTIME_TOKEN + ACTIONS_RESULTS_URL), which are only present
// inside a GitHub Actions runner; when they are absent the returned backend has
// no runtime client and Save/Restore report errUtils.ErrCacheUnavailable.
func NewBackend(opts cache.Options) (cache.Backend, error) {
	defer perf.Track(opts.AtmosConfig, "githubcache.NewBackend")()

	owner, repo := resolveOwnerRepo(opts.Options)

	// Resolve the GitHub token for the REST list/delete operations using the
	// canonical Atmos resolver, so the cache respects the same precedence as the
	// rest of Atmos: --github-token flag > ATMOS_PRO_GITHUB_TOKEN >
	// ATMOS_GITHUB_TOKEN > GITHUB_TOKEN > `gh auth token`. Note that save/restore
	// of content do not use this token — they authenticate with the Actions
	// runtime token (ACTIONS_RUNTIME_TOKEN).
	restClient := newRESTClient(ghtoken.GetGitHubToken())

	sum := sha256.Sum256([]byte(cacheVersionSalt))
	version := hex.EncodeToString(sum[:])

	b := &Backend{
		blobClient: &http.Client{Timeout: blobTimeout},
		restClient: restClient,
		baseURL:    "https://api.github.com",
		owner:      owner,
		repo:       repo,
		version:    version,
	}

	// The runtime client (used only by save/restore) is available solely inside a
	// runner. When the token/URL are absent, leave it nil so the admin operations
	// (list/delete) still work; Save/Restore guard on the nil client.
	runtimeToken := os.Getenv("ACTIONS_RUNTIME_TOKEN")
	resultsURL := os.Getenv("ACTIONS_RESULTS_URL")
	if runtimeToken != "" && resultsURL != "" {
		b.client = newTwirpClient(resultsURL, runtimeToken)
	}

	return b, nil
}

// errRuntimeUnavailable builds the error returned when a save/restore operation
// is attempted outside a GitHub Actions runner (no runtime cache client).
func errRuntimeUnavailable() error {
	return errUtils.Build(errUtils.ErrCacheUnavailable).
		WithExplanation("Saving and restoring cache content runs only inside a GitHub Actions runner").
		WithHint("ACTIONS_RUNTIME_TOKEN and ACTIONS_RESULTS_URL are only set inside a runner; use `atmos ci cache list` and `atmos ci cache delete` to administer the cache locally").
		Err()
}

// Name returns the backend type name.
func (b *Backend) Name() string {
	defer perf.Track(nil, "githubcache.Name")()

	return backendName
}

// Save uploads data under key. Returns ErrCacheAlreadyExists when the key exists.
func (b *Backend) Save(ctx context.Context, key string, data io.Reader, size int64) error {
	defer perf.Track(nil, "githubcache.Save")()

	if b.client == nil {
		return errRuntimeUnavailable()
	}

	createResp, err := b.client.CreateCacheEntry(ctx, &createCacheEntryRequest{Key: key, Version: b.version})
	if err != nil {
		return err
	}
	if !createResp.OK || createResp.SignedUploadURL == "" {
		// ok=false means an entry with this key+version already exists.
		return fmt.Errorf("%w: %s", errUtils.ErrCacheAlreadyExists, key)
	}

	if err := b.uploadBlob(ctx, createResp.SignedUploadURL, data, size); err != nil {
		return err
	}

	finalizeResp, err := b.client.FinalizeCacheEntryUpload(ctx, &finalizeCacheEntryRequest{
		Key:       key,
		SizeBytes: size,
		Version:   b.version,
	})
	if err != nil {
		return err
	}
	if !finalizeResp.OK {
		return fmt.Errorf("%w: finalize rejected for key %s", errUtils.ErrCacheSaveFailed, key)
	}
	return nil
}

// Restore downloads the entry for key, falling back to restoreKeys. Returns
// ErrCacheNotFound when nothing matches.
func (b *Backend) Restore(ctx context.Context, key string, restoreKeys []string) (string, io.ReadCloser, error) {
	defer perf.Track(nil, "githubcache.Restore")()

	if b.client == nil {
		return "", nil, errRuntimeUnavailable()
	}

	resp, err := b.client.GetCacheEntryDownloadURL(ctx, &getCacheEntryDownloadURLRequest{
		Key:         key,
		RestoreKeys: restoreKeys,
		Version:     b.version,
	})
	if err != nil {
		return "", nil, err
	}
	if !resp.OK || resp.SignedDownloadURL == "" {
		return "", nil, fmt.Errorf("%w: %s", errUtils.ErrCacheNotFound, key)
	}

	rc, err := b.downloadBlob(ctx, resp.SignedDownloadURL)
	if err != nil {
		return "", nil, err
	}

	matched := resp.MatchedKey
	if matched == "" {
		matched = key
	}
	return matched, rc, nil
}

// List returns cache entries via the REST caches API, filtered by key prefix.
func (b *Backend) List(ctx context.Context, opts cache.ListOptions) ([]cache.Entry, error) {
	defer perf.Track(nil, "githubcache.List")()

	if b.owner == "" || b.repo == "" {
		return nil, fmt.Errorf("%w: owner and repo are required to list caches", errUtils.ErrCacheListFailed)
	}

	var entries []cache.Entry
	page := 1
	for {
		resp, next, err := b.listCaches(ctx, page)
		if err != nil {
			return nil, err
		}
		for _, c := range resp.ActionsCache {
			if opts.KeyPrefix != "" && !strings.HasPrefix(c.Key, opts.KeyPrefix) {
				continue
			}
			entries = append(entries, cache.Entry{
				Key:       c.Key,
				Size:      c.SizeInBytes,
				CreatedAt: c.CreatedAt,
				ID:        strconv.FormatInt(c.ID, 10),
			})
		}
		if next == 0 {
			break
		}
		page = next
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].CreatedAt.After(entries[j].CreatedAt) })
	return entries, nil
}

// Delete removes a cache entry by key via the REST caches API. Missing keys are
// a no-op.
func (b *Backend) Delete(ctx context.Context, key string) error {
	defer perf.Track(nil, "githubcache.Delete")()

	if b.owner == "" || b.repo == "" {
		return fmt.Errorf("%w: owner and repo are required to delete caches", errUtils.ErrCacheDeleteFailed)
	}

	endpoint := fmt.Sprintf("%s/repos/%s/%s/actions/caches?key=%s", b.baseURL, b.owner, b.repo, url.QueryEscape(key))
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return wrapErr(errUtils.ErrCacheDeleteFailed, err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := b.restClient.Do(req) //nolint:gosec // G704: targets the GitHub REST caches API URL, not user input.
	if err != nil {
		return wrapErr(errUtils.ErrCacheDeleteFailed, err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK, http.StatusNoContent, http.StatusNotFound:
		return nil
	default:
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%w: delete returned status %d: %s", errUtils.ErrCacheDeleteFailed, resp.StatusCode, string(body))
	}
}

// listCaches calls GET /repos/{owner}/{repo}/actions/caches with pagination.
func (b *Backend) listCaches(ctx context.Context, page int) (*listCachesResponse, int, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/actions/caches?per_page=%d&page=%d", b.baseURL, b.owner, b.repo, githubPaginationLimit, page)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, wrapErr(errUtils.ErrCacheListFailed, err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := b.restClient.Do(req) //nolint:gosec // G704: targets the GitHub REST caches API URL, not user input.
	if err != nil {
		return nil, 0, wrapErr(errUtils.ErrCacheListFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, 0, fmt.Errorf("%w: list returned status %d: %s", errUtils.ErrCacheListFailed, resp.StatusCode, string(body))
	}

	var result listCachesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, 0, wrapErr(errUtils.ErrCacheListFailed, err)
	}
	return &result, parseNextPage(resp.Header.Get("Link")), nil
}

// uploadBlob PUTs data to an Azure Blob SAS URL as a single block blob.
func (b *Backend) uploadBlob(ctx context.Context, uploadURL string, data io.Reader, size int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, data)
	if err != nil {
		return wrapErr(errUtils.ErrCacheSaveFailed, err)
	}
	req.ContentLength = size
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("x-ms-blob-type", "BlockBlob")

	resp, err := b.blobClient.Do(req) //nolint:gosec // G704: targets a GitHub-provided signed blob URL, not user input.
	if err != nil {
		return wrapErr(errUtils.ErrCacheSaveFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%w: blob upload returned status %d: %s", errUtils.ErrCacheSaveFailed, resp.StatusCode, string(body))
	}
	return nil
}

// downloadBlob GETs a signed blob URL and returns the body reader.
func (b *Backend) downloadBlob(ctx context.Context, downloadURL string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, wrapErr(errUtils.ErrCacheRestoreFailed, err)
	}

	resp, err := b.blobClient.Do(req) //nolint:gosec // G704: targets a GitHub-provided signed blob URL, not user input.
	if err != nil {
		return nil, wrapErr(errUtils.ErrCacheRestoreFailed, err)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, fmt.Errorf("%w: blob download returned status %d: %s", errUtils.ErrCacheRestoreFailed, resp.StatusCode, string(body))
	}
	return resp.Body, nil
}

func init() {
	cache.Register(backendName, NewBackend)
}

// Ensure Backend implements cache.Backend.
var _ cache.Backend = (*Backend)(nil)

// twirpClient implements cacheClient using the GitHub Actions runtime Twirp API.
type twirpClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// newTwirpClient creates a Twirp client for the cache service.
func newTwirpClient(resultsURL, token string) *twirpClient {
	if !strings.HasSuffix(resultsURL, "/") {
		resultsURL += "/"
	}
	return &twirpClient{
		baseURL:    resultsURL,
		token:      token,
		httpClient: &http.Client{Timeout: httpTimeout},
	}
}

func (c *twirpClient) CreateCacheEntry(ctx context.Context, req *createCacheEntryRequest) (*createCacheEntryResponse, error) {
	defer perf.Track(nil, "githubcache.twirp.CreateCacheEntry")()

	var resp createCacheEntryResponse
	if err := c.call(ctx, "CreateCacheEntry", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *twirpClient) FinalizeCacheEntryUpload(ctx context.Context, req *finalizeCacheEntryRequest) (*finalizeCacheEntryResponse, error) {
	defer perf.Track(nil, "githubcache.twirp.FinalizeCacheEntryUpload")()

	var resp finalizeCacheEntryResponse
	if err := c.call(ctx, "FinalizeCacheEntryUpload", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *twirpClient) GetCacheEntryDownloadURL(ctx context.Context, req *getCacheEntryDownloadURLRequest) (*getCacheEntryDownloadURLResponse, error) {
	defer perf.Track(nil, "githubcache.twirp.GetCacheEntryDownloadURL")()

	var resp getCacheEntryDownloadURLResponse
	if err := c.call(ctx, "GetCacheEntryDownloadURL", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// call performs a single Twirp JSON POST.
func (c *twirpClient) call(ctx context.Context, method string, reqBody, out any) error {
	endpoint := c.baseURL + cacheServicePath + "/" + method

	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal %s request: %w", method, err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create %s request: %w", method, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(httpReq) //nolint:gosec // G704: targets the GitHub Actions results service URL, not user input.
	if err != nil {
		return fmt.Errorf("failed to call %s: %w", method, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%w: %s returned status %d: %s", errUtils.ErrCacheBackendRequest, method, resp.StatusCode, string(respBody))
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("failed to decode %s response: %w", method, err)
	}
	return nil
}

// newRESTClient builds an HTTP client that injects the GitHub token (if any).
func newRESTClient(token string) *http.Client {
	if token == "" {
		return &http.Client{Timeout: httpTimeout}
	}
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	client := oauth2.NewClient(context.Background(), ts)
	client.Timeout = httpTimeout
	return client
}

// resolveOwnerRepo resolves owner/repo from options/GITHUB_REPOSITORY, falling
// back to the local git remote (a github.com repository) when neither is set.
// The git fallback is what lets `atmos ci cache list`/`delete` administer the
// cache locally, outside a runner where GITHUB_REPOSITORY is unset.
func resolveOwnerRepo(options map[string]any) (string, string) {
	defer perf.Track(nil, "githubcache.resolveOwnerRepo")()

	owner, repo := repoFromEnv(options)
	if owner != "" && repo != "" {
		return owner, repo
	}
	return ownerRepoFromLocalGit()
}

// ownerRepoFromLocalGit resolves owner/repo from the local git remote when the
// remote is hosted on github.com. It returns empty strings on any failure
// (not a git repo, no remote, non-github host); callers surface a friendly
// error when owner/repo end up empty.
func ownerRepoFromLocalGit() (string, string) {
	repo, err := git.GetLocalRepo()
	if err != nil {
		log.Debug("CI cache: could not open local git repo for owner/repo resolution", "error", err)
		return "", ""
	}
	info, err := git.GetRepoInfo(repo)
	if err != nil {
		log.Debug("CI cache: could not read local git remote for owner/repo resolution", "error", err)
		return "", ""
	}
	if info.RepoHost != "github.com" {
		log.Debug("CI cache: local git remote is not hosted on github.com", "host", info.RepoHost)
		return "", ""
	}
	return info.RepoOwner, info.RepoName
}

// repoFromEnv resolves owner/repo from options or GITHUB_REPOSITORY.
func repoFromEnv(options map[string]any) (string, string) {
	owner, _ := options["owner"].(string)
	repo, _ := options["repo"].(string)
	if owner != "" && repo != "" {
		return owner, repo
	}
	envOwner, envRepo := splitGitHubRepository(os.Getenv("GITHUB_REPOSITORY"))
	if owner == "" {
		owner = envOwner
	}
	if repo == "" {
		repo = envRepo
	}
	return owner, repo
}

// splitGitHubRepository splits "owner/repo" into its parts (empty on mismatch).
func splitGitHubRepository(ghRepo string) (string, string) {
	parts := strings.SplitN(ghRepo, "/", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}

// parseNextPage extracts the next page number from a GitHub Link header.
func parseNextPage(linkHeader string) int {
	if linkHeader == "" {
		return 0
	}
	for _, part := range strings.Split(linkHeader, ",") {
		if p := nextPageFromLinkPart(strings.TrimSpace(part)); p > 0 {
			return p
		}
	}
	return 0
}

// nextPageFromLinkPart returns the page number from a single rel="next" Link part.
func nextPageFromLinkPart(part string) int {
	if !strings.Contains(part, `rel="next"`) {
		return 0
	}
	start := strings.Index(part, "<")
	end := strings.Index(part, ">")
	if start < 0 || end < 0 || end <= start {
		return 0
	}
	urlStr := part[start+1 : end]
	if idx := strings.Index(urlStr, "?"); idx >= 0 {
		urlStr = urlStr[idx+1:]
	}
	return pageParam(urlStr)
}

// pageParam extracts the integer "page=" parameter from a URL query string.
func pageParam(query string) int {
	for _, param := range strings.Split(query, "&") {
		if strings.HasPrefix(param, "page=") {
			if p, err := strconv.Atoi(strings.TrimPrefix(param, "page=")); err == nil {
				return p
			}
		}
	}
	return 0
}
