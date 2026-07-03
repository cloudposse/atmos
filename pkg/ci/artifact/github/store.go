package github

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/artifact"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	storeName = "github/artifacts"

	// Metadata is stored as a JSON sidecar within the artifact zip.
	metadataFilename = "metadata.json"

	// archiveFilename is the name of the tar archive within the zip.
	archiveFilename = "archive.tar"

	// Default retention for artifacts.
	defaultRetentionDays = 7

	// GithubPaginationLimit is the max number of items per page for GitHub API.
	githubPaginationLimit = 100

	// HTTPTimeout is the timeout for HTTP requests.
	httpTimeout = 30 * time.Second

	// ListArtifactsMaxAttempts bounds retries for transient GitHub API failures.
	listArtifactsMaxAttempts = 5

	// artifactServicePath is the Twirp service path for artifact operations.
	artifactServicePath = "twirp/github.actions.results.api.v1.ArtifactService"

	// artifactVersion is the artifact API version.
	artifactVersion = 4

	// HTTP header names and values used by the runtime Twirp client.
	headerContentType   = "Content-Type"
	headerAuthorization = "Authorization"
	contentTypeJSON     = "application/json"
)

var listArtifactsRetryBaseDelay = time.Second

// artifactUploader handles the GitHub Actions runtime API calls for artifact upload.
// This is extracted as an interface for testability.
type artifactUploader interface {
	// CreateArtifact initiates artifact creation and returns a signed upload URL.
	CreateArtifact(ctx context.Context, req *createArtifactRequest) (*createArtifactResponse, error)

	// UploadBlob uploads data to the signed blob URL.
	UploadBlob(ctx context.Context, uploadURL string, data []byte) error

	// FinalizeArtifact finalizes the artifact after upload.
	FinalizeArtifact(ctx context.Context, req *finalizeArtifactRequest) (*finalizeArtifactResponse, error)
}

// artifactDownloader fetches artifacts via the GitHub Actions runtime API.
// Unlike the REST API (which only serves an artifact after its producing run
// has completed), the runtime API can read artifacts from the in-progress run,
// enabling same-run plan-then-apply handoff. It is scoped to the current run
// (across all of its jobs), so it cannot see other runs' artifacts — callers
// fall back to the REST API for those.
type artifactDownloader interface {
	// ListArtifacts lists the artifacts in the current run (across jobs). Each
	// entry carries the backend IDs of the job that uploaded it.
	ListArtifacts(ctx context.Context, req *runtimeListArtifactsRequest) (*runtimeListArtifactsResponse, error)

	// GetSignedArtifactURL returns a signed blob URL for an artifact, addressed
	// by the backend IDs of the job that uploaded it.
	GetSignedArtifactURL(ctx context.Context, req *getSignedArtifactURLRequest) (*getSignedArtifactURLResponse, error)
}

// runtimeListArtifactsRequest is the request body for the runtime ListArtifacts API.
// The backend IDs are the current job's; the service returns all artifacts in the
// run (the run backend ID is shared across the run's jobs).
type runtimeListArtifactsRequest struct {
	WorkflowRunBackendID    string `json:"workflow_run_backend_id"`
	WorkflowJobRunBackendID string `json:"workflow_job_run_backend_id"`
}

// runtimeArtifact is a single artifact entry from the runtime ListArtifacts API.
// The backend IDs identify the job that uploaded the artifact and are required to
// request its signed download URL.
type runtimeArtifact struct {
	WorkflowRunBackendID    string `json:"workflow_run_backend_id"`
	WorkflowJobRunBackendID string `json:"workflow_job_run_backend_id"`
	Name                    string `json:"name"`
}

// runtimeListArtifactsResponse is the response from the runtime ListArtifacts API.
type runtimeListArtifactsResponse struct {
	Artifacts []runtimeArtifact `json:"artifacts"`
}

// getSignedArtifactURLRequest is the request body for the GetSignedArtifactURL API.
type getSignedArtifactURLRequest struct {
	WorkflowRunBackendID    string `json:"workflow_run_backend_id"`
	WorkflowJobRunBackendID string `json:"workflow_job_run_backend_id"`
	Name                    string `json:"name"`
}

// getSignedArtifactURLResponse is the response from the GetSignedArtifactURL API.
type getSignedArtifactURLResponse struct {
	SignedURL string `json:"signed_url"`
}

// createArtifactRequest is the request body for the CreateArtifact API.
type createArtifactRequest struct {
	Version                 int    `json:"version"`
	Name                    string `json:"name"`
	WorkflowRunBackendID    string `json:"workflow_run_backend_id"`
	WorkflowJobRunBackendID string `json:"workflow_job_run_backend_id"`
	ExpiresAfter            string `json:"expires_after,omitempty"`
}

