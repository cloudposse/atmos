package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/artifact"
	log "github.com/cloudposse/atmos/pkg/logger"
)

const (
	testQueryOwner = "testowner"
	testQueryRepo  = "testrepo"

	// Paths served by the fake GitHub API in this file.
	testRepoArtifactsPath = "/repos/" + testQueryOwner + "/" + testQueryRepo + "/actions/artifacts"
	testRunID             = "424242"
	testRunArtifactsPath  = "/repos/" + testQueryOwner + "/" + testQueryRepo + "/actions/runs/" + testRunID + "/artifacts"
)

// testQueryArtifact is a fake artifact served by the fake GitHub API.
type testQueryArtifact struct {
	ID      int64
	Name    string
	Size    int64
	Created time.Time
}

func (a testQueryArtifact) toMap() map[string]any {
	return map[string]any{
		"id":            a.ID,
		"name":          a.Name,
		"size_in_bytes": a.Size,
		"created_at":    a.Created.Format(time.RFC3339),
		"expires_at":    a.Created.Add(7 * 24 * time.Hour).Format(time.RFC3339),
	}
}

func writeArtifactsJSON(t *testing.T, w http.ResponseWriter, artifacts []testQueryArtifact) {
	t.Helper()

	items := make([]map[string]any, 0, len(artifacts))
	for _, a := range artifacts {
		items = append(items, a.toMap())
	}
	body, err := json.Marshal(map[string]any{"total_count": len(items), "artifacts": items})
	require.NoError(t, err)

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

// recordingServer wraps an httptest server and records every request it receives.
type recordingServer struct {
	*httptest.Server

	mu       sync.Mutex
	requests []string
}

func newRecordingServer(t *testing.T, handler http.HandlerFunc) *recordingServer {
	t.Helper()

	rs := &recordingServer{}
	rs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rs.mu.Lock()
		rs.requests = append(rs.requests, r.Method+" "+r.URL.RequestURI())
		rs.mu.Unlock()
		handler(w, r)
	}))
	t.Cleanup(rs.Close)

	return rs
}

// count returns the number of recorded requests whose method and path match.
func (rs *recordingServer) count(method, path string) int {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	n := 0
	for _, req := range rs.requests {
		if strings.HasPrefix(req, method+" "+path+"?") || req == method+" "+path {
			n++
		}
	}

	return n
}

func (rs *recordingServer) store() *Store {
	return &Store{
		httpClient: rs.Client(),
		baseURL:    rs.URL,
		prefix:     "planfile",
		owner:      testQueryOwner,
		repo:       testQueryRepo,
	}
}

// fastListRetries makes the list retry backoff negligible and restores it after the test.
func fastListRetries(t *testing.T) {
	t.Helper()

	old := listArtifactsRetryBaseDelay
	listArtifactsRetryBaseDelay = time.Nanosecond
	t.Cleanup(func() { listArtifactsRetryBaseDelay = old })
}

// captureWarnings captures log output at warn level and restores the logger after the test.
func captureWarnings(t *testing.T) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer
	oldLevel := log.GetLevel()
	log.SetLevel(log.WarnLevel)
	log.SetOutput(&buf)
	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
		log.SetLevel(oldLevel)
	})

	return &buf
}

// nameFilteringHandler emulates the GitHub API. Unfiltered repo-wide listings fail with an empty
// HTTP 500 (the production regression), name-filtered listings return the exact matches, and
// DELETE succeeds.
func nameFilteringHandler(t *testing.T, artifacts []testQueryArtifact) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		case r.URL.Path == testRepoArtifactsPath && r.URL.Query().Get("name") == "":
			w.WriteHeader(http.StatusInternalServerError)
		case r.URL.Path == testRepoArtifactsPath:
			var matches []testQueryArtifact
			for _, a := range artifacts {
				if a.Name == r.URL.Query().Get("name") {
					matches = append(matches, a)
				}
			}
			writeArtifactsJSON(t, w, matches)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

