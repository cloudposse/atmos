package github

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/cloudposse/atmos/pkg/ci/providers/github/ghtest"
	atmosio "github.com/cloudposse/atmos/pkg/io"
)

// TestPostComment_LimitsOversizedBodies preserves upsert identity and reports
// truncation instead of submitting a body rejected by GitHub (issue #3088).
func TestPostComment_LimitsOversizedBodies(t *testing.T) {
	for _, body := range []string{strings.Repeat("x", 70000), strings.Repeat("界", 24000)} {
		t.Run(body[:1], func(t *testing.T) {
			s := ghtest.NewServer(t)
			p := newTestProvider(t, s)
			marker := "<!-- atmos:ci:aggregate:plan:all -->"
			opts := &provider.PostCommentOptions{
				Owner: "owner", Repo: "repo", PRNumber: 42,
				Marker: marker, Body: body + "\n" + marker,
				Behavior: provider.CommentBehaviorUpsert,
			}
			for range 2 {
				_, err := p.PostComment(context.Background(), opts)
				require.NoError(t, err)
			}
			comments := s.CommentsFor("owner", "repo", 42)
			require.Len(t, comments, 1, "truncation must preserve the marker for updates")
			assert.LessOrEqual(t, len(comments[0].Body), 65536)
			assert.True(t, utf8.ValidString(comments[0].Body))
			assert.Contains(t, comments[0].Body, marker)
			assert.Contains(t, comments[0].Body, "truncated")
			assert.Equal(t, body+"\n"+marker, opts.Body, "do not mutate the caller's full summary")
		})
	}
}

func TestPostComment_PreservesBodyAtLimit(t *testing.T) {
	s := ghtest.NewServer(t)
	p := newTestProvider(t, s)
	body := strings.Repeat("x", 65536)
	_, err := p.PostComment(context.Background(), &provider.PostCommentOptions{
		Owner: "owner", Repo: "repo", PRNumber: 42,
		Body: body, Behavior: provider.CommentBehaviorCreate,
	})
	require.NoError(t, err)
	comments := s.CommentsFor("owner", "repo", 42)
	require.Len(t, comments, 1)
	assert.Equal(t, body, comments[0].Body)
}

func TestPostComment_TruncationDoesNotRestoreMaskedMarker(t *testing.T) {
	atmosio.Reset()
	t.Cleanup(atmosio.Reset)
	const secret = "test-marker"
	atmosio.RegisterSecret(secret)
	s := ghtest.NewServer(t)
	p := newTestProvider(t, s)
	marker := "<!-- " + secret + " -->"
	_, err := p.PostComment(context.Background(), &provider.PostCommentOptions{
		Owner: "owner", Repo: "repo", PRNumber: 42,
		Marker: marker, Body: strings.Repeat("x", 70000) + marker,
		Behavior: provider.CommentBehaviorCreate,
	})
	require.NoError(t, err)
	comments := s.CommentsFor("owner", "repo", 42)
	require.Len(t, comments, 1)
	assert.False(t, strings.Contains(comments[0].Body, secret))
}

func TestPostComment_RejectsOversizedMarker(t *testing.T) {
	s := ghtest.NewServer(t)
	p := newTestProvider(t, s)
	marker := "<!-- " + strings.Repeat("x", 70000) + " -->"
	_, err := p.PostComment(context.Background(), &provider.PostCommentOptions{
		Owner: "owner", Repo: "repo", PRNumber: 42,
		Marker: marker, Body: marker, Behavior: provider.CommentBehaviorCreate,
	})
	require.ErrorIs(t, err, errUtils.ErrCICommentPostFailed)
	assert.Empty(t, s.CommentsFor("owner", "repo", 42))
}
