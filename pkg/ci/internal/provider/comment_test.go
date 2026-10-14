package provider

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestNormalizeBehavior(t *testing.T) {
	tests := []struct {
		name    string
		in      CommentBehavior
		want    CommentBehavior
		wantErr bool
	}{
		{name: "empty defaults to upsert", in: "", want: CommentBehaviorUpsert},
		{name: "create", in: CommentBehaviorCreate, want: CommentBehaviorCreate},
		{name: "update", in: CommentBehaviorUpdate, want: CommentBehaviorUpdate},
		{name: "upsert", in: CommentBehaviorUpsert, want: CommentBehaviorUpsert},
		{name: "typo fails fast", in: CommentBehavior("upsrt"), wantErr: true},
		{name: "case sensitive", in: CommentBehavior("Upsert"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeBehavior(tt.in)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, errUtils.ErrCICommentPostFailed)
				assert.Empty(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestValidatePostCommentOptions(t *testing.T) {
	const marker = "<!-- atmos:ci:plan:vpc:dev -->"

	tests := []struct {
		name    string
		opts    *PostCommentOptions
		wantErr bool
	}{
		{name: "nil opts", opts: nil, wantErr: true},
		{name: "missing owner", opts: &PostCommentOptions{Repo: "r", PRNumber: 1}, wantErr: true},
		{name: "missing repo", opts: &PostCommentOptions{Owner: "o", PRNumber: 1}, wantErr: true},
		{name: "zero PR number", opts: &PostCommentOptions{Owner: "o", Repo: "r"}, wantErr: true},
		{name: "negative PR number", opts: &PostCommentOptions{Owner: "o", Repo: "r", PRNumber: -1}, wantErr: true},
		{name: "marker missing from body", opts: &PostCommentOptions{Owner: "o", Repo: "r", PRNumber: 1, Marker: marker, Body: "no marker"}, wantErr: true},
		{name: "marker present in body", opts: &PostCommentOptions{Owner: "o", Repo: "r", PRNumber: 1, Marker: marker, Body: marker + "\nbody"}},
		{name: "empty marker skips body check", opts: &PostCommentOptions{Owner: "o", Repo: "r", PRNumber: 1, Body: "anything"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePostCommentOptions(tt.opts)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, errUtils.ErrCICommentPostFailed)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestNormalizeTarget(t *testing.T) {
	tests := []struct {
		name string
		in   CommentTarget
		want CommentTarget
	}{
		{name: "empty means auto", in: "", want: CommentTargetAuto},
		{name: "auto", in: CommentTargetAuto, want: CommentTargetAuto},
		{name: "pr", in: CommentTargetPR, want: CommentTargetPR},
		{name: "commit", in: CommentTargetCommit, want: CommentTargetCommit},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeTarget(tt.in)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}

	for _, bad := range []CommentTarget{"issue", "PR", "Commit"} {
		t.Run("invalid "+string(bad), func(t *testing.T) {
			got, err := NormalizeTarget(bad)
			require.ErrorIs(t, err, errUtils.ErrCICommentTargetInvalid)
			assert.Empty(t, got)
		})
	}
}

func TestValidatePostCommitCommentOptions(t *testing.T) {
	const marker = "<!-- atmos:ci:deploy -->"

	tests := []struct {
		name    string
		opts    *PostCommitCommentOptions
		wantErr bool
	}{
		{name: "nil opts", opts: nil, wantErr: true},
		{name: "missing owner", opts: &PostCommitCommentOptions{Repo: "r", SHA: "abc"}, wantErr: true},
		{name: "missing repo", opts: &PostCommitCommentOptions{Owner: "o", SHA: "abc"}, wantErr: true},
		{name: "missing sha", opts: &PostCommitCommentOptions{Owner: "o", Repo: "r"}, wantErr: true},
		{name: "marker missing from body", opts: &PostCommitCommentOptions{Owner: "o", Repo: "r", SHA: "abc", Marker: marker, Body: "no marker"}, wantErr: true},
		{name: "marker present in body", opts: &PostCommitCommentOptions{Owner: "o", Repo: "r", SHA: "abc", Marker: marker, Body: marker + "\nbody"}},
		{name: "empty marker skips body check", opts: &PostCommitCommentOptions{Owner: "o", Repo: "r", SHA: "abc", Body: "anything"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePostCommitCommentOptions(tt.opts)
			if tt.wantErr {
				require.ErrorIs(t, err, errUtils.ErrCICommentPostFailed)
				return
			}
			require.NoError(t, err)
		})
	}
}