func TestStore_ExactKeyOperations_UseServerSideNameFilter(t *testing.T) {
	now := time.Now()
	const key = "prod/mycomponent/sha1.tfplan"
	const wantName = "planfile-prod--mycomponent--sha1.tfplan"

	artifacts := []testQueryArtifact{
		{ID: 7, Name: wantName, Size: 10, Created: now},
		{ID: 8, Name: "planfile-prod--other--sha2.tfplan", Size: 20, Created: now},
	}

	tests := []struct {
		name string
		run  func(t *testing.T, s *Store)
	}{
		{
			name: "findArtifact",
			run: func(t *testing.T, s *Store) {
				a, err := s.findArtifact(context.Background(), key)
				require.NoError(t, err)
				assert.Equal(t, int64(7), a.ID)
			},
		},
		{
			name: "Exists",
			run: func(t *testing.T, s *Store) {
				ok, err := s.Exists(context.Background(), key)
				require.NoError(t, err)
				assert.True(t, ok)
			},
		},
		{
			name: "Exists for missing key",
			run: func(t *testing.T, s *Store) {
				ok, err := s.Exists(context.Background(), "prod/missing/sha.tfplan")
				require.NoError(t, err)
				assert.False(t, ok)
			},
		},
		{
			name: "GetMetadata",
			run: func(t *testing.T, s *Store) {
				meta, err := s.GetMetadata(context.Background(), key)
				require.NoError(t, err)
				require.NotNil(t, meta)
				assert.WithinDuration(t, now, meta.CreatedAt, time.Second)
				require.NotNil(t, meta.ExpiresAt)
			},
		},
		{
			name: "GetMetadata for missing key",
			run: func(t *testing.T, s *Store) {
				_, err := s.GetMetadata(context.Background(), "prod/missing/sha.tfplan")
				assert.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
			},
		},
		{
			name: "Delete",
			run: func(t *testing.T, s *Store) {
				require.NoError(t, s.Delete(context.Background(), key))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fastListRetries(t)
			srv := newRecordingServer(t, nameFilteringHandler(t, artifacts))

			tt.run(t, srv.store())

			// The unfiltered listing 500s in this fixture, so success proves it was never used, and
			// every list request must carry the name filter.
			srv.mu.Lock()
			defer srv.mu.Unlock()
			for _, req := range srv.requests {
				if strings.HasPrefix(req, "GET "+testRepoArtifactsPath) {
					assert.Contains(t, req, "name=", "list request must be name-filtered: %s", req)
				}
			}
		})
	}
}

func TestStore_Delete_DeletesTheNamedArtifact(t *testing.T) {
	fastListRetries(t)
	artifacts := []testQueryArtifact{
		{ID: 55, Name: "planfile-prod--mycomponent--sha1.tfplan", Created: time.Now()},
	}
	srv := newRecordingServer(t, nameFilteringHandler(t, artifacts))

	require.NoError(t, srv.store().Delete(context.Background(), "prod/mycomponent/sha1.tfplan"))

	assert.Equal(t, 1, srv.count(http.MethodDelete, testRepoArtifactsPath+"/55"))
}

func TestStore_findArtifact_KeyBeyondFirstHundredRepoArtifacts(t *testing.T) {
	fastListRetries(t)
	now := time.Now()
	target := testQueryArtifact{ID: 999, Name: "planfile-prod--old--sha.tfplan", Created: now.Add(-48 * time.Hour)}

	// Emulate a busy repo: the unfiltered first page holds 100 newer, unrelated artifacts and the
	// target only exists further down. A name filter must still find it.
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		if name := r.URL.Query().Get("name"); name != "" {
			var matches []testQueryArtifact
			if name == target.Name {
				matches = append(matches, target)
			}
			writeArtifactsJSON(t, w, matches)
			return
		}
		decoys := make([]testQueryArtifact, 0, githubPaginationLimit)
		for i := 0; i < githubPaginationLimit; i++ {
			decoys = append(decoys, testQueryArtifact{ID: int64(i + 1), Name: fmt.Sprintf("decoy-%d", i), Created: now})
		}
		writeArtifactsJSON(t, w, decoys)
	})
	s := srv.store()

	a, err := s.findArtifact(context.Background(), "prod/old/sha.tfplan")
	require.NoError(t, err)
	assert.Equal(t, int64(999), a.ID)

	ok, err := s.Exists(context.Background(), "prod/old/sha.tfplan")
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestStore_findArtifact_FirstMatchWinsAndGuardsNameEquality(t *testing.T) {
	fastListRetries(t)
	now := time.Now()

	// A misbehaving server returns a non-matching artifact first; the equality guard skips it and
	// the first exact match (newest-first order) wins.
	srv := newRecordingServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeArtifactsJSON(t, w, []testQueryArtifact{
			{ID: 1, Name: "planfile-prod--x--sha.tfplan.bak", Created: now},
			{ID: 2, Name: "planfile-prod--x--sha.tfplan", Created: now},
			{ID: 3, Name: "planfile-prod--x--sha.tfplan", Created: now.Add(-time.Hour)},
		})
	})

	a, err := srv.store().findArtifact(context.Background(), "prod/x/sha.tfplan")
	require.NoError(t, err)
	assert.Equal(t, int64(2), a.ID)
}

