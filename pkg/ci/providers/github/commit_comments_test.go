package github

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/cloudposse/atmos/pkg/ci/providers/github/ghtest"
	atmosio "github.com/cloudposse/atmos/pkg/io"
)

// Compile-time assertion that the GitHub provider can comment on commits.
var _ provider.CommitCommenter = (*Provider)(nil)

const (
	commitSHA    = "0123456789abcdef0123456789abcdef01234567"
	commitMarker = "<!-- atmos:ci:deploy -->"
)

func commitOpts(behavior provider.CommentBehavior, body string) *provider.PostCommitCommentOptions {
	return &provider.PostCommitCommentOptions{
		Owner: "owner", Repo: "repo", SHA: commitSHA,
		Marker: commitMarker, Body: commitMarker + "\n" + body, Behavior: behavior,
	}
}

func TestProvider_PostCommitComment_UpsertCreatesThenUpdates(t *testing.T) {
	s := ghtest.NewServer(t)
	p := newTestProvider(t, s)

	first, err := p.PostCommitComment(context.Background(), commitOpts(provider.CommentBehaviorUpsert, "first"))
	require.NoError(t, err)
	assert.True(t, first.Created)
	assert.Contains(t, first.URL, "commitcomment-")

	second, err := p.PostCommitComment(context.Background(), commitOpts(provider.CommentBehaviorUpsert, "second"))
	require.NoError(t, err)
	assert.False(t, second.Created)
	assert.Equal(t, first.ID, second.ID)

	current := s.CommitCommentsFor("owner", "repo", commitSHA)
	require.Len(t, current, 1)
	assert.Contains(t, current[0].Body, "second")
	assert.NotContains(t, current[0].Body, "first")
	assert.Empty(t, s.CommentsFor("owner", "repo", 0))
}

func TestProvider_PostCommitComment_Behaviors(t *testing.T) {
	t.Run("create always creates", func(t *testing.T) {
		s := ghtest.NewServer(t)
		p := newTestProvider(t, s)
		for range 2 {
			res, err := p.PostCommitComment(context.Background(), commitOpts(provider.CommentBehaviorCreate, "body"))
			require.NoError(t, err)
			assert.True(t, res.Created)
		}
		assert.Len(t, s.CommitCommentsFor("owner", "repo", commitSHA), 2)
	})

	t.Run("update without an existing comment is not found", func(t *testing.T) {
		s := ghtest.NewServer(t)
		p := newTestProvider(t, s)
		res, err := p.PostCommitComment(context.Background(), commitOpts(provider.CommentBehaviorUpdate, "body"))
		require.ErrorIs(t, err, errUtils.ErrCICommentNotFound)
		assert.Nil(t, res)
		assert.Empty(t, s.Comments())
	})

	t.Run("update edits a seeded comment", func(t *testing.T) {
		s := ghtest.NewServer(t, ghtest.WithSeedCommitComments("owner", "repo", commitSHA,
			ghtest.Comment{ID: 7, Body: commitMarker + "\nold"}))
		p := newTestProvider(t, s)
		res, err := p.PostCommitComment(context.Background(), commitOpts(provider.CommentBehaviorUpdate, "new"))
		require.NoError(t, err)
		assert.False(t, res.Created)
		assert.EqualValues(t, 7, res.ID)
	})
}

func TestProvider_PostCommitComment_Errors(t *testing.T) {
	t.Run("invalid options", func(t *testing.T) {
		s := ghtest.NewServer(t)
		p := newTestProvider(t, s)
		_, err := p.PostCommitComment(context.Background(), &provider.PostCommitCommentOptions{Owner: "owner", Repo: "repo"})
		require.ErrorIs(t, err, errUtils.ErrCICommentPostFailed)
		assert.Empty(t, s.Requests())
	})

	t.Run("create failure carries a permission hint", func(t *testing.T) {
		s := ghtest.NewServer(t, ghtest.WithFailure(http.MethodPost, "/repos/owner/repo/commits/", http.StatusForbidden, "denied"))
		p := newTestProvider(t, s)
		_, err := p.PostCommitComment(context.Background(), commitOpts(provider.CommentBehaviorCreate, "body"))
		require.ErrorIs(t, err, errUtils.ErrCICommentPostFailed)
		assert.Contains(t, allHintsJoin(err), "contents: write")
	})

	t.Run("list failure", func(t *testing.T) {
		s := ghtest.NewServer(t, ghtest.WithFailure(http.MethodGet, "/repos/owner/repo/commits/", http.StatusInternalServerError, "boom"))
		p := newTestProvider(t, s)
		_, err := p.PostCommitComment(context.Background(), commitOpts(provider.CommentBehaviorUpsert, "body"))
		require.ErrorIs(t, err, errUtils.ErrCICommentListFailed)
	})

	t.Run("edit failure", func(t *testing.T) {
		s := ghtest.NewServer(t,
			ghtest.WithSeedCommitComments("owner", "repo", commitSHA, ghtest.Comment{ID: 7, Body: commitMarker}),
			ghtest.WithFailure(http.MethodPatch, "/repos/owner/repo/comments/", http.StatusInternalServerError, "boom"))
		p := newTestProvider(t, s)
		_, err := p.PostCommitComment(context.Background(), commitOpts(provider.CommentBehaviorUpsert, "body"))
		require.ErrorIs(t, err, errUtils.ErrCICommentUpdateFailed)
	})

	t.Run("client not configured", func(t *testing.T) {
		for _, k := range []string{"ATMOS_CI_GITHUB_TOKEN", "ATMOS_PRO_GITHUB_TOKEN", "GITHUB_TOKEN", "GH_TOKEN"} {
			t.Setenv(k, "")
		}
		_, err := NewProvider().PostCommitComment(context.Background(), commitOpts(provider.CommentBehaviorUpsert, "body"))
		require.ErrorIs(t, err, errUtils.ErrCICommentPostFailed)
		require.ErrorIs(t, err, errUtils.ErrGitHubTokenNotFound)
	})
}

func TestProvider_PostCommitComment_MasksSecrets(t *testing.T) {
	// MaskPublishedContent runs over the body before it is sent, like pull request comments.
	const secret = "commit-comment-secret-7Qx9"
	atmosio.RegisterSecret(secret)
	t.Cleanup(atmosio.Reset)

	s := ghtest.NewServer(t)
	p := newTestProvider(t, s)
	_, err := p.PostCommitComment(context.Background(), commitOpts(provider.CommentBehaviorCreate, "token "+secret))
	require.NoError(t, err)
	require.Len(t, s.Comments(), 1)
	assert.NotContains(t, s.Comments()[0].Body, secret)
	assert.Contains(t, s.Comments()[0].Body, commitMarker)
}
