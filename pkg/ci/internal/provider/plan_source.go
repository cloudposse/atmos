package provider

import "context"

// CommitTreeResolver identifies the immutable source tree of a commit. It lets
// plan storage correlate equivalent source checkouts across rewritten commits.
type CommitTreeResolver interface {
	CommitTree(ctx context.Context, run *Context) (string, error)
}

// ReviewPullRequestResolver resolves a PR head only while the provider's queue
// entry still identifies the checked-out queue commit. PR events may resolve
// the immutable head revision from their checkout-bound event payload. A stale queue run must
// never resolve the latest head of a rebuilt or removed entry.
type ReviewPullRequestResolver interface {
	ReviewPullRequest(ctx context.Context, run *Context) (*PullRequestRevision, error)
}
