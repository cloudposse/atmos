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
	"github.com/cloudposse/atmos/pkg/ci/plugins/terraform/planfile"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	storeName = "github-artifacts"

	// Metadata is stored as a JSON sidecar within the artifact zip.
	metadataFilename = "metadata.json"
	bundleFilename   = "bundle.tar"

	// Default retention for artifacts.
	defaultRetentionDays = 7

	// ArtifactNamePrefix is the prefix for planfile artifact names.
	artifactNamePrefix = "planfile-"

	// ArtifactPrefixLen is the length of the artifact name prefix.
	artifactPrefixLen = len(artifactNamePrefix)

	// GithubPaginationLimit is the max number of items per page for GitHub API.
	githubPaginationLimit = 100

	// HTTPTimeout is the timeout for HTTP requests.
	httpTimeout = 30 * time.Second

	// artifactServicePath is the Twirp service path for artifact operations.
	artifactServicePath = "twirp/github.actions.results.api.v1.ArtifactService"

	// artifactVersion is the artifact API version.
	artifactVersion = 4
)

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

// artifact represents a GitHub Actions artifact from the REST API.
type artifact struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	SizeInBytes int64     `json:"size_in_bytes"`
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// listArtifactsResponse is the response from the list artifacts REST API.
type listArtifactsResponse struct {
	TotalCount int        `json:"total_count"`
	Artifacts  []artifact `json:"artifacts"`
}

// Store implements the planfile.Store interface using GitHub Actions Artifacts.
type Store struct {
	httpClient    *http.Client
	baseURL       string
	uploader      artifactUploader
	owner         string
	repo          string
	retentionDays int
}

// NewStore creates a new GitHub Artifacts store.
func NewStore(opts planfile.StoreOptions) (planfile.Store, error) {
	defer perf.Track(opts.AtmosConfig, "github.NewStore")()

	token := getGitHubToken()
	if token == "" {
		return nil, errUtils.ErrGitHubTokenNotFound
	}

	owner, repo := getRepoInfo(opts.Options)
	if owner == "" || repo == "" {
		return nil, fmt.Errorf("%w: owner and repo are required for GitHub Artifacts store", errUtils.ErrPlanfileStoreNotFound)
	}

	retentionDays := getRetentionDays(opts.Options)

	// Create the runtime uploader if running inside GitHub Actions.
	var uploader artifactUploader
	runtimeToken := os.Getenv("ACTIONS_RUNTIME_TOKEN")
	resultsURL := os.Getenv("ACTIONS_RESULTS_URL")
	if runtimeToken != "" && resultsURL != "" {
		uploader = newRuntimeUploader(resultsURL, runtimeToken)
	}

	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	httpClient := oauth2.NewClient(context.Background(), ts)
	httpClient.Timeout = httpTimeout

	return &Store{
		httpClient:    httpClient,
		baseURL:       "https://api.github.com",
		uploader:      uploader,
		owner:         owner,
		repo:          repo,
		retentionDays: retentionDays,
	}, nil
}

// getGitHubToken returns the GitHub token from environment variables.
func getGitHubToken() string {
	token := os.Getenv("GITHUB_TOKEN")
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

// artifactName returns the artifact name for a given key.
func (s *Store) artifactName(key string) string {
	// Replace path separators with dashes for artifact naming.
	return "planfile-" + sanitizeKey(key)
}

// Upload uploads a planfile as a GitHub artifact.
// This requires running within GitHub Actions with ACTIONS_RUNTIME_TOKEN and
// ACTIONS_RESULTS_URL environment variables set.
func (s *Store) Upload(ctx context.Context, key string, data io.Reader, metadata *planfile.Metadata) error {
	defer perf.Track(nil, "github.Upload")()

	if s.uploader == nil {
		return fmt.Errorf("%w: GitHub Artifacts upload requires running within GitHub Actions (ACTIONS_RUNTIME_TOKEN and ACTIONS_RESULTS_URL must be set)", errUtils.ErrNotImplemented)
	}

	// Parse backend IDs from runtime token.
	runtimeToken := os.Getenv("ACTIONS_RUNTIME_TOKEN")
	ids, err := getBackendIDsFromToken(runtimeToken)
	if err != nil {
		return fmt.Errorf("%w: failed to parse runtime token: %w", errUtils.ErrPlanfileUploadFailed, err)
	}

	// Read the bundle data (opaque tar blob — SHA256 is set by the caller).
	bundleData, err := io.ReadAll(data)
	if err != nil {
		return fmt.Errorf("%w: failed to read bundle data: %w", errUtils.ErrPlanfileUploadFailed, err)
	}

	// Create a zip archive wrapping the tar bundle (GitHub Actions API requires zip).
	zipData, err := createArtifactZip(bundleData, metadata)
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
		return fmt.Errorf("%w: failed to create artifact: %w", errUtils.ErrPlanfileUploadFailed, err)
	}

	if createResp.SignedUploadURL == "" {
		return fmt.Errorf("%w: artifact service returned empty upload URL", errUtils.ErrPlanfileUploadFailed)
	}

	// Step 2: Upload the zip data to the signed URL.
	if err := s.uploader.UploadBlob(ctx, createResp.SignedUploadURL, zipData); err != nil {
		return fmt.Errorf("%w: failed to upload artifact data: %w", errUtils.ErrPlanfileUploadFailed, err)
	}

	// Step 3: Finalize the artifact.
	finalizeReq := &finalizeArtifactRequest{
		Name:                    artifactName,
		Size:                    int64(len(zipData)),
		WorkflowRunBackendID:    ids.WorkflowRunBackendID,
		WorkflowJobRunBackendID: ids.WorkflowJobRunBackendID,
	}

	if _, err := s.uploader.FinalizeArtifact(ctx, finalizeReq); err != nil {
		return fmt.Errorf("%w: failed to finalize artifact: %w", errUtils.ErrPlanfileUploadFailed, err)
	}

	return nil
}

