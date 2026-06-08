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

// issueCommentsPerPage is the page size for listing issue comments.
// GitHub allows up to 100 per page.
const issueCommentsPerPage = 100

// PostComment creates or upserts a PR comment using the GitHub Issues API.
// PR comments on GitHub are issue comments on the PR's underlying issue.
//
// Behavior semantics:
//   - CommentBehaviorCreate: always CreateComment.
//   - CommentBehaviorUpdate: find by marker, EditComment; ErrCICommentNotFound if absent.
//   - CommentBehaviorUpsert (default): find by marker, EditComment if found, else CreateComment.
func (p *Provider) PostComment(ctx context.Context, opts *provider.PostCommentOptions) (*provider.Comment, error) {
	defer perf.Track(nil, "github.Provider.PostComment")()

	return p.postComment(ctx, opts)
}

func (p *Provider) postComment(ctx context.Context, opts *provider.PostCommentOptions) (*provider.Comment, error) {
	if err := p.ensureClient(); err != nil {
		return nil, errUtils.Build(errUtils.ErrCICommentPostFailed).WithCause(err).Err()
	}

	if err := validatePostCommentOptions(opts); err != nil {
		return nil, err
	}

	behavior, err := normalizeBehavior(opts.Behavior)
	if err != nil {
		return nil, err
	}
	if behavior == provider.CommentBehaviorCreate {
		return p.createComment(ctx, opts)
	}

	return p.upsertOrUpdate(ctx, opts, behavior)
}

// upsertOrUpdate handles the non-create behaviors: find existing by marker,
// then edit if found or create/error based on behavior.
func (p *Provider) upsertOrUpdate(ctx context.Context, opts *provider.PostCommentOptions, behavior provider.CommentBehavior) (*provider.Comment, error) {
	existing, err := p.findCommentByMarker(ctx, opts.Owner, opts.Repo, opts.PRNumber, opts.Marker)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrCICommentListFailed).WithCause(err).Err()
	}

	if existing != nil {
		return p.editComment(ctx, opts, existing.GetID())
	}

	if behavior == provider.CommentBehaviorUpdate {
		return nil, errUtils.Build(errUtils.ErrCICommentNotFound).
			WithExplanation("No existing PR comment matched the marker; behavior=update requires one").
			WithContext("marker", opts.Marker).
			Err()
	}

	return p.createComment(ctx, opts)
}

// validatePostCommentOptions rejects nil or incomplete option structs, and
// enforces the marker-in-body invariant so repeat runs can reliably reconcile
// against the same comment. An upsert that writes a body without its marker
// would leave a comment that future runs cannot match — breaking idempotency
// and causing duplicate comments on subsequent plans.
func validatePostCommentOptions(opts *provider.PostCommentOptions) error {
	if opts == nil || opts.Owner == "" || opts.Repo == "" || opts.PRNumber <= 0 {
		return errUtils.Build(errUtils.ErrCICommentPostFailed).
			WithExplanation("Owner, Repo, and PRNumber are required to post a PR comment").
			Err()
	}
	if opts.Marker != "" && !strings.Contains(opts.Body, opts.Marker) {
		return errUtils.Build(errUtils.ErrCICommentPostFailed).
			WithExplanation("Marker must appear in Body so future runs can find and update this comment; without it, upserts will create duplicates").
			WithContext("marker", opts.Marker).
			Err()
	}
	return nil
}

// normalizeBehavior resolves the configured behavior. An empty value defaults
// to upsert; any other value must be one of the declared CommentBehavior
// constants. Unknown values fail fast so typos in ci.comments.behavior surface
// immediately rather than silently behaving as upsert.
func normalizeBehavior(b provider.CommentBehavior) (provider.CommentBehavior, error) {
	switch b {
	case "":
		return provider.CommentBehaviorUpsert, nil
	case provider.CommentBehaviorCreate, provider.CommentBehaviorUpdate, provider.CommentBehaviorUpsert:
		return b, nil
	default:
		return "", errUtils.Build(errUtils.ErrCICommentPostFailed).
			WithExplanation("ci.comments.behavior must be one of: create, update, upsert").
			WithContext("behavior", string(b)).
			Err()
	}
}