// createArtifactResponse is the response from the CreateArtifact API.
type createArtifactResponse struct {
	OK              bool   `json:"ok"`
	SignedUploadURL string `json:"signed_upload_url"`
}

// finalizeArtifactRequest is the request body for the FinalizeArtifact API.
type finalizeArtifactRequest struct {
	Name                    string `json:"name"`
	Size                    int64  `json:"size"`
	Hash                    string `json:"hash,omitempty"`
	WorkflowRunBackendID    string `json:"workflow_run_backend_id"`
	WorkflowJobRunBackendID string `json:"workflow_job_run_backend_id"`
}

// finalizeArtifactResponse is the response from the FinalizeArtifact API.
type finalizeArtifactResponse struct {
	OK         bool  `json:"ok"`
	ArtifactID int64 `json:"artifact_id,string"`
}

// backendIDs holds the workflow backend IDs extracted from the runtime token.
type backendIDs struct {
	WorkflowRunBackendID    string
	WorkflowJobRunBackendID string
}

// githubArtifact represents a GitHub Actions artifact from the REST API.
type githubArtifact struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	SizeInBytes int64     `json:"size_in_bytes"`
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// listArtifactsResponse is the response from the list artifacts REST API.
type listArtifactsResponse struct {
	TotalCount int              `json:"total_count"`
	Artifacts  []githubArtifact `json:"artifacts"`
}

type listArtifactsAttemptResult struct {
	response  *listArtifactsResponse
	nextPage  int
	retryable bool
}

// Store implements the artifact.Backend interface using GitHub Actions Artifacts.
type Store struct {
	httpClient    *http.Client
	baseURL       string
	uploader      artifactUploader
	downloader    artifactDownloader
	owner         string
	repo          string
	prefix        string
	retentionDays int
}

// NewStore creates a new GitHub Artifacts backend.
func NewStore(opts artifact.StoreOptions) (artifact.Backend, error) {
	defer perf.Track(opts.AtmosConfig, "github.NewStore")()

	token := getGitHubToken()
	if token == "" {
		return nil, errUtils.ErrGitHubTokenNotFound
	}

	owner, repo := getRepoInfo(opts.Options)
	if owner == "" || repo == "" {
		return nil, fmt.Errorf("%w: owner and repo are required for GitHub Artifacts store", errUtils.ErrArtifactStoreNotFound)
	}

	retentionDays := getRetentionDays(opts.Options)
	prefix, _ := opts.Options["prefix"].(string)

	// Create the runtime client if running inside GitHub Actions. It backs both
	// upload and the same-run download path (the REST API can't read an artifact
	// from the still-running producing run).
	var uploader artifactUploader
	var downloader artifactDownloader
	runtimeToken := os.Getenv("ACTIONS_RUNTIME_TOKEN")
	resultsURL := os.Getenv("ACTIONS_RESULTS_URL")
	if runtimeToken != "" && resultsURL != "" {
		runtime := newRuntimeUploader(resultsURL, runtimeToken)
		uploader = runtime
		downloader = runtime
	}

	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	httpClient := oauth2.NewClient(context.Background(), ts)
	httpClient.Timeout = httpTimeout

	return &Store{
		httpClient:    httpClient,
		baseURL:       "https://api.github.com",
		uploader:      uploader,
		downloader:    downloader,
		owner:         owner,
		repo:          repo,
		prefix:        prefix,
		retentionDays: retentionDays,
	}, nil
}

// getGitHubToken returns the GitHub token from environment variables.
// Token precedence: ATMOS_CI_GITHUB_TOKEN > GITHUB_TOKEN > GH_TOKEN.
func getGitHubToken() string {
	token := os.Getenv("ATMOS_CI_GITHUB_TOKEN")
	if token == "" {
		token = os.Getenv("GITHUB_TOKEN")
	}
	if token == "" {
		token = os.Getenv("GH_TOKEN")
	}
	return token
}

// getRepoInfo extracts owner and repo from options or environment.
func getRepoInfo(options map[string]any) (string, string) {
	owner, _ := options["owner"].(string)
	repo, _ := options["repo"].(string)

	if owner != "" && repo != "" {
		return owner, repo
	}

	return fillRepoFromEnv(owner, repo)
}

// fillRepoFromEnv fills missing owner/repo from GITHUB_REPOSITORY env var.
func fillRepoFromEnv(owner, repo string) (string, string) {
	ghRepo := os.Getenv("GITHUB_REPOSITORY")
	if ghRepo == "" {
		return owner, repo
	}

	parts := splitRepoString(ghRepo)
	if len(parts) != 2 {
		return owner, repo
	}

	if owner == "" {
		owner = parts[0]
	}
	if repo == "" {
		repo = parts[1]
	}
	return owner, repo
}

// getRetentionDays returns retention days from options or default.
func getRetentionDays(options map[string]any) int {
	if days, ok := options["retention_days"].(int); ok && days > 0 {
		return days
	}
	return defaultRetentionDays
}

