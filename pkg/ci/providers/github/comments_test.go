package github

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	cockroachdb "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/cloudposse/atmos/pkg/ci/providers/github/ghtest"
	atmosio "github.com/cloudposse/atmos/pkg/io"
)

const commentsPath = "/repos/owner/repo/issues/42/comments"

// allHintsJoin returns all hints attached to err joined with newlines so tests
// can assert hint presence via substring match.
func allHintsJoin(err error) string {
	return strings.Join(cockroachdb.GetAllHints(err), "\n")
}

// newTestProvider builds a *Provider whose client is created by the real
// NewClient() and pointed at the fake GitHub API through GITHUB_API_URL, so
// PostComment exercises the real HTTP path through go-github.
func newTestProvider(t *testing.T, s *ghtest.Server) *Provider {
	t.Helper()

	t.Setenv("ATMOS_CI_GITHUB_API_URL", "")
	t.Setenv("ATMOS_CI_GITHUB_TOKEN", "")
	t.Setenv("ATMOS_PRO_GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_API_URL", s.URL())
	t.Setenv("GITHUB_TOKEN", "github-token-for-tests")

	client, err := NewClient()
	require.NoError(t, err)

	return NewProviderWithClient(client)
}

// countRequests returns how many recorded requests used the given method against
// the PR 42 comments endpoint.
func countRequests(s *ghtest.Server, method string) int {
	return len(listRequests(s, method))
}

// listRequests returns the recorded comment-list requests for method, in order.
func listRequests(s *ghtest.Server, method string) []ghtest.RecordedRequest {
	var out []ghtest.RecordedRequest
	for _, r := range s.Requests() {
		if r.Method == method && r.Path == commentsPath {
			out = append(out, r)
		}
	}
	return out
}

func TestProvider_PostComment_UpsertCreatesWhenMarkerAbsent(t *testing.T) {
	// Two comments exist, neither containing the marker.
	s := ghtest.NewServer(t, ghtest.WithSeedComments(
		"owner", "repo", 42,
		ghtest.Comment{ID: 1, Body: "unrelated"},
		ghtest.Comment{ID: 2, Body: "also unrelated"},
	))
	p := newTestProvider(t, s)

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

	assert.Equal(t, 1, countRequests(s, http.MethodGet), "upsert must list existing comments")
	assert.Equal(t, 1, countRequests(s, http.MethodPost), "should create a new comment when marker is absent")
	assert.True(t, res.Created)

	writes := s.Comments()
	require.Len(t, writes, 1)
	assert.False(t, writes[0].Edited)
	assert.Equal(t, writes[0].ID, res.ID)
	assert.NotZero(t, res.ID)
	assert.NotEmpty(t, res.URL)
	assert.Contains(t, writes[0].Body, marker)
	assert.Contains(t, writes[0].Body, "plan body")
}

func TestProvider_PostComment_UpsertUpdatesExistingMatch(t *testing.T) {
	s := ghtest.NewServer(t, ghtest.WithSeedComments(
		"owner", "repo", 42,
		ghtest.Comment{ID: 1, Body: "old: <!-- atmos:ci:plan:vpc:dev --> stale"},
	))
	p := newTestProvider(t, s)

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

	assert.Equal(t, 0, countRequests(s, http.MethodPost), "upsert with existing match must not POST")
	assert.False(t, res.Created, "must be treated as update")
	assert.Equal(t, int64(1), res.ID)

	writes := s.Comments()
	require.Len(t, writes, 1)
	assert.True(t, writes[0].Edited)
	assert.Equal(t, int64(1), writes[0].ID)
	assert.Contains(t, writes[0].Body, "new body")

	// The edit replaces the seeded comment rather than adding one.
	current := s.CommentsFor("owner", "repo", 42)
	require.Len(t, current, 1)
	assert.Contains(t, current[0].Body, "new body")
}