func TestStore_ExactKeyOperations_NameIsQueryEscaped(t *testing.T) {
	fastListRetries(t)
	const key = "prod/comp/a b&x=1+2#frag.tfplan"
	const wantName = "planfile-prod--comp--a b&x=1+2#frag.tfplan"

	var mu sync.Mutex
	var gotNames []string
	var rawQueries []string
	srv := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotNames = append(gotNames, r.URL.Query().Get("name"))
		rawQueries = append(rawQueries, r.URL.RawQuery)
		mu.Unlock()
		writeArtifactsJSON(t, w, []testQueryArtifact{{ID: 1, Name: wantName, Created: time.Now()}})
	})

	ok, err := srv.store().Exists(context.Background(), key)
	require.NoError(t, err)
	assert.True(t, ok)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, gotNames, 1)
	assert.Equal(t, wantName, gotNames[0], "the server must decode the exact artifact name")
	assert.NotContains(t, rawQueries[0], "&x=1", "reserved characters must be percent-encoded")
	assert.NotContains(t, rawQueries[0], "#", "reserved characters must be percent-encoded")
	assert.Contains(t, rawQueries[0], "per_page=100")
	assert.Contains(t, rawQueries[0], "page=1")
}

// listFallbackServer emulates a repo whose unfiltered listing fails while the run-scoped listing works.
type listFallbackServer struct {
	primaryStatus int
	primaryPages  [][]testQueryArtifact // Served when primaryStatus is 200 for page N (1-based).
	failPrimaryAt int                   // When >0, primary page N (and later) fails with primaryFailStatus.
	primaryFail   int
	runStatus     int
	runPages      [][]testQueryArtifact
}

func (f *listFallbackServer) handler(t *testing.T) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case testRepoArtifactsPath:
			f.servePrimary(t, w, r)
		case testRunArtifactsPath:
			f.serveRun(t, w, r)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

func (f *listFallbackServer) servePrimary(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	page := queryPage(r)
	if f.primaryStatus != 0 && f.primaryStatus != http.StatusOK {
		w.WriteHeader(f.primaryStatus)
		return
	}
	if f.failPrimaryAt > 0 && page >= f.failPrimaryAt {
		w.WriteHeader(f.primaryFail)
		return
	}
	servePages(t, w, r, testRepoArtifactsPath, f.primaryPages)
}

func (f *listFallbackServer) serveRun(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	if f.runStatus != 0 && f.runStatus != http.StatusOK {
		w.WriteHeader(f.runStatus)
		return
	}
	servePages(t, w, r, testRunArtifactsPath, f.runPages)
}

func queryPage(r *http.Request) int {
	page, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || page < 1 {
		return 1
	}
	return page
}

func servePages(t *testing.T, w http.ResponseWriter, r *http.Request, path string, pages [][]testQueryArtifact) {
	t.Helper()

	page := queryPage(r)
	if page > len(pages) {
		writeArtifactsJSON(t, w, nil)
		return
	}
	if page < len(pages) {
		w.Header().Set("Link", fmt.Sprintf(`<http://%s%s?per_page=100&page=%d>; rel="next"`, r.Host, path, page+1))
	}
	writeArtifactsJSON(t, w, pages[page-1])
}

func TestStore_List_FallsBackToRunScopedListing(t *testing.T) {
	fastListRetries(t)
	t.Setenv("GITHUB_RUN_ID", testRunID)
	warnings := captureWarnings(t)

	now := time.Now()
	fake := &listFallbackServer{
		primaryStatus: http.StatusInternalServerError,
		runPages: [][]testQueryArtifact{
			{
				{ID: 1, Name: "planfile-prod--mycomponent--old.tfplan", Size: 100, Created: now.Add(-2 * time.Hour)},
				{ID: 2, Name: "unrelated-artifact", Size: 5, Created: now},
			},
			{
				{ID: 3, Name: "planfile-prod--mycomponent--new.tfplan", Size: 300, Created: now},
				{ID: 4, Name: "planfile-dev--mycomponent--dev.tfplan", Size: 400, Created: now},
			},
		},
	}
	srv := newRecordingServer(t, fake.handler(t))

	files, err := srv.store().List(context.Background(), artifact.Query{Stacks: []string{"prod"}, Components: []string{"mycomponent"}})
	require.NoError(t, err)

	require.Len(t, files, 2)
	assert.Equal(t, "prod/mycomponent/new.tfplan", files[0].Name, "newest first")
	assert.Equal(t, int64(300), files[0].Size)
	assert.Equal(t, "prod/mycomponent/old.tfplan", files[1].Name)

	assert.Equal(t, listArtifactsMaxAttempts, srv.count(http.MethodGet, testRepoArtifactsPath), "retries are exhausted before falling back")
	assert.Equal(t, 2, srv.count(http.MethodGet, testRunArtifactsPath), "both fallback pages are fetched")
	assert.Equal(t, 1, strings.Count(warnings.String(), "limited to the current workflow run"), "exactly one warning is logged")
}