// Name returns the store type name.
func (s *Store) Name() string {
	defer perf.Track(nil, "github.Store.Name")()

	return storeName
}

// artifactName returns the GitHub Actions artifact name for a given key.
// If a prefix is configured, it is prepended with a dash separator.
func (s *Store) artifactName(key string) string {
	sanitized := sanitizeKey(key)
	if s.prefix != "" {
		return s.prefix + "-" + sanitized
	}
	return sanitized
}

// Upload uploads a single data stream as a GitHub artifact.
// Creates a zip containing archive.tar (the data stream) + metadata.json.
// This requires running within GitHub Actions with ACTIONS_RUNTIME_TOKEN and
// ACTIONS_RESULTS_URL environment variables set. GitHub withholds those from
// `run:` steps, so surface them with the actions/github-runtime helper action
// when invoking Atmos from a shell step.
func (s *Store) Upload(ctx context.Context, key string, data io.Reader, size int64, metadata *artifact.Metadata) error {
	defer perf.Track(nil, "github.Upload")()

	if s.uploader == nil {
		return fmt.Errorf("%w: GitHub Artifacts upload requires running within GitHub Actions (ACTIONS_RUNTIME_TOKEN and ACTIONS_RESULTS_URL must be set)", errUtils.ErrNotImplemented)
	}

	// Parse backend IDs from runtime token.
	runtimeToken := os.Getenv("ACTIONS_RUNTIME_TOKEN")
	ids, err := getBackendIDsFromToken(runtimeToken)
	if err != nil {
		return fmt.Errorf("%w: failed to parse runtime token: %w", errUtils.ErrArtifactUploadFailed, err)
	}

	// Create a zip archive with the tar stream + metadata.
	zipData, err := createArtifactZip(data, metadata)
	if err != nil {
		return err
	}

	artifactName := s.artifactName(key)

	// Step 1: Create the artifact to get a signed upload URL.
	createReq := &createArtifactRequest{
		Version:                 artifactVersion,
		Name:                    artifactName,
		WorkflowRunBackendID:    ids.WorkflowRunBackendID,
		WorkflowJobRunBackendID: ids.WorkflowJobRunBackendID,
	}
	if s.retentionDays > 0 {
		createReq.ExpiresAfter = fmt.Sprintf("%dd", s.retentionDays)
	}

	createResp, err := s.uploader.CreateArtifact(ctx, createReq)
	if err != nil {
		return fmt.Errorf("%w: failed to create artifact: %w", errUtils.ErrArtifactUploadFailed, err)
	}

	if createResp.SignedUploadURL == "" {
		return fmt.Errorf("%w: artifact service returned empty upload URL", errUtils.ErrArtifactUploadFailed)
	}

	// Step 2: Upload the zip data to the signed URL.
	if err := s.uploader.UploadBlob(ctx, createResp.SignedUploadURL, zipData); err != nil {
		return fmt.Errorf("%w: failed to upload artifact data: %w", errUtils.ErrArtifactUploadFailed, err)
	}

	// Step 3: Finalize the artifact.
	finalizeReq := &finalizeArtifactRequest{
		Name:                    artifactName,
		Size:                    int64(len(zipData)),
		WorkflowRunBackendID:    ids.WorkflowRunBackendID,
		WorkflowJobRunBackendID: ids.WorkflowJobRunBackendID,
	}

	if _, err := s.uploader.FinalizeArtifact(ctx, finalizeReq); err != nil {
		return fmt.Errorf("%w: failed to finalize artifact: %w", errUtils.ErrArtifactUploadFailed, err)
	}

	return nil
}

// createArtifactZip creates a zip archive containing archive.tar (data stream) and metadata.json.
// The GitHub Actions runtime API requires zip format.
func createArtifactZip(data io.Reader, metadata *artifact.Metadata) ([]byte, error) {
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)

	// Add the tar stream as archive.tar.
	archiveWriter, err := zipWriter.Create(archiveFilename)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create zip entry for %s: %w", errUtils.ErrArtifactUploadFailed, archiveFilename, err)
	}

	dataBytes, err := io.ReadAll(data)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to read data: %w", errUtils.ErrArtifactUploadFailed, err)
	}

	if _, err := archiveWriter.Write(dataBytes); err != nil {
		return nil, fmt.Errorf("%w: failed to write %s to zip: %w", errUtils.ErrArtifactUploadFailed, archiveFilename, err)
	}

	// Add metadata as a sidecar entry.
	if metadata != nil {
		metadataWriter, err := zipWriter.Create(metadataFilename)
		if err != nil {
			return nil, fmt.Errorf("%w: failed to create zip entry for metadata: %w", errUtils.ErrArtifactUploadFailed, err)
		}
		metadataJSON, err := json.MarshalIndent(metadata, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("%w: failed to marshal metadata: %w", errUtils.ErrArtifactUploadFailed, err)
		}
		if _, err := metadataWriter.Write(metadataJSON); err != nil {
			return nil, fmt.Errorf("%w: failed to write metadata to zip: %w", errUtils.ErrArtifactUploadFailed, err)
		}
	}

	if err := zipWriter.Close(); err != nil {
		return nil, fmt.Errorf("%w: failed to close zip archive: %w", errUtils.ErrArtifactUploadFailed, err)
	}

	return buf.Bytes(), nil
}

