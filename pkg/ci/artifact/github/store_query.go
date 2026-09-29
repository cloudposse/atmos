package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/artifact"
	log "github.com/cloudposse/atmos/pkg/logger"
)

// envGitHubRunID is the environment variable holding the current workflow run ID.
const envGitHubRunID = "GITHUB_RUN_ID"

// listArtifactsStatusError is returned when the list artifacts REST API answers with a non-200 status.
// It preserves the historical error text and matches errUtils.ErrArtifactListFailed via errors.Is.
type listArtifactsStatusError struct {
	StatusCode int
	Body       string
}

// Error renders the status and, when present, the response body.
func (e *listArtifactsStatusError) Error() string { //nolint:lintroller // Trivial error-interface method; perf.Track overhead is unwarranted.
	if e.Body == "" {
		return fmt.Sprintf("%s: status %d", errUtils.ErrArtifactListFailed, e.StatusCode)
	}

	return fmt.Sprintf("%s: status %d: %s", errUtils.ErrArtifactListFailed, e.StatusCode, e.Body)
}

// Unwrap lets errors.Is match errUtils.ErrArtifactListFailed.
func (e *listArtifactsStatusError) Unwrap() error { //nolint:lintroller // Trivial error-interface method; perf.Track overhead is unwarranted.
	return errUtils.ErrArtifactListFailed
}

// listArtifactsQuery describes one page request to the list artifacts REST API.
type listArtifactsQuery struct {
	perPage int
	page    int
	// name is an optional exact artifact name filter, applied server-side.
	name string
	// runID, when non-empty, scopes the listing to a single workflow run.
	runID string
}

// path returns the REST path (without host) for the query.
func (q listArtifactsQuery) path(owner, repo string) string {
	if q.runID != "" {
		return fmt.Sprintf("/repos/%s/%s/actions/runs/%s/artifacts", owner, repo, url.PathEscape(q.runID))
	}

	return fmt.Sprintf("/repos/%s/%s/actions/artifacts", owner, repo)
}

// values returns the escaped query parameters for the query.
func (q listArtifactsQuery) values() url.Values {
	values := url.Values{}
	values.Set("per_page", strconv.Itoa(q.perPage))
	values.Set("page", strconv.Itoa(q.page))
	if q.name != "" {
		values.Set("name", q.name)
	}

	return values
}

// listArtifacts fetches one page from the list artifacts REST API, retrying transient failures.
func (s *Store) listArtifacts(ctx context.Context, q listArtifactsQuery) (*listArtifactsResponse, int, error) {
	var lastErr error
	for attempt := 1; attempt <= listArtifactsMaxAttempts; attempt++ {
		result, err := s.listArtifactsOnce(ctx, q)
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

func (s *Store) listArtifactsOnce(ctx context.Context, q listArtifactsQuery) (*listArtifactsAttemptResult, error) {
	endpoint := s.baseURL + q.path(s.owner, s.repo) + "?" + q.values().Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
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
		}, &listArtifactsStatusError{StatusCode: resp.StatusCode, Body: string(body)}
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

// findArtifactsByName lists the artifacts whose name is exactly artifactName, newest first.
// The filter is applied server-side, so the result is correct on repositories with many artifacts.
func (s *Store) findArtifactsByName(ctx context.Context, artifactName string) ([]githubArtifact, error) {
	resp, _, err := s.listArtifacts(ctx, listArtifactsQuery{perPage: githubPaginationLimit, page: 1, name: artifactName})
	if err != nil {
		return nil, err
	}

	// Keep an equality guard so a server that ignores the filter can never return a wrong artifact.
	matches := make([]githubArtifact, 0, len(resp.Artifacts))
	for _, a := range resp.Artifacts {
		if a.Name == artifactName {
			matches = append(matches, a)
		}
	}

	return matches, nil
}

// listPageFetcher fetches one page of artifacts and returns the next page number (0 when done).
type listPageFetcher func(ctx context.Context, page int) (*listArtifactsResponse, int, error)

// repoPageFetcher fetches pages from the repository-wide artifacts listing.
func (s *Store) repoPageFetcher() listPageFetcher {
	return func(ctx context.Context, page int) (*listArtifactsResponse, int, error) {
		return s.listArtifacts(ctx, listArtifactsQuery{perPage: githubPaginationLimit, page: page})
	}
}

// runPageFetcher fetches pages from the artifacts listing of a single workflow run.
func (s *Store) runPageFetcher(runID string) listPageFetcher {
	return func(ctx context.Context, page int) (*listArtifactsResponse, int, error) {
		return s.listArtifacts(ctx, listArtifactsQuery{perPage: githubPaginationLimit, page: page, runID: runID})
	}
}

// listWithRunFallback lists the artifacts matching prefix, newest first.
// The repository-wide listing is the primary path. If it fails with a server error (HTTP 5xx)
// after retries and GITHUB_RUN_ID is set, the listing restarts from scratch against the
// current run's artifacts so that the results never contain duplicates.
func (s *Store) listWithRunFallback(ctx context.Context, prefix string) ([]artifact.ArtifactInfo, error) {
	files, err := s.collectArtifacts(ctx, prefix, s.repoPageFetcher())
	if err == nil {
		return files, nil
	}

	runID := os.Getenv(envGitHubRunID)
	var statusErr *listArtifactsStatusError
	if runID == "" || !errors.As(err, &statusErr) || statusErr.StatusCode < http.StatusInternalServerError {
		return nil, err
	}

	log.Warn("Repository-wide artifact listing failed; results are limited to the current workflow run",
		"status", statusErr.StatusCode, "run_id", runID, "owner", s.owner, "repo", s.repo)

	return s.collectArtifacts(ctx, prefix, s.runPageFetcher(runID))
}

// collectArtifacts pages through fetch, keeps the artifacts under the store prefix that match
// the query prefix, and returns them sorted by last modified (newest first).
func (s *Store) collectArtifacts(ctx context.Context, prefix string, fetch listPageFetcher) ([]artifact.ArtifactInfo, error) {
	var files []artifact.ArtifactInfo
	page := 1

	for {
		resp, nextPage, err := fetch(ctx, page)
		if err != nil {
			return nil, err
		}

		for _, a := range resp.Artifacts {
			// If a prefix is configured, only include matching artifacts and strip it.
			key := s.stripArtifactPrefix(a.Name)
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