func TestStore_List_FallbackRestartsFromScratchWithoutDuplicates(t *testing.T) {
	fastListRetries(t)
	t.Setenv("GITHUB_RUN_ID", testRunID)
	captureWarnings(t)

	now := time.Now()
	shared := testQueryArtifact{ID: 1, Name: "planfile-prod--c--a.tfplan", Created: now}
	fake := &listFallbackServer{
		// Primary page 1 succeeds, then page 2 fails persistently.
		primaryPages:  [][]testQueryArtifact{{shared}, {{ID: 9, Name: "planfile-prod--c--never.tfplan", Created: now}}},
		failPrimaryAt: 2,
		primaryFail:   http.StatusBadGateway,
		runPages:      [][]testQueryArtifact{{shared, {ID: 2, Name: "planfile-prod--c--b.tfplan", Created: now.Add(-time.Hour)}}},
	}
	srv := newRecordingServer(t, fake.handler(t))

	files, err := srv.store().List(context.Background(), artifact.Query{All: true})
	require.NoError(t, err)

	require.Len(t, files, 2, "the shared artifact must not be listed twice")
	assert.Equal(t, "prod/c/a.tfplan", files[0].Name)
	assert.Equal(t, "prod/c/b.tfplan", files[1].Name)
}

func TestStore_List_NoFallback(t *testing.T) {
	tests := []struct {
		name          string
		runID         string
		primaryStatus int
		wantPrimary   int
	}{
		{name: "5xx without GITHUB_RUN_ID", runID: "", primaryStatus: http.StatusInternalServerError, wantPrimary: listArtifactsMaxAttempts},
		{name: "401 with GITHUB_RUN_ID", runID: testRunID, primaryStatus: http.StatusUnauthorized, wantPrimary: 1},
		{name: "403 with GITHUB_RUN_ID", runID: testRunID, primaryStatus: http.StatusForbidden, wantPrimary: 1},
		{name: "404 with GITHUB_RUN_ID", runID: testRunID, primaryStatus: http.StatusNotFound, wantPrimary: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fastListRetries(t)
			t.Setenv("GITHUB_RUN_ID", tt.runID)
			warnings := captureWarnings(t)

			fake := &listFallbackServer{
				primaryStatus: tt.primaryStatus,
				runPages:      [][]testQueryArtifact{{{ID: 1, Name: "planfile-prod--c--a.tfplan", Created: time.Now()}}},
			}
			srv := newRecordingServer(t, fake.handler(t))

			files, err := srv.store().List(context.Background(), artifact.Query{All: true})

			require.Error(t, err)
			assert.Nil(t, files)
			assert.ErrorIs(t, err, errUtils.ErrArtifactListFailed)
			assert.Contains(t, err.Error(), fmt.Sprintf("status %d", tt.primaryStatus))
			assert.Equal(t, tt.wantPrimary, srv.count(http.MethodGet, testRepoArtifactsPath))
			assert.Zero(t, srv.count(http.MethodGet, testRunArtifactsPath), "the run-scoped endpoint must not be called")
			assert.NotContains(t, warnings.String(), "workflow run")
		})
	}
}

func TestStore_List_NoFallbackOnTransportError(t *testing.T) {
	fastListRetries(t)
	t.Setenv("GITHUB_RUN_ID", testRunID)

	calls := 0
	s := &Store{
		httpClient: &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			calls++
			return nil, fmt.Errorf("dial failed")
		})},
		baseURL: "https://api.github.test",
		prefix:  "planfile",
		owner:   testQueryOwner,
		repo:    testQueryRepo,
	}

	_, err := s.List(context.Background(), artifact.Query{All: true})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "dial failed")
	assert.Equal(t, 1, calls, "a network error neither retries nor falls back")
}