func TestProvider_PostComment_CreateAlwaysPosts(t *testing.T) {
	// A matching marker already exists, yet create behavior must still post a new comment.
	marker := "<!-- atmos:ci:plan:vpc:dev -->"
	s := ghtest.NewServer(t, ghtest.WithSeedComments(
		"owner", "repo", 42,
		ghtest.Comment{ID: 1, Body: marker + " existing"},
	))
	p := newTestProvider(t, s)

	_, err := p.PostComment(context.Background(), &provider.PostCommentOptions{
		Owner:    "owner",
		Repo:     "repo",
		PRNumber: 42,
		Marker:   marker,
		Body:     marker + "\nbody",
		Behavior: provider.CommentBehaviorCreate,
	})
	require.NoError(t, err)
	assert.Equal(t, 0, countRequests(s, http.MethodGet), "create behavior must skip the list call")
	assert.Equal(t, 1, countRequests(s, http.MethodPost))
	assert.Len(t, s.CommentsFor("owner", "repo", 42), 2)
}

func TestProvider_PostComment_MasksRegisteredSecrets(t *testing.T) {
	atmosio.Reset()
	t.Cleanup(atmosio.Reset)

	const secret = "comment-secret-ABCD1234"
	atmosio.RegisterSecret(secret)

	s := ghtest.NewServer(t)
	p := newTestProvider(t, s)

	marker := "<!-- atmos:ci:plan:service:test -->"
	_, err := p.PostComment(context.Background(), &provider.PostCommentOptions{
		Owner:    "owner",
		Repo:     "repo",
		PRNumber: 42,
		Marker:   marker,
		Body:     marker + "\ncredential: " + secret,
		Behavior: provider.CommentBehaviorCreate,
	})
	require.NoError(t, err)

	writes := s.Comments()
	require.Len(t, writes, 1)
	assert.Contains(t, writes[0].Body, marker)
	assert.NotContains(t, writes[0].Body, secret)
	assert.Contains(t, writes[0].Body, atmosio.MaskReplacement)
}

func TestProvider_PostComment_UpdateReturnsNotFoundWhenAbsent(t *testing.T) {
	s := ghtest.NewServer(t)
	p := newTestProvider(t, s)

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
	assert.Empty(t, s.Comments(), "update with no existing comment must not write")
}

func TestProvider_PostComment_ValidatesRequiredFields(t *testing.T) {
	s := ghtest.NewServer(t)
	p := newTestProvider(t, s)

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

	// No HTTP traffic should happen for invalid input.
	assert.Empty(t, s.Requests())
}

// TestProvider_PostComment_RequiresMarkerInBody verifies the invariant that
// Body must contain Marker. Without this check, an upsert that writes a body
// missing the marker would cause subsequent runs to fail to match the existing
// comment and create duplicates.
func TestProvider_PostComment_RequiresMarkerInBody(t *testing.T) {
	s := ghtest.NewServer(t)
	p := newTestProvider(t, s)

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
	assert.Empty(t, s.Requests())
}

// TestProvider_PostComment_EmptyMarkerSkipsInvariantCheck — when Marker is
// empty the body-invariant check does not apply; the call should still reach
// the HTTP layer (and in this test, succeed at create via an empty list).
func TestProvider_PostComment_EmptyMarkerSkipsInvariantCheck(t *testing.T) {
	s := ghtest.NewServer(t)
	p := newTestProvider(t, s)

	_, err := p.PostComment(context.Background(), &provider.PostCommentOptions{
		Owner:    "owner",
		Repo:     "repo",
		PRNumber: 42,
		Marker:   "",
		Body:     "body without marker",
		Behavior: provider.CommentBehaviorUpsert,
	})
	require.NoError(t, err)
	require.Len(t, s.Comments(), 1)
	assert.Equal(t, "body without marker", s.Comments()[0].Body)
}