// listArtifacts calls GET /repos/{owner}/{repo}/actions/artifacts with pagination params.
func (s *Store) listArtifacts(ctx context.Context, perPage, page int) (*listArtifactsResponse, int, error) {
	var lastErr error
	for attempt := 1; attempt <= listArtifactsMaxAttempts; attempt++ {
		result, err := s.listArtifactsOnce(ctx, perPage, page)
		if err == nil {
			return result.response, result.nextPage, nil
		}
		lastErr = err
		if result == nil || !result.retryable || attempt == listArtifactsMaxAttempts {
			break
		}
		if err := sleepBeforeListRetry(ctx, attempt); err != nil {
			return nil, 0, err
		}
	}

	return nil, 0, lastErr
}

func sleepBeforeListRetry(ctx context.Context, attempt int) error {
	delay := listArtifactsRetryBaseDelay * time.Duration(1<<(attempt-1))
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func isRetryableListArtifactsStatus(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests || statusCode >= http.StatusInternalServerError
}

func (s *Store) listArtifactsOnce(ctx context.Context, perPage, page int) (*listArtifactsAttemptResult, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/actions/artifacts?per_page=%d&page=%d", s.baseURL, s.owner, s.repo, perPage, page)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return &listArtifactsAttemptResult{retryable: false}, fmt.Errorf("failed to create list artifacts request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return &listArtifactsAttemptResult{retryable: false}, fmt.Errorf("failed to list artifacts: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return &listArtifactsAttemptResult{
			retryable: isRetryableListArtifactsStatus(resp.StatusCode),
		}, fmt.Errorf("%w: status %d: %s", errUtils.ErrArtifactListFailed, resp.StatusCode, string(body))
	}

	var result listArtifactsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return &listArtifactsAttemptResult{retryable: false}, fmt.Errorf("failed to decode list artifacts response: %w", err)
	}

	nextPage := parseNextPage(resp.Header.Get("Link"))

	return &listArtifactsAttemptResult{
		response: &result,
		nextPage: nextPage,
	}, nil
}

// downloadArtifactURL calls GET /repos/{owner}/{repo}/actions/artifacts/{id}/zip
// with redirect-following disabled and returns the Location header URL.
func (s *Store) downloadArtifactURL(ctx context.Context, artifactID int64) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/actions/artifacts/%d/zip", s.baseURL, s.owner, s.repo, artifactID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create download artifact request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	// Use a client that does not follow redirects to capture the Location header.
	// Reuse the oauth2 transport so the token is still injected automatically.
	noRedirectClient := &http.Client{
		Transport: s.httpClient.Transport,
		Timeout:   httpTimeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := noRedirectClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to get artifact download URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusFound {
		location := resp.Header.Get("Location")
		if location == "" {
			return "", fmt.Errorf("artifact download returned redirect without Location header")
		}
		return location, nil
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("artifact download returned status %d: %s", resp.StatusCode, string(body))
	}

	// Some test servers may return 200 directly; in that case there's no redirect URL.
	return "", fmt.Errorf("artifact download returned 200 instead of redirect")
}