func TestStore_List_FallbackEndpointAlsoFails(t *testing.T) {
	fastListRetries(t)
	t.Setenv("GITHUB_RUN_ID", testRunID)
	captureWarnings(t)

	fake := &listFallbackServer{
		primaryStatus: http.StatusInternalServerError,
		runStatus:     http.StatusServiceUnavailable,
	}
	srv := newRecordingServer(t, fake.handler(t))

	files, err := srv.store().List(context.Background(), artifact.Query{All: true})

	require.Error(t, err)
	assert.Nil(t, files)
	assert.ErrorIs(t, err, errUtils.ErrArtifactListFailed)
	assert.Contains(t, err.Error(), "status 503")
	assert.Equal(t, listArtifactsMaxAttempts, srv.count(http.MethodGet, testRunArtifactsPath), "the fallback reuses the same retries")
}

func TestStore_List_SuccessfulPrimaryNeverCallsFallback(t *testing.T) {
	fastListRetries(t)
	t.Setenv("GITHUB_RUN_ID", testRunID)
	warnings := captureWarnings(t)

	now := time.Now()
	fake := &listFallbackServer{
		primaryPages: [][]testQueryArtifact{{{ID: 1, Name: "planfile-prod--c--a.tfplan", Created: now}}},
		runPages:     [][]testQueryArtifact{{{ID: 2, Name: "planfile-prod--c--run-only.tfplan", Created: now}}},
	}
	srv := newRecordingServer(t, fake.handler(t))

	files, err := srv.store().List(context.Background(), artifact.Query{All: true})

	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, "prod/c/a.tfplan", files[0].Name)
	assert.Zero(t, srv.count(http.MethodGet, testRunArtifactsPath))
	assert.NotContains(t, warnings.String(), "workflow run")
}

func TestListArtifactsStatusError(t *testing.T) {
	tests := []struct {
		name    string
		err     *listArtifactsStatusError
		wantMsg string
	}{
		{
			name:    "with body",
			err:     &listArtifactsStatusError{StatusCode: http.StatusInternalServerError, Body: `{"message":"boom"}`},
			wantMsg: `failed to list artifacts: status 500: {"message":"boom"}`,
		},
		{
			name:    "empty body omits trailing separator",
			err:     &listArtifactsStatusError{StatusCode: http.StatusInternalServerError},
			wantMsg: "failed to list artifacts: status 500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantMsg, tt.err.Error())
			assert.ErrorIs(t, tt.err, errUtils.ErrArtifactListFailed)

			// Wrapping keeps both the sentinel and the status reachable.
			wrapped := fmt.Errorf("%w: outer: %w", errUtils.ErrArtifactDownloadFailed, tt.err)
			assert.ErrorIs(t, wrapped, errUtils.ErrArtifactListFailed)
			var got *listArtifactsStatusError
			require.True(t, errors.As(wrapped, &got))
			assert.Equal(t, tt.err.StatusCode, got.StatusCode)
		})
	}
}

func TestStore_listArtifacts_ReturnsTypedStatusError(t *testing.T) {
	fastListRetries(t)
	srv := newRecordingServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("rate limited"))
	})

	_, _, err := srv.store().listArtifacts(context.Background(), listArtifactsQuery{perPage: 10, page: 1})

	var statusErr *listArtifactsStatusError
	require.True(t, errors.As(err, &statusErr))
	assert.Equal(t, http.StatusForbidden, statusErr.StatusCode)
	assert.Equal(t, "rate limited", statusErr.Body)
	assert.ErrorIs(t, err, errUtils.ErrArtifactListFailed)
	assert.Equal(t, "failed to list artifacts: status 403: rate limited", err.Error())
}

func TestListArtifactsQuery_PathAndValues(t *testing.T) {
	tests := []struct {
		name       string
		query      listArtifactsQuery
		wantPath   string
		wantValues string
	}{
		{
			name:       "repo wide",
			query:      listArtifactsQuery{perPage: 100, page: 3},
			wantPath:   "/repos/o/r/actions/artifacts",
			wantValues: "page=3&per_page=100",
		},
		{
			name:       "name filter is escaped",
			query:      listArtifactsQuery{perPage: 100, page: 1, name: "a b&c"},
			wantPath:   "/repos/o/r/actions/artifacts",
			wantValues: "name=a+b%26c&page=1&per_page=100",
		},
		{
			name:       "run scoped",
			query:      listArtifactsQuery{perPage: 100, page: 2, runID: "77"},
			wantPath:   "/repos/o/r/actions/runs/77/artifacts",
			wantValues: "page=2&per_page=100",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantPath, tt.query.path("o", "r"))
			assert.Equal(t, tt.wantValues, tt.query.values().Encode())
		})
	}
}
