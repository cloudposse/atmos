package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	cockroachdb "github.com/cockroachdb/errors"
	"github.com/google/go-github/v59/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
)

// allHintsJoin returns all hints attached to err joined with newlines so tests
// can assert hint presence via substring match.
func allHintsJoin(err error) string {
	return strings.Join(cockroachdb.GetAllHints(err), "\n")
}

// newTestProvider wires an httptest.Server to a *Provider so PostComment
// exercises the real HTTP path through go-github.
func newTestProvider(t *testing.T, handler http.Handler) *Provider {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	serverURL, err := url.Parse(server.URL + "/")
	require.NoError(t, err)
	ghClient := github.NewClient(nil)
	ghClient.BaseURL = serverURL

	return NewProviderWithClient(&Client{client: ghClient})
}

func TestProvider_PostComment_UpsertCreatesWhenMarkerAbsent(t *testing.T) {
	var listCalled, createCalled bool
	var createdBody string

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/owner/repo/issues/42/comments", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			listCalled = true
			// Return two comments, neither containing the marker.
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"id": 1, "body": "unrelated"},
				{"id": 2, "body": "also unrelated"},
			})
		case http.MethodPost:
			createCalled = true
			var body struct {
				Body string `json:"body"`
			}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			createdBody = body.Body
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       123,
				"html_url": "https://github.com/owner/repo/pull/42#issuecomment-123",
				"body":     body.Body,
			})
		default:
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
		}
	})

	p := newTestProvider(t, mux)

	marker := "<!-- atmos:ci:plan:vpc:dev -->"
	res, err := p.PostComment(context.Background(), &provider.PostCommentOptions{
		Owner:    "owner",
		Repo:     "repo",
		PRNumber: 42,
		Marker:   marker,
		Body:     marker + "\nplan body",
		Behavior: provider.CommentBehaviorUpsert,
	})
	require.NoError(t, err)
	require.NotNil(t, res)

	assert.True(t, listCalled, "upsert must list existing comments")
	assert.True(t, createCalled, "should create a new comment when marker is absent")
	assert.True(t, res.Created)
	assert.Equal(t, int64(123), res.ID)
	assert.Contains(t, createdBody, marker)
	assert.Contains(t, createdBody, "plan body")
}

