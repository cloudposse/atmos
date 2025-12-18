package github

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"time"

	"github.com/google/go-github/v59/github"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/planfile"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	storeName = "github-artifacts"

	// Metadata is stored as a JSON file within the artifact.
	metadataFilename = "metadata.json"
	planFilename     = "plan.tfplan"

	// Default retention for artifacts.
	defaultRetentionDays = 7
)

// Store implements the planfile.Store interface using GitHub Actions Artifacts.
type Store struct {
	client        *github.Client
	owner         string
	repo          string
	retentionDays int
}

// NewStore creates a new GitHub Artifacts store.
func NewStore(opts planfile.StoreOptions) (planfile.Store, error) {
	defer perf.Track(opts.AtmosConfig, "github.NewStore")()

	// Get GitHub token from environment.
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		token = os.Getenv("GH_TOKEN")
	}
	if token == "" {
		return nil, errUtils.ErrGitHubTokenNotFound
	}

	// Get repository info.
	owner, _ := opts.Options["owner"].(string)
	repo, _ := opts.Options["repo"].(string)

	// Try to get from GITHUB_REPOSITORY environment variable.
	if owner == "" || repo == "" {
		ghRepo := os.Getenv("GITHUB_REPOSITORY")
		if ghRepo != "" {
			parts := splitRepoString(ghRepo)
			if len(parts) == 2 {
				if owner == "" {
					owner = parts[0]
				}
				if repo == "" {
					repo = parts[1]
				}
			}
		}
	}

	if owner == "" || repo == "" {
		return nil, fmt.Errorf("%w: owner and repo are required for GitHub Artifacts store", errUtils.ErrPlanfileStoreNotFound)
	}

	retentionDays := defaultRetentionDays
	if days, ok := opts.Options["retention_days"].(int); ok && days > 0 {
		retentionDays = days
	}

	// Create GitHub client.
	client := github.NewClient(nil).WithAuthToken(token)

	return &Store{
		client:        client,
		owner:         owner,
		repo:          repo,
		retentionDays: retentionDays,
	}, nil
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
// Note: GitHub Actions artifacts must be uploaded using the @actions/artifact toolkit
// within a workflow. This implementation provides the interface but may require
// shell-out to the actions toolkit for actual uploads.
func (s *Store) Upload(ctx context.Context, key string, data io.Reader, metadata *planfile.Metadata) error {
	defer perf.Track(nil, "github.Upload")()

	// Read the planfile data.
	planData, err := io.ReadAll(data)
	if err != nil {
		return fmt.Errorf("%w: failed to read planfile data: %v", errUtils.ErrPlanfileUploadFailed, err)
	}

	// Create a zip archive containing the plan and metadata.
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)

	// Add the planfile.
	planWriter, err := zipWriter.Create(planFilename)
	if err != nil {
		return fmt.Errorf("%w: failed to create zip entry for plan: %v", errUtils.ErrPlanfileUploadFailed, err)
	}
	if _, err := planWriter.Write(planData); err != nil {
		return fmt.Errorf("%w: failed to write plan to zip: %v", errUtils.ErrPlanfileUploadFailed, err)
	}

	// Add metadata if provided.
	if metadata != nil {
		metadataWriter, err := zipWriter.Create(metadataFilename)
		if err != nil {
			return fmt.Errorf("%w: failed to create zip entry for metadata: %v", errUtils.ErrPlanfileUploadFailed, err)
		}
		metadataJSON, err := json.MarshalIndent(metadata, "", "  ")
		if err != nil {
			return fmt.Errorf("%w: failed to marshal metadata: %v", errUtils.ErrPlanfileUploadFailed, err)
		}
		if _, err := metadataWriter.Write(metadataJSON); err != nil {
			return fmt.Errorf("%w: failed to write metadata to zip: %v", errUtils.ErrPlanfileUploadFailed, err)
		}
	}

	if err := zipWriter.Close(); err != nil {
		return fmt.Errorf("%w: failed to close zip archive: %v", errUtils.ErrPlanfileUploadFailed, err)
	}

	// Note: GitHub Actions artifact upload requires the @actions/artifact toolkit.
	// This implementation creates the artifact data but actual upload in GitHub Actions
	// typically needs to use the artifact client or REST API with proper authentication.
	// For now, we'll use the REST API to upload.

	artifactName := s.artifactName(key)

	// GitHub's artifact upload API is complex and typically uses the actions toolkit.
	// For direct API access, we'd need to use the Artifacts API endpoints.
	// This is a simplified implementation that may need enhancement.

	_ = artifactName // Use artifact name (will be used with proper API)
	_ = buf          // Use buffer (will be used with proper API)

	return fmt.Errorf("%w: GitHub Artifacts upload requires running within GitHub Actions with proper toolkit", errUtils.ErrPlanfileUploadFailed)
}

