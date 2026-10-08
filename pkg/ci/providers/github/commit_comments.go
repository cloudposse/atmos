package github

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/google/go-github/v59/github"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Compile-time assertion that Provider can comment on commits.
var _ provider.CommitCommenter = (*Provider)(nil)

// PostCommitComment creates or upserts a comment on a commit using the GitHub commit comments API
// (POST /repos/{owner}/{repo}/commits/{sha}/comments). It is how a run with no pull request, such as
// a push, reports where a pull request comment would otherwise land. Behavior semantics match
// PostComment: create always posts, update requires a marker match, and upsert edits a match or
// creates one.
func (p *Provider) PostCommitComment(ctx context.Context, opts *provider.PostCommitCommentOptions) (*provider.Comment, error) {
	defer perf.Track(nil, "github.Provider.PostCommitComment")()

	if err := p.ensureClient(); err != nil {
		return nil, errUtils.Build(errUtils.ErrCICommentPostFailed).WithCause(err).Err()
	}
	if err := provider.ValidatePostCommitCommentOptions(opts); err != nil {
		return nil, err
	}
	masked := *opts
	masked.Body = provider.MaskPublishedContent(opts.Body)
	if err := provider.ValidatePostCommitCommentOptions(&masked); err != nil {
		return nil, err
	}
	opts = &masked

	behavior, err := provider.NormalizeBehavior(opts.Behavior)
	if err != nil {
		return nil, err
	}
	if behavior == provider.CommentBehaviorCreate {
		return p.createCommitComment(ctx, opts)
	}

	existing, err := p.findCommitCommentByMarker(ctx, opts)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrCICommentListFailed).WithCause(err).Err()
	}
	if existing != nil {
		return p.editCommitComment(ctx, opts, existing.GetID())
	}
	if behavior == provider.CommentBehaviorUpdate {
		return nil, errUtils.Build(errUtils.ErrCICommentNotFound).
			WithExplanation("No existing commit comment matched the marker; behavior=update requires one").
			WithContext("marker", opts.Marker).
			Err()
	}
	return p.createCommitComment(ctx, opts)
}

// findCommitCommentByMarker walks all commit comment pages looking for the first comment whose body
// contains the marker. Returns (nil, nil) when none match or the options carry no marker.
func (p *Provider) findCommitCommentByMarker(ctx context.Context, opts *provider.PostCommitCommentOptions) (*github.RepositoryComment, error) {
	if opts.Marker == "" {
		return nil, nil
	}

	listOpts := &github.ListOptions{PerPage: issueCommentsPerPage}
	for {
		comments, resp, err := p.client.GitHub().Repositories.ListCommitComments(ctx, opts.Owner, opts.Repo, opts.SHA, listOpts)
		if err != nil {
			return nil, wrapGitHubCommentAPIError(err)
		}
		for _, c := range comments {
			if strings.Contains(c.GetBody(), opts.Marker) {
				return c, nil
			}
		}
		if resp == nil || resp.NextPage == 0 {
			return nil, nil
		}
		listOpts.Page = resp.NextPage
	}
}

func (p *Provider) createCommitComment(ctx context.Context, opts *provider.PostCommitCommentOptions) (*provider.Comment, error) {
	payload := &github.RepositoryComment{Body: github.String(opts.Body)}

	created, _, err := p.client.GitHub().Repositories.CreateComment(ctx, opts.Owner, opts.Repo, opts.SHA, payload)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrCICommentPostFailed).
			WithCause(wrapGitHubCommitCommentAPIError(err)).
			Err()
	}

	log.Debug("Created commit comment", "owner", opts.Owner, "repo", opts.Repo, "sha", opts.SHA, "id", created.GetID())
	return &provider.Comment{
		ID:      created.GetID(),
		URL:     created.GetHTMLURL(),
		Body:    created.GetBody(),
		Created: true,
	}, nil
}

func (p *Provider) editCommitComment(ctx context.Context, opts *provider.PostCommitCommentOptions, id int64) (*provider.Comment, error) {
	payload := &github.RepositoryComment{Body: github.String(opts.Body)}

	updated, _, err := p.client.GitHub().Repositories.UpdateComment(ctx, opts.Owner, opts.Repo, id, payload)
	comment, err := updatedComment(updated, err, wrapGitHubCommitCommentAPIError, id)
	if err != nil {
		return nil, err
	}

	log.Debug("Updated commit comment", "owner", opts.Owner, "repo", opts.Repo, "sha", opts.SHA, "id", comment.ID)
	return comment, nil
}

// wrapGitHubCommitCommentAPIError decorates permission failures with the hint for the usual
// misconfiguration: commit comments need `contents: write`, not `pull-requests: write`.
func wrapGitHubCommitCommentAPIError(err error) error {
	hinted := wrapGitHubCommentAPIError(err)
	var ghErr *github.ErrorResponse
	if !errors.As(err, &ghErr) || ghErr.Response == nil {
		return hinted
	}
	switch ghErr.Response.StatusCode {
	case http.StatusForbidden, http.StatusNotFound:
		return errUtils.Build(hinted).
			WithHint("Commit comments need `permissions: contents: write` (not `pull-requests: write`) on the workflow token.").
			Err()
	default:
		return hinted
	}
}
