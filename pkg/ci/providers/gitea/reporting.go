package gitea

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/cloudposse/atmos/pkg/perf"
)

type commitStatus struct {
	ID          int64  `json:"id"`
	Context     string `json:"context"`
	State       string `json:"status"`
	Description string `json:"description"`
	TargetURL   string `json:"target_url"`
}

const maxDescriptionLength = 140

func (p *Provider) setStatus(ctx context.Context, opts *provider.CreateCheckRunOptions) (*provider.CheckRun, error) {
	if opts.Owner == "" || opts.Repo == "" || opts.SHA == "" || opts.Name == "" {
		return nil, fmt.Errorf("%w: Gitea status requires owner, repository, SHA, and name", errUtils.ErrCIStatusContextIncomplete)
	}
	mapped := "pending"
	switch opts.Status {
	case provider.CheckRunStateSuccess:
		mapped = "success"
	case provider.CheckRunStateFailure:
		mapped = "failure"
	case provider.CheckRunStateError, provider.CheckRunStateCancelled:
		mapped = "error"
	}
	description := []rune(provider.MaskPublishedContent(opts.Title))
	if len(description) > maxDescriptionLength {
		description = description[:maxDescriptionLength]
	}
	var result commitStatus
	err := p.request(ctx, http.MethodPost, repoPath(opts.Owner, opts.Repo)+"/statuses/"+url.PathEscape(opts.SHA),
		map[string]string{"state": mapped, "context": provider.MaskPublishedContent(opts.Name), "description": string(description), "target_url": provider.MaskPublishedContent(opts.DetailsURL)}, &result)
	if err != nil {
		return nil, err
	}
	return &provider.CheckRun{ID: result.ID, Name: opts.Name, Status: opts.Status, Title: string(description), DetailsURL: opts.DetailsURL}, nil
}

// CreateCheckRun publishes a commit status, keyed by the check name.
func (p *Provider) CreateCheckRun(ctx context.Context, opts *provider.CreateCheckRunOptions) (*provider.CheckRun, error) {
	defer perf.Track(nil, "gitea.CreateCheckRun")()

	if opts == nil {
		return nil, errUtils.ErrCICheckRunCreateFailed
	}
	r, err := p.setStatus(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrCICheckRunCreateFailed, err)
	}
	return r, nil
}

// UpdateCheckRun appends a new status for the same context; Gitea displays its latest value.
func (p *Provider) UpdateCheckRun(ctx context.Context, opts *provider.UpdateCheckRunOptions) (*provider.CheckRun, error) {
	defer perf.Track(nil, "gitea.UpdateCheckRun")()

	if opts == nil {
		return nil, errUtils.ErrCICheckRunUpdateFailed
	}
	r, err := p.setStatus(ctx, &provider.CreateCheckRunOptions{Owner: opts.Owner, Repo: opts.Repo, SHA: opts.SHA, Name: opts.Name, Title: opts.Title, DetailsURL: opts.DetailsURL, Status: opts.Status})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrCICheckRunUpdateFailed, err)
	}
	return r, nil
}

// GetStatus reads the latest status for each context on a commit.
func (p *Provider) GetStatus(ctx context.Context, opts provider.StatusOptions) (*provider.Status, error) {
	defer perf.Track(nil, "gitea.GetStatus")()

	if opts.SHA == "" {
		run, err := p.Context()
		if err != nil {
			return nil, err
		}
		opts.SHA = run.SHA
	}
	if opts.Owner == "" || opts.Repo == "" || opts.SHA == "" {
		return nil, fmt.Errorf("%w: Gitea status requires owner, repository and SHA", errUtils.ErrCIStatusContextIncomplete)
	}
	branch := &provider.BranchStatus{Branch: opts.Branch, CommitSHA: opts.SHA}
	if err := p.readChecks(ctx, opts, branch); err != nil {
		return nil, err
	}

	result := &provider.Status{Repository: opts.Owner + "/" + opts.Repo, CurrentBranch: branch}
	if opts.IncludeUserPRs || opts.IncludeReviewRequests {
		if err := p.addPullRequests(ctx, opts, result); err != nil {
			return nil, err
		}
	}
	return result, nil
}

