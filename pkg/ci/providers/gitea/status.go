package gitea

import (
	"context"
	"net/http"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
)

type apiUser struct {
	ID int64 `json:"id"`
}
type pullRequest struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	URL       string    `json:"html_url"`
	Head      revision  `json:"head"`
	Base      revision  `json:"base"`
	User      apiUser   `json:"user"`
	Reviewers []apiUser `json:"requested_reviewers"`
}

func (p *Provider) addPullRequests(ctx context.Context, opts provider.StatusOptions, result *provider.Status) error {
	var user apiUser
	if err := p.request(ctx, http.MethodGet, "/user", nil, &user); err != nil {
		return err
	}
	open := listing[pullRequest]{
		path:     repoPath(opts.Owner, opts.Repo) + "/pulls?state=open",
		id:       func(pr pullRequest) int64 { return int64(pr.Number) },
		limitErr: errUtils.ErrCIStatusFetchFailed,
	}
	return forEachPage(ctx, p, open, func(prs []pullRequest) error {
		for i := range prs {
			if err := p.addPullRequest(ctx, opts, result, &prs[i], user.ID); err != nil {
				return err
			}
		}
		return nil
	})
}

func (p *Provider) addPullRequest(ctx context.Context, opts provider.StatusOptions, result *provider.Status, pr *pullRequest, userID int64) error {
	created := opts.IncludeUserPRs && pr.User.ID == userID
	review := opts.IncludeReviewRequests && requestedReviewer(pr, userID)
	current := pr.Head.SHA == opts.SHA
	if !created && !review && !current {
		return nil
	}
	status, err := p.GetStatus(ctx, provider.StatusOptions{Owner: opts.Owner, Repo: opts.Repo, SHA: pr.Head.SHA})
	if err != nil {
		return err
	}
	item := &provider.PRStatus{Number: pr.Number, Title: pr.Title, URL: pr.URL, Branch: pr.Head.Ref, BaseBranch: pr.Base.Ref, Checks: status.CurrentBranch.Checks, AllPassed: len(status.CurrentBranch.Checks) > 0}
	item.AllPassed = allChecksPassed(item.Checks)
	if current {
		result.CurrentBranch.PullRequest = item
	}
	if created {
		result.CreatedByUser = append(result.CreatedByUser, item)
	}
	if review {
		result.ReviewRequests = append(result.ReviewRequests, item)
	}
	return nil
}

func requestedReviewer(pr *pullRequest, userID int64) bool {
	for _, reviewer := range pr.Reviewers {
		if reviewer.ID == userID {
			return true
		}
	}
	return false
}

func allChecksPassed(checks []*provider.CheckStatus) bool {
	if len(checks) == 0 {
		return false
	}
	for _, check := range checks {
		if check.Conclusion != "success" {
			return false
		}
	}
	return true
}
