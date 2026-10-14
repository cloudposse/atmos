package generic

import (
	"context"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/cloudposse/atmos/pkg/perf"
)

// PostComment renders a preview of the PR comment locally instead of posting it.
// A laptop run usually has no pull request number and often no owner/repo, so unlike
// platform providers this does not require them; it validates only the body and the
// marker-in-body invariant, then renders the masked body (raw when the provider is
// unbound, as terminal markdown when bound via BindOutput).
func (p *Provider) PostComment(_ context.Context, opts *provider.PostCommentOptions) (*provider.Comment, error) {
	defer perf.Track(nil, "generic.Provider.PostComment")()

	if err := validateComment(opts); err != nil {
		return nil, err
	}

	behavior, err := provider.NormalizeBehavior(opts.Behavior)
	if err != nil {
		return nil, err
	}

	masked := provider.MaskPublishedContent(opts.Body)
	out := p.out()
	if opts.PRNumber > 0 {
		out.Infof("PR comment preview (%s, PR #%d)", behavior, opts.PRNumber)
	} else {
		out.Infof("PR comment preview (%s)", behavior)
	}
	p.render(out, masked)

	return &provider.Comment{
		ID:      p.nextCommentID.Add(1),
		Body:    masked,
		Created: true,
	}, nil
}

// validateComment rejects nil options, an empty body, and a marker that is missing from the body.
func validateComment(opts *provider.PostCommentOptions) error {
	if opts == nil || opts.Body == "" {
		return errUtils.Build(errUtils.ErrCICommentPostFailed).
			WithExplanation("A non-empty Body is required to post a PR comment").
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