// createArtifactZip creates a zip archive wrapping the tar bundle as a single entry.
// The GitHub Actions runtime API requires zip format, so the tar bundle is wrapped
// in an outer zip. Metadata is stored as a sidecar entry for convenience.
func createArtifactZip(bundleData []byte, metadata *planfile.Metadata) ([]byte, error) {
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)

	// Add the tar bundle as a single entry.
	bundleWriter, err := zipWriter.Create(bundleFilename)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create zip entry for bundle: %w", errUtils.ErrPlanfileUploadFailed, err)
	}
	if _, err := bundleWriter.Write(bundleData); err != nil {
		return nil, fmt.Errorf("%w: failed to write bundle to zip: %w", errUtils.ErrPlanfileUploadFailed, err)
	}

	// Add metadata as a sidecar entry.
	if metadata != nil {
		metadataWriter, err := zipWriter.Create(metadataFilename)
		if err != nil {
			return nil, fmt.Errorf("%w: failed to create zip entry for metadata: %w", errUtils.ErrPlanfileUploadFailed, err)
		}
		metadataJSON, err := json.MarshalIndent(metadata, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("%w: failed to marshal metadata: %w", errUtils.ErrPlanfileUploadFailed, err)
		}
		if _, err := metadataWriter.Write(metadataJSON); err != nil {
			return nil, fmt.Errorf("%w: failed to write metadata to zip: %w", errUtils.ErrPlanfileUploadFailed, err)
		}
	}

	if err := zipWriter.Close(); err != nil {
		return nil, fmt.Errorf("%w: failed to close zip archive: %w", errUtils.ErrPlanfileUploadFailed, err)
	}

	return buf.Bytes(), nil
}

// listArtifacts calls GET /repos/{owner}/{repo}/actions/artifacts with pagination params.
func (s *Store) listArtifacts(ctx context.Context, perPage, page int) (*listArtifactsResponse, int, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/actions/artifacts?per_page=%d&page=%d", s.baseURL, s.owner, s.repo, perPage, page)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create list artifacts request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list artifacts: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, 0, fmt.Errorf("list artifacts returned status %d: %s", resp.StatusCode, string(body))
	}

	var result listArtifactsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, 0, fmt.Errorf("failed to decode list artifacts response: %w", err)
	}

	nextPage := parseNextPage(resp.Header.Get("Link"))

	return &result, nextPage, nil
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

// Download downloads a planfile from GitHub artifacts.
func (s *Store) Download(ctx context.Context, key string) (io.ReadCloser, *planfile.Metadata, error) {
	defer perf.Track(nil, "github.Download")()

	a, err := s.findArtifact(ctx, key)
	if err != nil {
		return nil, nil, err
	}

	zipData, err := s.downloadArtifactContent(ctx, a.ID)
	if err != nil {
		return nil, nil, err
	}

	return extractBundleFromZip(zipData)
}

// findArtifact finds an artifact by key.
func (s *Store) findArtifact(ctx context.Context, key string) (*artifact, error) {
	artifactName := s.artifactName(key)

	resp, _, err := s.listArtifacts(ctx, githubPaginationLimit, 1)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to list artifacts for download: %w", errUtils.ErrPlanfileDownloadFailed, err)
	}

	for i := range resp.Artifacts {
		if resp.Artifacts[i].Name == artifactName {
			return &resp.Artifacts[i], nil
		}
	}

	return nil, fmt.Errorf("%w: %s", errUtils.ErrPlanfileNotFound, key)
}

// downloadArtifactContent downloads artifact content as zip data.
func (s *Store) downloadArtifactContent(ctx context.Context, artifactID int64) ([]byte, error) {
	downloadURL, err := s.downloadArtifactURL(ctx, artifactID)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to get artifact download URL: %w", errUtils.ErrPlanfileDownloadFailed, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create download request: %w", errUtils.ErrPlanfileDownloadFailed, err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to download artifact: %w", errUtils.ErrPlanfileDownloadFailed, err)
	}
	defer resp.Body.Close()

	zipData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to read artifact content: %w", errUtils.ErrPlanfileDownloadFailed, err)
	}

	return zipData, nil
}