// deleteArtifact calls DELETE /repos/{owner}/{repo}/actions/artifacts/{id}.
func (s *Store) deleteArtifact(ctx context.Context, artifactID int64) error {
	url := fmt.Sprintf("%s/repos/%s/%s/actions/artifacts/%d", s.baseURL, s.owner, s.repo, artifactID)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create delete artifact request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete artifact: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete artifact returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// parseNextPage extracts the next page number from a GitHub Link header.
// Returns 0 if there is no next page.
func parseNextPage(linkHeader string) int {
	if linkHeader == "" {
		return 0
	}

	// Link header format: <url>; rel="next", <url>; rel="last"
	for _, part := range strings.Split(linkHeader, ",") {
		part = strings.TrimSpace(part)
		if !strings.Contains(part, `rel="next"`) {
			continue
		}

		// Extract URL from angle brackets.
		start := strings.Index(part, "<")
		end := strings.Index(part, ">")
		if start < 0 || end < 0 || end <= start {
			continue
		}

		urlStr := part[start+1 : end]
		// Strip everything before and including '?' to get query string.
		if idx := strings.Index(urlStr, "?"); idx >= 0 {
			urlStr = urlStr[idx+1:]
		}
		// Extract page parameter from query string.
		for _, param := range strings.Split(urlStr, "&") {
			if strings.HasPrefix(param, "page=") {
				val := strings.TrimPrefix(param, "page=")
				if p, err := strconv.Atoi(val); err == nil {
					return p
				}
			}
		}
	}

	return 0
}

// Download downloads an artifact and extracts the tar stream from the zip.
func (s *Store) Download(ctx context.Context, key string) (io.ReadCloser, *artifact.Metadata, error) {
	defer perf.Track(nil, "github.Download")()

	// Prefer the runtime API when available: it can read an artifact from the
	// in-progress run (e.g. a planfile uploaded by an earlier job in the same
	// run), which the REST API cannot. The runtime API is scoped to the current
	// run, so fall back to REST for artifacts from other (completed) runs.
	if s.downloader != nil {
		rc, meta, err := s.downloadViaRuntime(ctx, key)
		if err == nil {
			return rc, meta, nil
		}
		log.Debug("Runtime artifact download failed; falling back to REST API",
			"key", key, "error", err)
	}

	a, err := s.findArtifact(ctx, key)
	if err != nil {
		return nil, nil, err
	}

	zipData, err := s.downloadArtifactContent(ctx, a.ID)
	if err != nil {
		return nil, nil, err
	}

	return extractFromZip(zipData)
}

// downloadViaRuntime fetches an artifact from the current run via the GitHub
// Actions runtime API and extracts the tar stream + metadata from its zip.
func (s *Store) downloadViaRuntime(ctx context.Context, key string) (io.ReadCloser, *artifact.Metadata, error) {
	defer perf.Track(nil, "github.downloadViaRuntime")()

	ids, err := getBackendIDsFromToken(os.Getenv("ACTIONS_RUNTIME_TOKEN"))
	if err != nil {
		return nil, nil, fmt.Errorf("%w: failed to parse runtime token: %w", errUtils.ErrArtifactDownloadFailed, err)
	}

	// List the run's artifacts (across jobs) and find ours by name. A signed URL
	// must be requested with the backend IDs of the job that uploaded the
	// artifact, which an earlier job in the run set — not the current job's.
	listResp, err := s.downloader.ListArtifacts(ctx, &runtimeListArtifactsRequest{
		WorkflowRunBackendID:    ids.WorkflowRunBackendID,
		WorkflowJobRunBackendID: ids.WorkflowJobRunBackendID,
	})
	if err != nil {
		return nil, nil, err
	}

	name := s.artifactName(key)
	var found *runtimeArtifact
	for i := range listResp.Artifacts {
		if listResp.Artifacts[i].Name == name {
			found = &listResp.Artifacts[i]
			break
		}
	}
	if found == nil {
		return nil, nil, fmt.Errorf("%w: %s", errUtils.ErrArtifactNotFound, key)
	}

	resp, err := s.downloader.GetSignedArtifactURL(ctx, &getSignedArtifactURLRequest{
		WorkflowRunBackendID:    found.WorkflowRunBackendID,
		WorkflowJobRunBackendID: found.WorkflowJobRunBackendID,
		Name:                    name,
	})
	if err != nil {
		return nil, nil, err
	}
	if resp.SignedURL == "" {
		return nil, nil, fmt.Errorf("%w: runtime API returned an empty signed URL", errUtils.ErrArtifactDownloadFailed)
	}

	zipData, err := s.fetchBlob(ctx, resp.SignedURL)
	if err != nil {
		return nil, nil, err
	}

	return extractFromZip(zipData)
}

// fetchBlob downloads the bytes at a pre-signed blob URL. The URL carries its
// own authentication, so it is fetched with a plain client (no GitHub token).
func (s *Store) fetchBlob(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create blob request: %w", errUtils.ErrArtifactDownloadFailed, err)
	}

	client := &http.Client{Timeout: httpTimeout}
	resp, err := client.Do(req) //nolint:gosec // G704: url is a GitHub-issued, pre-signed blob URL from the runtime API.
	if err != nil {
		return nil, fmt.Errorf("%w: failed to download artifact blob: %w", errUtils.ErrArtifactDownloadFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%w: blob download returned status %d: %s", errUtils.ErrArtifactDownloadFailed, resp.StatusCode, string(body))
	}

	return io.ReadAll(resp.Body)
}

// findArtifact finds an artifact by key.
func (s *Store) findArtifact(ctx context.Context, key string) (*githubArtifact, error) {
	artifactName := s.artifactName(key)

	resp, _, err := s.listArtifacts(ctx, githubPaginationLimit, 1)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to list artifacts for download: %w", errUtils.ErrArtifactDownloadFailed, err)
	}

	for i := range resp.Artifacts {
		if resp.Artifacts[i].Name == artifactName {
			return &resp.Artifacts[i], nil
		}
	}

	return nil, fmt.Errorf("%w: %s", errUtils.ErrArtifactNotFound, key)
}

