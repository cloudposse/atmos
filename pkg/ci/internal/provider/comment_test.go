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