// extractBundleFromZip extracts the tar bundle and metadata from the outer zip.
// Returns the tar bundle data as an io.ReadCloser and metadata.
func extractBundleFromZip(zipData []byte) (io.ReadCloser, *planfile.Metadata, error) {
	zipReader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return nil, nil, fmt.Errorf("%w: failed to open artifact zip: %w", errUtils.ErrPlanfileDownloadFailed, err)
	}

	var bundleData []byte
	var metadata *planfile.Metadata

	for _, file := range zipReader.File {
		switch file.Name {
		case bundleFilename:
			bundleData, err = readZipFile(file)
			if err != nil {
				return nil, nil, err
			}
		case metadataFilename:
			metadata = readMetadataFile(file)
		}
	}

	if bundleData == nil {
		return nil, nil, fmt.Errorf("%w: bundle not found in artifact zip", errUtils.ErrPlanfileDownloadFailed)
	}

	return io.NopCloser(bytes.NewReader(bundleData)), metadata, nil
}

// readZipFile reads a file from the zip archive.
func readZipFile(file *zip.File) ([]byte, error) {
	rc, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("%w: failed to open plan file in zip: %w", errUtils.ErrPlanfileDownloadFailed, err)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to read plan file: %w", errUtils.ErrPlanfileDownloadFailed, err)
	}
	return data, nil
}

// readMetadataFile reads metadata from a zip file (returns nil on error).
func readMetadataFile(file *zip.File) *planfile.Metadata {
	rc, err := file.Open()
	if err != nil {
		return nil
	}
	defer rc.Close()

	var m planfile.Metadata
	if err := json.NewDecoder(rc).Decode(&m); err != nil {
		return nil
	}
	return &m
}

// Delete deletes a planfile artifact.
func (s *Store) Delete(ctx context.Context, key string) error {
	defer perf.Track(nil, "github.Delete")()

	artifactName := s.artifactName(key)

	// List artifacts to find the one matching our key.
	resp, _, err := s.listArtifacts(ctx, githubPaginationLimit, 1)
	if err != nil {
		return fmt.Errorf("%w: failed to list artifacts for deletion: %w", errUtils.ErrPlanfileDeleteFailed, err)
	}

	for _, a := range resp.Artifacts {
		if a.Name == artifactName {
			if err := s.deleteArtifact(ctx, a.ID); err != nil {
				return fmt.Errorf("%w: failed to delete artifact: %w", errUtils.ErrPlanfileDeleteFailed, err)
			}
			return nil
		}
	}

	return nil // Already deleted or never existed.
}

// List lists planfile artifacts.
func (s *Store) List(ctx context.Context, prefix string) ([]planfile.PlanfileInfo, error) {
	defer perf.Track(nil, "github.List")()

	var files []planfile.PlanfileInfo
	page := 1

	for {
		resp, nextPage, err := s.listArtifacts(ctx, githubPaginationLimit, page)
		if err != nil {
			return nil, fmt.Errorf("%w: failed to list artifacts: %w", errUtils.ErrPlanfileListFailed, err)
		}

		for _, a := range resp.Artifacts {
			name := a.Name
			// Only include planfile artifacts.
			if len(name) < artifactPrefixLen || name[:artifactPrefixLen] != artifactNamePrefix {
				continue
			}

			// Extract the key from artifact name.
			key := desanitizeKey(name[artifactPrefixLen:])

			// Check prefix match.
			if prefix != "" && !hasPrefix(key, prefix) {
				continue
			}

			files = append(files, planfile.PlanfileInfo{
				Key:          key,
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

// Exists checks if a planfile artifact exists.
func (s *Store) Exists(ctx context.Context, key string) (bool, error) {
	defer perf.Track(nil, "github.Exists")()

	artifactName := s.artifactName(key)

	resp, _, err := s.listArtifacts(ctx, githubPaginationLimit, 1)
	if err != nil {
		return false, fmt.Errorf("%w: failed to check artifact existence: %w", errUtils.ErrPlanfileListFailed, err)
	}

	for _, a := range resp.Artifacts {
		if a.Name == artifactName {
			return true, nil
		}
	}

	return false, nil
}

// GetMetadata retrieves metadata for a planfile artifact.
func (s *Store) GetMetadata(ctx context.Context, key string) (*planfile.Metadata, error) {
	defer perf.Track(nil, "github.GetMetadata")()

	artifactName := s.artifactName(key)

	resp, _, err := s.listArtifacts(ctx, githubPaginationLimit, 1)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to get artifact metadata: %w", errUtils.ErrPlanfileListFailed, err)
	}

	for _, a := range resp.Artifacts {
		if a.Name == artifactName {
			// Return basic metadata from artifact info.
			// Full metadata would require downloading the artifact.
			meta := &planfile.Metadata{}
			meta.CreatedAt = a.CreatedAt
			meta.ExpiresAt = func() *time.Time {
				t := a.ExpiresAt
				return &t
			}()
			return meta, nil
		}
	}

	return nil, fmt.Errorf("%w: %s", errUtils.ErrPlanfileNotFound, key)
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
	planfile.Register(storeName, NewStore)
}

// Ensure Store implements planfile.Store.
var _ planfile.Store = (*Store)(nil)