// downloadArtifactContent downloads artifact content as zip data.
func (s *Store) downloadArtifactContent(ctx context.Context, artifactID int64) ([]byte, error) {
	downloadURL, err := s.downloadArtifactURL(ctx, artifactID)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to get artifact download URL: %w", errUtils.ErrArtifactDownloadFailed, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create download request: %w", errUtils.ErrArtifactDownloadFailed, err)
	}

	resp, err := s.httpClient.Do(req) //nolint:gosec // G704: downloadURL is the redirect Location issued by the GitHub REST API, not user input.
	if err != nil {
		return nil, fmt.Errorf("%w: failed to download artifact: %w", errUtils.ErrArtifactDownloadFailed, err)
	}
	defer resp.Body.Close()

	zipData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to read artifact content: %w", errUtils.ErrArtifactDownloadFailed, err)
	}

	return zipData, nil
}

// extractFromZip extracts the archive.tar stream and metadata from the zip archive.
// Returns an io.ReadCloser for the tar data and the parsed metadata.
func extractFromZip(zipData []byte) (io.ReadCloser, *artifact.Metadata, error) {
	zipReader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return nil, nil, fmt.Errorf("%w: failed to open artifact zip: %w", errUtils.ErrArtifactDownloadFailed, err)
	}

	var tarData []byte
	var metadata *artifact.Metadata

	for _, file := range zipReader.File {
		switch file.Name {
		case metadataFilename:
			metadata = readMetadataFile(file)
		case archiveFilename:
			data, err := readZipFile(file)
			if err != nil {
				return nil, nil, err
			}
			tarData = data
		}
	}

	if tarData == nil {
		return nil, nil, fmt.Errorf("%w: no %s found in artifact zip", errUtils.ErrArtifactDownloadFailed, archiveFilename)
	}

	return io.NopCloser(bytes.NewReader(tarData)), metadata, nil
}

// readZipFile reads a file from the zip archive.
func readZipFile(file *zip.File) ([]byte, error) {
	rc, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("%w: failed to open file in zip: %w", errUtils.ErrArtifactDownloadFailed, err)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to read file: %w", errUtils.ErrArtifactDownloadFailed, err)
	}
	return data, nil
}

// readMetadataFile reads metadata from a zip file (returns nil on error).
func readMetadataFile(file *zip.File) *artifact.Metadata {
	rc, err := file.Open()
	if err != nil {
		return nil
	}
	defer rc.Close()

	var m artifact.Metadata
	if err := json.NewDecoder(rc).Decode(&m); err != nil {
		return nil
	}
	return &m
}

// Delete deletes an artifact.
func (s *Store) Delete(ctx context.Context, key string) error {
	defer perf.Track(nil, "github.Delete")()

	artifactName := s.artifactName(key)

	// List artifacts to find the one matching our key.
	resp, _, err := s.listArtifacts(ctx, githubPaginationLimit, 1)
	if err != nil {
		return fmt.Errorf("%w: failed to list artifacts for deletion: %w", errUtils.ErrArtifactDeleteFailed, err)
	}

	for _, a := range resp.Artifacts {
		if a.Name == artifactName {
			if err := s.deleteArtifact(ctx, a.ID); err != nil {
				return fmt.Errorf("%w: failed to delete artifact: %w", errUtils.ErrArtifactDeleteFailed, err)
			}
			return nil
		}
	}

	return nil // Already deleted or never existed.
}

// List lists artifacts matching the given query.
func (s *Store) List(ctx context.Context, query artifact.Query) ([]artifact.ArtifactInfo, error) {
	defer perf.Track(nil, "github.List")()

	// Convert query to a prefix for filtering.
	prefix := s.queryToPrefix(query)

	var files []artifact.ArtifactInfo
	page := 1

	for {
		resp, nextPage, err := s.listArtifacts(ctx, githubPaginationLimit, page)
		if err != nil {
			return nil, fmt.Errorf("%w: failed to list artifacts: %w", errUtils.ErrArtifactListFailed, err)
		}

		for _, a := range resp.Artifacts {
			name := a.Name

			// If a prefix is configured, only include matching artifacts and strip it.
			key := s.stripArtifactPrefix(name)
			if key == "" {
				continue
			}
			key = desanitizeKey(key)

			// Check prefix match.
			if prefix != "" && !hasPrefix(key, prefix) {
				continue
			}

			files = append(files, artifact.ArtifactInfo{
				Name:         key,
				Size:         a.SizeInBytes,
				LastModified: a.CreatedAt,
			})
		}

		if nextPage == 0 {
			break
		}
		page = nextPage
	}

	// Sort by last modified (newest first).
	sort.Slice(files, func(i, j int) bool {
		return files[i].LastModified.After(files[j].LastModified)
	})

	return files, nil
}

