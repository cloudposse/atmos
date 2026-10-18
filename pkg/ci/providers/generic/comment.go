package generic

import (
	"context"
	"strings"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Ensure Provider implements provider.CommitCommenter.
var _ provider.CommitCommenter = (*Provider)(nil)

// shortSHALength is how many characters of a commit SHA the preview shows.
const shortSHALength = 7

// commentLedger remembers, for the life of the process, which comments the generic provider has
// rendered, keyed by marker and location. It stands in for the remote lookup a platform provider
// does, so behavior=update fails with ErrCICommentNotFound locally exactly when it would fail on
// GitHub because no earlier comment carries the marker.
type commentLedger struct {
	mu  sync.Mutex
	ids map[string]int64
}

func newCommentLedger() *commentLedger {
	return &commentLedger{ids: map[string]int64{}}
}

// ledgerKey scopes a marker to where the comment lives, so a pull request comment never satisfies
// an update of a commit comment, nor a comment on one pull request an update on another.
func ledgerKey(scope, marker string) string {
	return scope + "\x00" + marker
}

// lookup returns the ID of an earlier comment with the key, if any.
func (l *commentLedger) lookup(key string) (int64, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	id, ok := l.ids[key]
	return id, ok
}

// remember records the ID of a rendered comment under the key.
func (l *commentLedger) remember(key string, id int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.ids[key] = id
}

// reconcile applies the comment behavior to the ledger and returns the comment ID and whether it
// was created. A comment without a marker can never be found again, so it is always a create.
func (p *Provider) reconcile(scope, marker string, behavior provider.CommentBehavior) (id int64, created bool, err error) {
	if marker == "" {
		return p.nextCommentID.Add(1), true, nil
	}
	key := ledgerKey(scope, marker)
	if behavior != provider.CommentBehaviorCreate {
		if existing, ok := p.comments.lookup(key); ok {
			return existing, false, nil
		}
		if behavior == provider.CommentBehaviorUpdate {
			return 0, false, errUtils.Build(errUtils.ErrCICommentNotFound).
				WithExplanation("No earlier comment in this run matched the marker; behavior=update requires one").
				WithContext("marker", marker).
				Err()
		}
	}
	id = p.nextCommentID.Add(1)
	p.comments.remember(key, id)
	return id, true, nil
}

// PostComment renders a preview of the PR comment locally instead of posting it.
// A laptop run usually has no pull request number and often no owner/repo, so unlike
// platform providers this does not require them; it validates only the body and the
// marker-in-body invariant, then renders the masked body (raw when the provider is
// unbound, as terminal markdown when bound via BindOutput).
//
// The provider remembers the markers it rendered, so behavior=update fails with
// ErrCICommentNotFound when no earlier comment in the process carries the marker, as it does on GitHub.
func (p *Provider) PostComment(_ context.Context, opts *provider.PostCommentOptions) (*provider.Comment, error) {
	defer perf.Track(nil, "generic.Provider.PostComment")()

	if err := validateComment(opts); err != nil {
		return nil, err
	}

	behavior, err := provider.NormalizeBehavior(opts.Behavior)
	if err != nil {
		return nil, err
	}

	scope := "pr:" + itoa(opts.PRNumber)
	id, created, err := p.reconcile(scope, opts.Marker, behavior)
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
		ID:      id,
		Body:    masked,
		Created: created,
	}, nil
}

// PostCommitComment renders a preview of a commit comment locally instead of posting it. Like
// PostComment it needs no owner or repository, only a body and the marker-in-body invariant.
func (p *Provider) PostCommitComment(_ context.Context, opts *provider.PostCommitCommentOptions) (*provider.Comment, error) {
	defer perf.Track(nil, "generic.Provider.PostCommitComment")()

	if opts == nil {
		return nil, validateComment(nil)
	}
	if err := validateComment(&provider.PostCommentOptions{Body: opts.Body, Marker: opts.Marker}); err != nil {
		return nil, err
	}

	behavior, err := provider.NormalizeBehavior(opts.Behavior)
	if err != nil {
		return nil, err
	}

	id, created, err := p.reconcile("commit:"+opts.SHA, opts.Marker, behavior)
	if err != nil {
		return nil, err
	}

	masked := provider.MaskPublishedContent(opts.Body)
	out := p.out()
	if opts.SHA != "" {
		out.Infof("commit comment preview (%s, commit %s)", behavior, shortSHA(opts.SHA))
	} else {
		out.Infof("commit comment preview (%s)", behavior)
	}
	p.render(out, masked)

	return &provider.Comment{
		ID:      id,
		Body:    masked,
		Created: created,
	}, nil
}

// shortSHA abbreviates a commit SHA for display.
func shortSHA(sha string) string {
	if len(sha) > shortSHALength {
		return sha[:shortSHALength]
	}
	return sha
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