// TestProvider_PostComment_RejectsUnknownBehavior verifies that
// misconfigured ci.comments.behavior values fail fast rather than silently
// defaulting to upsert.
func TestProvider_PostComment_RejectsUnknownBehavior(t *testing.T) {
	s := ghtest.NewServer(t)
	p := newTestProvider(t, s)

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
	assert.Empty(t, s.Requests())
}

func TestProvider_PostComment_403HintedForMissingPermission(t *testing.T) {
	// Even the list call surfaces a 403 when permissions are missing.
	s := ghtest.NewServer(t, ghtest.WithFailure(http.MethodGet, commentsPath, http.StatusForbidden, "Resource not accessible by integration"))
	p := newTestProvider(t, s)

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
	s := ghtest.NewServer(t, ghtest.WithFailure(http.MethodGet, commentsPath, http.StatusNotFound, "Not Found"))
	p := newTestProvider(t, s)

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

func TestProvider_PostComment_CreateFailureWrapsPostFailed(t *testing.T) {
	s := ghtest.NewServer(t, ghtest.WithFailure(http.MethodPost, commentsPath, http.StatusForbidden, "Resource not accessible by integration"))
	p := newTestProvider(t, s)

	_, err := p.PostComment(context.Background(), &provider.PostCommentOptions{
		Owner:    "owner",
		Repo:     "repo",
		PRNumber: 42,
		Marker:   "m",
		Body:     "m body",
		Behavior: provider.CommentBehaviorUpsert,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrCICommentPostFailed)
	assert.NotErrorIs(t, err, errUtils.ErrCICommentListFailed, "the list succeeded; only the create failed")
}

func TestProvider_PostComment_DefaultBehaviorIsUpsert(t *testing.T) {
	s := ghtest.NewServer(t)
	p := newTestProvider(t, s)

	_, err := p.PostComment(context.Background(), &provider.PostCommentOptions{
		Owner:    "owner",
		Repo:     "repo",
		PRNumber: 42,
		Marker:   "m",
		Body:     "m body",
		// Behavior left empty on purpose.
	})
	require.NoError(t, err)
	assert.Equal(t, 1, countRequests(s, http.MethodGet))
	assert.Equal(t, 1, countRequests(s, http.MethodPost))
}

func TestProvider_PostComment_PaginatesListSearch(t *testing.T) {
	// Marker sits on the last page — verify the walk follows `Link: <...>; rel="next"`.
	// The provider requests issueCommentsPerPage per page, so seed more than one page.
	marker := "<!-- atmos:ci:plan:vpc:dev -->"
	seeded := make([]ghtest.Comment, 0, issueCommentsPerPage+1)
	for i := 1; i <= issueCommentsPerPage; i++ {
		seeded = append(seeded, ghtest.Comment{ID: int64(i), Body: fmt.Sprintf("no match %d", i)})
	}
	const matchID = int64(77777)
	seeded = append(seeded, ghtest.Comment{ID: matchID, Body: "found: " + marker + " here"})

	s := ghtest.NewServer(t, ghtest.WithSeedComments("owner", "repo", 42, seeded...))
	p := newTestProvider(t, s)

	_, err := p.PostComment(context.Background(), &provider.PostCommentOptions{
		Owner:    "owner",
		Repo:     "repo",
		PRNumber: 42,
		Marker:   marker,
		Body:     marker + "\nupdated",
		Behavior: provider.CommentBehaviorUpsert,
	})
	require.NoError(t, err)

	gets := listRequests(s, http.MethodGet)
	require.Lenf(t, gets, 2, "must walk to the second page; recorded requests: %+v", s.Requests())
	assert.Contains(t, gets[1].RawQuery, "page=2", "second request must ask for page 2; recorded requests: %+v", s.Requests())
	writes := s.Comments()
	require.Len(t, writes, 1)
	assert.True(t, writes[0].Edited)
	assert.Equal(t, matchID, writes[0].ID, "should paginate and edit the page-2 match")
}