// queryToPrefix converts an artifact.Query to a prefix string for filtering.
func (s *Store) queryToPrefix(query artifact.Query) string {
	if query.All {
		return ""
	}

	var prefix string
	if len(query.Stacks) > 0 {
		prefix = query.Stacks[0]
	}
	if len(query.Components) > 0 && prefix != "" {
		prefix += "/" + query.Components[0]
	}

	return prefix
}

// Exists checks if an artifact exists.
func (s *Store) Exists(ctx context.Context, key string) (bool, error) {
	defer perf.Track(nil, "github.Exists")()

	artifactName := s.artifactName(key)

	resp, _, err := s.listArtifacts(ctx, githubPaginationLimit, 1)
	if err != nil {
		return false, fmt.Errorf("%w: failed to check artifact existence: %w", errUtils.ErrArtifactListFailed, err)
	}

	for _, a := range resp.Artifacts {
		if a.Name == artifactName {
			return true, nil
		}
	}

	return false, nil
}

// GetMetadata retrieves metadata for an artifact.
func (s *Store) GetMetadata(ctx context.Context, key string) (*artifact.Metadata, error) {
	defer perf.Track(nil, "github.GetMetadata")()

	artifactName := s.artifactName(key)

	resp, _, err := s.listArtifacts(ctx, githubPaginationLimit, 1)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to get artifact metadata: %w", errUtils.ErrArtifactListFailed, err)
	}

	for _, a := range resp.Artifacts {
		if a.Name == artifactName {
			// Return basic metadata from artifact info.
			// Full metadata would require downloading the artifact.
			meta := &artifact.Metadata{}
			meta.CreatedAt = a.CreatedAt
			meta.ExpiresAt = func() *time.Time {
				t := a.ExpiresAt
				return &t
			}()
			return meta, nil
		}
	}

	return nil, fmt.Errorf("%w: %s", errUtils.ErrArtifactNotFound, key)
}

// stripArtifactPrefix strips the configured prefix from an artifact name and returns the remainder.
// If a prefix is configured but the name doesn't match, returns empty string.
// If no prefix is configured, returns the full name (all artifacts match).
func (s *Store) stripArtifactPrefix(name string) string {
	if s.prefix == "" {
		return name
	}
	fullPrefix := s.prefix + "-"
	if len(name) > len(fullPrefix) && name[:len(fullPrefix)] == fullPrefix {
		return name[len(fullPrefix):]
	}
	return ""
}

// sanitizeKey converts a storage key to a valid artifact name.
func sanitizeKey(key string) string {
	result := make([]byte, 0, len(key))
	for i := 0; i < len(key); i++ {
		c := key[i]
		if c == '/' || c == '\\' {
			result = append(result, '-', '-')
		} else {
			result = append(result, c)
		}
	}
	return string(result)
}

// desanitizeKey converts an artifact name back to a storage key.
func desanitizeKey(name string) string {
	result := make([]byte, 0, len(name))
	i := 0
	for i < len(name) {
		if i+1 < len(name) && name[i] == '-' && name[i+1] == '-' {
			result = append(result, '/')
			i += 2
		} else {
			result = append(result, name[i])
			i++
		}
	}
	return string(result)
}

// splitRepoString splits "owner/repo" into ["owner", "repo"].
func splitRepoString(repo string) []string {
	for i := 0; i < len(repo); i++ {
		if repo[i] == '/' {
			return []string{repo[:i], repo[i+1:]}
		}
	}
	return []string{repo}
}

// hasPrefix checks if s has the given prefix.
func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

// runtimeUploader implements artifactUploader using the GitHub Actions runtime API.
type runtimeUploader struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// newRuntimeUploader creates a new runtime uploader for the GitHub Actions artifact API.
func newRuntimeUploader(resultsURL, token string) *runtimeUploader {
	// Ensure base URL has trailing slash.
	if !strings.HasSuffix(resultsURL, "/") {
		resultsURL += "/"
	}

	return &runtimeUploader{
		baseURL:    resultsURL,
		token:      token,
		httpClient: &http.Client{Timeout: httpTimeout},
	}
}

// CreateArtifact calls the CreateArtifact Twirp endpoint.
func (u *runtimeUploader) CreateArtifact(ctx context.Context, req *createArtifactRequest) (*createArtifactResponse, error) {
	defer perf.Track(nil, "github.runtimeUploader.CreateArtifact")()

	endpoint := u.baseURL + artifactServicePath + "/CreateArtifact"

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal create request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+u.token)

	resp, err := u.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to call CreateArtifact: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("CreateArtifact returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result createArtifactResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode CreateArtifact response: %w", err)
	}

	return &result, nil
}