// Download downloads a planfile from GitHub artifacts.
func (s *Store) Download(ctx context.Context, key string) (io.ReadCloser, *planfile.Metadata, error) {
	defer perf.Track(nil, "github.Download")()

	artifactName := s.artifactName(key)

	// List artifacts to find the one matching our key.
	artifacts, _, err := s.client.Actions.ListArtifacts(ctx, s.owner, s.repo, &github.ListOptions{
		PerPage: 100,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("%w: failed to list artifacts: %v", errUtils.ErrPlanfileDownloadFailed, err)
	}

	var targetArtifact *github.Artifact
	for _, artifact := range artifacts.Artifacts {
		if artifact.GetName() == artifactName {
			targetArtifact = artifact
			break
		}
	}

	if targetArtifact == nil {
		return nil, nil, fmt.Errorf("%w: %s", errUtils.ErrPlanfileNotFound, key)
	}

	// Download the artifact (follow up to 10 redirects).
	url, _, err := s.client.Actions.DownloadArtifact(ctx, s.owner, s.repo, targetArtifact.GetID(), 10)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: failed to get artifact download URL: %v", errUtils.ErrPlanfileDownloadFailed, err)
	}

	// Fetch the artifact content.
	resp, err := http.Get(url.String())
	if err != nil {
		return nil, nil, fmt.Errorf("%w: failed to download artifact: %v", errUtils.ErrPlanfileDownloadFailed, err)
	}
	defer resp.Body.Close()

	// Read the zip content.
	zipData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: failed to read artifact content: %v", errUtils.ErrPlanfileDownloadFailed, err)
	}

	// Open the zip archive.
	zipReader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return nil, nil, fmt.Errorf("%w: failed to open artifact zip: %v", errUtils.ErrPlanfileDownloadFailed, err)
	}

	var planData []byte
	var metadata *planfile.Metadata

	for _, file := range zipReader.File {
		switch file.Name {
		case planFilename:
			rc, err := file.Open()
			if err != nil {
				return nil, nil, fmt.Errorf("%w: failed to open plan file in zip: %v", errUtils.ErrPlanfileDownloadFailed, err)
			}
			planData, err = io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return nil, nil, fmt.Errorf("%w: failed to read plan file: %v", errUtils.ErrPlanfileDownloadFailed, err)
			}

		case metadataFilename:
			rc, err := file.Open()
			if err != nil {
				continue // Metadata is optional.
			}
			var m planfile.Metadata
			if err := json.NewDecoder(rc).Decode(&m); err == nil {
				metadata = &m
			}
			rc.Close()
		}
	}

	if planData == nil {
		return nil, nil, fmt.Errorf("%w: plan file not found in artifact", errUtils.ErrPlanfileDownloadFailed)
	}

	return io.NopCloser(bytes.NewReader(planData)), metadata, nil
}

// Delete deletes a planfile artifact.
func (s *Store) Delete(ctx context.Context, key string) error {
	defer perf.Track(nil, "github.Delete")()

	artifactName := s.artifactName(key)

	// List artifacts to find the one matching our key.
	artifacts, _, err := s.client.Actions.ListArtifacts(ctx, s.owner, s.repo, &github.ListOptions{
		PerPage: 100,
	})
	if err != nil {
		return fmt.Errorf("%w: failed to list artifacts: %v", errUtils.ErrPlanfileDeleteFailed, err)
	}

	for _, artifact := range artifacts.Artifacts {
		if artifact.GetName() == artifactName {
			_, err := s.client.Actions.DeleteArtifact(ctx, s.owner, s.repo, artifact.GetID())
			if err != nil {
				return fmt.Errorf("%w: failed to delete artifact: %v", errUtils.ErrPlanfileDeleteFailed, err)
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
	opts := &github.ListOptions{PerPage: 100}

	for {
		artifacts, resp, err := s.client.Actions.ListArtifacts(ctx, s.owner, s.repo, opts)
		if err != nil {
			return nil, fmt.Errorf("%w: failed to list artifacts: %v", errUtils.ErrPlanfileListFailed, err)
		}

		for _, artifact := range artifacts.Artifacts {
			name := artifact.GetName()
			// Only include planfile artifacts.
			if len(name) < 9 || name[:9] != "planfile-" {
				continue
			}

			// Extract the key from artifact name.
			key := desanitizeKey(name[9:])

			// Check prefix match.
			if prefix != "" && !hasPrefix(key, prefix) {
				continue
			}

			files = append(files, planfile.PlanfileInfo{
				Key:          key,
				Size:         int64(artifact.GetSizeInBytes()),
				LastModified: artifact.GetCreatedAt().Time,
			})
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
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

	artifacts, _, err := s.client.Actions.ListArtifacts(ctx, s.owner, s.repo, &github.ListOptions{
		PerPage: 100,
	})
	if err != nil {
		return false, fmt.Errorf("failed to list artifacts: %w", err)
	}

	for _, artifact := range artifacts.Artifacts {
		if artifact.GetName() == artifactName {
			return true, nil
		}
	}

	return false, nil
}

// GetMetadata retrieves metadata for a planfile artifact.
func (s *Store) GetMetadata(ctx context.Context, key string) (*planfile.Metadata, error) {
	defer perf.Track(nil, "github.GetMetadata")()

	artifactName := s.artifactName(key)

	artifacts, _, err := s.client.Actions.ListArtifacts(ctx, s.owner, s.repo, &github.ListOptions{
		PerPage: 100,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list artifacts: %w", err)
	}

	for _, artifact := range artifacts.Artifacts {
		if artifact.GetName() == artifactName {
			// Return basic metadata from artifact info.
			// Full metadata would require downloading the artifact.
			return &planfile.Metadata{
				CreatedAt: artifact.GetCreatedAt().Time,
				ExpiresAt: func() *time.Time {
					t := artifact.GetExpiresAt().Time
					return &t
				}(),
			}, nil
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

func init() {
	planfile.Register(storeName, NewStore)
}

// Ensure Store implements planfile.Store.
var _ planfile.Store = (*Store)(nil)