// findCommentByMarker walks all issue comment pages looking for the first
// comment whose body contains the marker. Returns (nil, nil) when none match.
func (p *Provider) findCommentByMarker(ctx context.Context, owner, repo string, prNumber int, marker string) (*github.IssueComment, error) {
	if marker == "" {
		return nil, nil
	}

	listOpts := &github.IssueListCommentsOptions{
		ListOptions: github.ListOptions{PerPage: issueCommentsPerPage},
	}

	for {
		comments, resp, err := p.client.GitHub().Issues.ListComments(ctx, owner, repo, prNumber, listOpts)
		if err != nil {
			return nil, wrapGitHubCommentAPIError(err)
		}

		for _, c := range comments {
			if strings.Contains(c.GetBody(), marker) {
				return c, nil
			}
		}

		if resp == nil || resp.NextPage == 0 {
			return nil, nil
		}
		listOpts.Page = resp.NextPage
	}
}

func (p *Provider) createComment(ctx context.Context, opts *provider.PostCommentOptions) (*provider.Comment, error) {
	body := opts.Body
	payload := &github.IssueComment{Body: github.String(body)}

	created, _, err := p.client.GitHub().Issues.CreateComment(ctx, opts.Owner, opts.Repo, opts.PRNumber, payload)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrCICommentPostFailed).
			WithCause(wrapGitHubCommentAPIError(err)).
			Err()
	}

	log.Debug("Created PR comment", "owner", opts.Owner, "repo", opts.Repo, "pr", opts.PRNumber, "id", created.GetID())
	return &provider.Comment{
		ID:      created.GetID(),
		URL:     created.GetHTMLURL(),
		Body:    created.GetBody(),
		Created: true,
	}, nil
}

func (p *Provider) editComment(ctx context.Context, opts *provider.PostCommentOptions, id int64) (*provider.Comment, error) {
	payload := &github.IssueComment{Body: github.String(opts.Body)}

	updated, _, err := p.client.GitHub().Issues.EditComment(ctx, opts.Owner, opts.Repo, id, payload)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrCICommentUpdateFailed).
			WithCause(wrapGitHubCommentAPIError(err)).
			WithContext("comment_id", id).
			Err()
	}

	log.Debug("Updated PR comment", "owner", opts.Owner, "repo", opts.Repo, "pr", opts.PRNumber, "id", updated.GetID())
	return &provider.Comment{
		ID:      updated.GetID(),
		URL:     updated.GetHTMLURL(),
		Body:    updated.GetBody(),
		Created: false,
	}, nil
}

// wrapGitHubCommentAPIError decorates permission-related failures with hints
// aimed at the common misconfiguration: missing `pull-requests: write`.
func wrapGitHubCommentAPIError(err error) error {
	var ghErr *github.ErrorResponse
	if !errors.As(err, &ghErr) || ghErr.Response == nil {
		return err
	}

	switch ghErr.Response.StatusCode {
	case http.StatusNotFound:
		return errUtils.Build(err).
			WithHint("A 404 from the GitHub Issues/Comments API usually means the token lacks permission to read or write PR comments.").
			WithHint("Ensure the workflow grants `permissions: pull-requests: write` and that the PR number resolves against the correct repository.").
			WithHint("Set ATMOS_CI_GITHUB_TOKEN to use a separate token with `pull-requests: write` scope when GITHUB_TOKEN is reserved for Terraform.").
			Err()
	case http.StatusForbidden:
		return errUtils.Build(err).
			WithHint("The token does not have permission to write PR comments on this repository.").
			WithHint("Add `permissions: pull-requests: write` to your workflow, or set ATMOS_CI_GITHUB_TOKEN to a token with that scope.").
			Err()
	default:
		return err
	}
}