// UploadBlob uploads data to the signed blob URL using a PUT request.
func (u *runtimeUploader) UploadBlob(ctx context.Context, uploadURL string, data []byte) error {
	defer perf.Track(nil, "github.runtimeUploader.UploadBlob")()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create upload request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/octet-stream")
	httpReq.Header.Set("x-ms-blob-type", "BlockBlob")

	resp, err := u.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to upload blob: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("blob upload returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// FinalizeArtifact calls the FinalizeArtifact Twirp endpoint.
func (u *runtimeUploader) FinalizeArtifact(ctx context.Context, req *finalizeArtifactRequest) (*finalizeArtifactResponse, error) {
	defer perf.Track(nil, "github.runtimeUploader.FinalizeArtifact")()

	endpoint := u.baseURL + artifactServicePath + "/FinalizeArtifact"

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal finalize request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+u.token)

	resp, err := u.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to call FinalizeArtifact: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("FinalizeArtifact returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result finalizeArtifactResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode FinalizeArtifact response: %w", err)
	}

	return &result, nil
}

// postRuntimeJSON posts a JSON request to a Twirp method on the runtime artifact
// service and decodes the JSON response into out. It backs the download-side
// runtime calls (ListArtifacts, GetSignedArtifactURL).
func (u *runtimeUploader) postRuntimeJSON(ctx context.Context, method string, in, out any) error {
	body, err := json.Marshal(in)
	if err != nil {
		return fmt.Errorf("failed to marshal %s request: %w", method, err)
	}

	endpoint := u.baseURL + artifactServicePath + "/" + method
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to build %s request: %w", method, err)
	}
	httpReq.Header.Set(headerContentType, contentTypeJSON)
	httpReq.Header.Set(headerAuthorization, "Bearer "+u.token)

	resp, err := u.httpClient.Do(httpReq) //nolint:gosec // G704: endpoint is the trusted ACTIONS_RESULTS_URL Twirp service.
	if err != nil {
		return fmt.Errorf("failed to call %s: %w", method, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%w: %s returned status %d: %s", errUtils.ErrArtifactDownloadFailed, method, resp.StatusCode, string(respBody))
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("failed to decode %s response: %w", method, err)
	}

	return nil
}

// ListArtifacts calls the ListArtifacts Twirp endpoint, returning the artifacts
// in the current run (across its jobs).
func (u *runtimeUploader) ListArtifacts(ctx context.Context, req *runtimeListArtifactsRequest) (*runtimeListArtifactsResponse, error) {
	defer perf.Track(nil, "github.runtimeUploader.ListArtifacts")()

	var result runtimeListArtifactsResponse
	if err := u.postRuntimeJSON(ctx, "ListArtifacts", req, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// GetSignedArtifactURL calls the GetSignedArtifactURL Twirp endpoint, returning
// a signed blob URL for the named artifact in the current run.
func (u *runtimeUploader) GetSignedArtifactURL(ctx context.Context, req *getSignedArtifactURLRequest) (*getSignedArtifactURLResponse, error) {
	defer perf.Track(nil, "github.runtimeUploader.GetSignedArtifactURL")()

	var result getSignedArtifactURLResponse
	if err := u.postRuntimeJSON(ctx, "GetSignedArtifactURL", req, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// getBackendIDsFromToken parses the ACTIONS_RUNTIME_TOKEN JWT to extract backend IDs.
// The token contains a "scp" claim with space-separated scopes like:
// "Actions.Results:ce7f54c7-61c7-4aae-887f-30da475f5f1a:ca395085-040a-526b-2ce8-bdc85f692774".
func getBackendIDsFromToken(token string) (*backendIDs, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid JWT format: expected 3 parts, got %d", len(parts))
	}

	// Decode the payload (second part).
	payload := parts[1]
	// Add padding if needed.
	switch len(payload) % 4 {
	case 2:
		payload += "=="
	case 3:
		payload += "="
	}

	decoded, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to decode JWT payload: %w", err)
	}

	var claims struct {
		Scp string `json:"scp"`
	}
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return nil, fmt.Errorf("failed to parse JWT claims: %w", err)
	}

	// Parse the scp claim to find Actions.Results scope.
	for _, scope := range strings.Split(claims.Scp, " ") {
		if strings.HasPrefix(scope, "Actions.Results:") {
			scopeParts := strings.Split(scope, ":")
			if len(scopeParts) != 3 {
				return nil, fmt.Errorf("invalid Actions.Results scope format: %s", scope)
			}
			return &backendIDs{
				WorkflowRunBackendID:    scopeParts[1],
				WorkflowJobRunBackendID: scopeParts[2],
			}, nil
		}
	}

	return nil, fmt.Errorf("Actions.Results scope not found in token")
}

func init() {
	artifact.Register(storeName, NewStore)
}

// Ensure Store implements artifact.Backend.
var _ artifact.Backend = (*Store)(nil)