type comment struct {
	ID   int64  `json:"id"`
	Body string `json:"body"`
	URL  string `json:"html_url"`
	User struct {
		ID int64 `json:"id"`
	} `json:"user"`
}

// PostComment creates or reconciles a PR comment by marker. Only comments owned
// by the authenticated account are eligible for updates.
func (p *Provider) PostComment(ctx context.Context, opts *provider.PostCommentOptions) (*provider.Comment, error) {
	defer perf.Track(nil, "gitea.PostComment")()

	safe, err := safeComment(opts)
	if err != nil {
		return nil, err
	}
	behavior, err := provider.NormalizeBehavior(opts.Behavior)
	if err != nil {
		return nil, err
	}
	path := repoPath(opts.Owner, opts.Repo) + "/issues/" + strconv.Itoa(opts.PRNumber) + "/comments"
	var match *comment
	if behavior != provider.CommentBehaviorCreate && safe.Marker != "" {
		match, err = p.findComment(ctx, path, safe.Marker)
		if err != nil {
			return nil, err
		}
	}
	if match == nil && behavior == provider.CommentBehaviorUpdate {
		return nil, errUtils.ErrCICommentNotFound
	}
	method := http.MethodPost
	if match != nil {
		method = http.MethodPatch
		path = repoPath(opts.Owner, opts.Repo) + "/issues/comments/" + fmt.Sprint(match.ID)
	}
	var result comment
	if err := p.request(ctx, method, path, map[string]string{"body": safe.Body}, &result); err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrCICommentPostFailed, err)
	}
	return &provider.Comment{ID: result.ID, Body: result.Body, URL: result.URL, Created: match == nil}, nil
}

func (p *Provider) readChecks(ctx context.Context, opts provider.StatusOptions, branch *provider.BranchStatus) error {
	latest := map[string]int64{}
	indices := map[string]int{}
	path := repoPath(opts.Owner, opts.Repo) + "/commits/" + url.PathEscape(opts.SHA) + "/statuses?sort=recentupdate"
	statuses := listing[commitStatus]{path: path, id: func(s commitStatus) int64 { return s.ID }, limitErr: errUtils.ErrCIStatusFetchFailed}
	return forEachPage(ctx, p, statuses, func(page []commitStatus) error {
		for _, s := range page {
			if id, exists := latest[s.Context]; exists && s.ID <= id {
				continue
			}
			latest[s.Context] = s.ID
			check := checkStatus(s)
			if index, exists := indices[s.Context]; exists {
				branch.Checks[index] = check
			} else {
				indices[s.Context] = len(branch.Checks)
				branch.Checks = append(branch.Checks, check)
			}
		}
		return nil
	})
}

func (p *Provider) findComment(ctx context.Context, path, marker string) (*comment, error) {
	var user apiUser
	if err := p.request(ctx, http.MethodGet, "/user", nil, &user); err != nil {
		return nil, err
	}
	if user.ID <= 0 {
		return nil, errUtils.ErrCICommentListFailed
	}
	var match *comment
	comments := listing[comment]{path: path, id: func(c comment) int64 { return c.ID }, limitErr: errUtils.ErrCICommentListFailed}
	err := forEachPage(ctx, p, comments, func(page []comment) error {
		for _, c := range page {
			if c.User.ID == user.ID && strings.Contains(c.Body, marker) {
				copy := c
				match = &copy
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return match, nil
}

func checkStatus(s commitStatus) *provider.CheckStatus {
	check := &provider.CheckStatus{Name: s.Context, Status: "completed", Conclusion: s.State, DetailsURL: s.TargetURL}
	if s.State == "pending" {
		check.Status, check.Conclusion = "pending", ""
	}
	if s.State == "error" {
		check.Conclusion = "failure"
	}
	return check
}

func safeComment(opts *provider.PostCommentOptions) (*provider.PostCommentOptions, error) {
	if err := provider.ValidatePostCommentOptions(opts); err != nil {
		return nil, err
	}
	safe := *opts
	safe.Body, safe.Marker = provider.MaskPublishedContent(opts.Body), provider.MaskPublishedContent(opts.Marker)
	if err := provider.ValidatePostCommentOptions(&safe); err != nil {
		return nil, err
	}
	return &safe, nil
}