func TestProvider_PostComment_UpsertUpdatesExistingMatch(t *testing.T) {
	var editedID int64
	var editedBody string

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/owner/repo/issues/42/comments", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method, "upsert with existing match must not POST")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": 1, "body": "old: <!-- atmos:ci:plan:vpc:dev --> stale"},
		})
	})
	mux.HandleFunc("/repos/owner/repo/issues/comments/1", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPatch, r.Method)
		var body struct {
			Body string `json:"body"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		editedID = 1
		editedBody = body.Body
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       1,
			"html_url": "https://github.com/owner/repo/pull/42#issuecomment-1",
			"body":     body.Body,
		})
	})

	p := newTestProvider(t, mux)

	marker := "<!-- atmos:ci:plan:vpc:dev -->"
	res, err := p.PostComment(context.Background(), &provider.PostCommentOptions{
		Owner:    "owner",
		Repo:     "repo",
		PRNumber: 42,
		Marker:   marker,
		Body:     marker + "\nnew body",
		Behavior: provider.CommentBehaviorUpsert,
	})
	require.NoError(t, err)
	require.NotNil(t, res)

	assert.False(t, res.Created, "must be treated as update")
	assert.Equal(t, int64(1), res.ID)
	assert.Equal(t, int64(1), editedID)
	assert.Contains(t, editedBody, "new body")
}

func TestProvider_PostComment_CreateAlwaysPosts(t *testing.T) {
	var listCalled, createCalled bool

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/owner/repo/issues/42/comments", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			listCalled = true
		case http.MethodPost:
			createCalled = true
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 999, "body": "ok"})
		}
	})

	p := newTestProvider(t, mux)

	marker := "<!-- atmos:ci:plan:vpc:dev -->"
	_, err := p.PostComment(context.Background(), &provider.PostCommentOptions{
		Owner:    "owner",
		Repo:     "repo",
		PRNumber: 42,
		Marker:   marker,
		Body:     marker + "\nbody",
		Behavior: provider.CommentBehaviorCreate,
	})
	require.NoError(t, err)
	assert.False(t, listCalled, "create behavior must skip the list call")
	assert.True(t, createCalled)
}

func TestProvider_PostComment_UpdateReturnsNotFoundWhenAbsent(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/owner/repo/issues/42/comments", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{})
	})

	p := newTestProvider(t, mux)

	marker := "<!-- atmos:ci:plan:vpc:dev -->"
	_, err := p.PostComment(context.Background(), &provider.PostCommentOptions{
		Owner:    "owner",
		Repo:     "repo",
		PRNumber: 42,
		Marker:   marker,
		Body:     marker + "\nbody",
		Behavior: provider.CommentBehaviorUpdate,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrCICommentNotFound)
}

func TestProvider_PostComment_ValidatesRequiredFields(t *testing.T) {
	// No HTTP traffic should happen — handler would be called zero times.
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected HTTP call: %s %s", r.Method, r.URL.Path)
	})
	p := newTestProvider(t, mux)

	cases := []struct {
		name string
		opts *provider.PostCommentOptions
	}{
		{"nil opts", nil},
		{"missing owner", &provider.PostCommentOptions{Repo: "r", PRNumber: 1}},
		{"missing repo", &provider.PostCommentOptions{Owner: "o", PRNumber: 1}},
		{"zero PR number", &provider.PostCommentOptions{Owner: "o", Repo: "r", PRNumber: 0}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := p.PostComment(context.Background(), tc.opts)
			require.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrCICommentPostFailed)
		})
	}
}

// TestProvider_PostComment_RequiresMarkerInBody verifies the invariant that
// Body must contain Marker. Without this check, an upsert that writes a body
// missing the marker would cause subsequent runs to fail to match the existing
// comment and create duplicates.
func TestProvider_PostComment_RequiresMarkerInBody(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected HTTP call: %s %s", r.Method, r.URL.Path)
	})
	p := newTestProvider(t, mux)

	_, err := p.PostComment(context.Background(), &provider.PostCommentOptions{
		Owner:    "owner",
		Repo:     "repo",
		PRNumber: 42,
		Marker:   "<!-- atmos:ci:plan:vpc:dev -->",
		Body:     "no marker here",
		Behavior: provider.CommentBehaviorUpsert,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrCICommentPostFailed)
}

// TestProvider_PostComment_EmptyMarkerSkipsInvariantCheck — when Marker is
// empty the body-invariant check does not apply; the call should still reach
// the HTTP layer (and in this test, succeed at create via an empty list).
func TestProvider_PostComment_EmptyMarkerSkipsInvariantCheck(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/owner/repo/issues/42/comments", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		case http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 5, "body": "ok"})
		}
	})
	p := newTestProvider(t, mux)

	_, err := p.PostComment(context.Background(), &provider.PostCommentOptions{
		Owner:    "owner",
		Repo:     "repo",
		PRNumber: 42,
		Marker:   "",
		Body:     "body without marker",
		Behavior: provider.CommentBehaviorUpsert,
	})
	require.NoError(t, err)
}

// TestProvider_PostComment_RejectsUnknownBehavior verifies that
// misconfigured ci.comments.behavior values fail fast rather than silently
// defaulting to upsert.
func TestProvider_PostComment_RejectsUnknownBehavior(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected HTTP call: %s %s", r.Method, r.URL.Path)
	})
	p := newTestProvider(t, mux)

	_, err := p.PostComment(context.Background(), &provider.PostCommentOptions{
		Owner:    "owner",
		Repo:     "repo",
		PRNumber: 42,
		Marker:   "m",
		Body:     "m body",
		Behavior: provider.CommentBehavior("upsrt"), // typo.
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrCICommentPostFailed)
}

func TestProvider_PostComment_403HintedForMissingPermission(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/owner/repo/issues/42/comments", func(w http.ResponseWriter, r *http.Request) {
		// Even the list call surfaces a 403 when permissions are missing.
		http.Error(w, `{"message":"Resource not accessible by integration"}`, http.StatusForbidden)
	})
	p := newTestProvider(t, mux)

	_, err := p.PostComment(context.Background(), &provider.PostCommentOptions{
		Owner:    "owner",
		Repo:     "repo",
		PRNumber: 42,
		Marker:   "m",
		Body:     "m body",
		Behavior: provider.CommentBehaviorUpsert,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrCICommentListFailed)
	hints := allHintsJoin(err)
	assert.Contains(t, hints, "pull-requests: write", "403 must hint at missing permission")
}

func TestProvider_PostComment_404HintedOnNotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/owner/repo/issues/42/comments", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
	})
	p := newTestProvider(t, mux)

	_, err := p.PostComment(context.Background(), &provider.PostCommentOptions{
		Owner:    "owner",
		Repo:     "repo",
		PRNumber: 42,
		Marker:   "m",
		Body:     "m body",
		Behavior: provider.CommentBehaviorUpsert,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrCICommentListFailed)
	assert.Contains(t, allHintsJoin(err), "pull-requests: write")
}

func TestProvider_PostComment_DefaultBehaviorIsUpsert(t *testing.T) {
	var listCalled, createCalled bool
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/owner/repo/issues/42/comments", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			listCalled = true
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		case http.MethodPost:
			createCalled = true
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 7, "body": "ok"})
		}
	})
	p := newTestProvider(t, mux)

	_, err := p.PostComment(context.Background(), &provider.PostCommentOptions{
		Owner:    "owner",
		Repo:     "repo",
		PRNumber: 42,
		Marker:   "m",
		Body:     "m body",
		// Behavior left empty on purpose.
	})
	require.NoError(t, err)
	assert.True(t, listCalled)
	assert.True(t, createCalled)
}

func TestProvider_PostComment_PaginatesListSearch(t *testing.T) {
	// Marker sits on page 2 — verify the walk follows `Link: <...>; rel="next"`.
	var editedID int64

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/owner/repo/issues/42/comments", func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		w.Header().Set("Content-Type", "application/json")
		switch page {
		case "", "1":
			w.Header().Set("Link", `</repos/owner/repo/issues/42/comments?page=2>; rel="next"`)
			_ = json.NewEncoder(w).Encode([]map[string]any{{"id": 1, "body": "no match"}})
		case "2":
			_ = json.NewEncoder(w).Encode([]map[string]any{{"id": 77, "body": "found: <!-- atmos:ci:plan:vpc:dev --> here"}})
		default:
			http.Error(w, "unexpected page", http.StatusBadRequest)
		}
	})
	mux.HandleFunc("/repos/owner/repo/issues/comments/77", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPatch, r.Method)
		editedID = 77
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 77, "body": "updated"})
	})

	p := newTestProvider(t, mux)
	marker := "<!-- atmos:ci:plan:vpc:dev -->"
	_, err := p.PostComment(context.Background(), &provider.PostCommentOptions{
		Owner:    "owner",
		Repo:     "repo",
		PRNumber: 42,
		Marker:   marker,
		Body:     marker + "\nupdated",
		Behavior: provider.CommentBehaviorUpsert,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(77), editedID, "should paginate and edit the page-2 match")
}
