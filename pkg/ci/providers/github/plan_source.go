package github

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/cloudposse/atmos/pkg/perf"
)

var (
	_ provider.CommitTreeResolver        = (*Provider)(nil)
	_ provider.ReviewPullRequestResolver = (*Provider)(nil)
)

// CommitTree uses the immutable checked-out SHA, never a moving branch ref.
func (p *Provider) CommitTree(ctx context.Context, run *provider.Context) (string, error) {
	defer perf.Track(nil, "github.Provider.CommitTree")()
	if !validPlanSourceContext(run) {
		return "", nil
	}
	if err := p.ensureClient(); err != nil {
		return "", err
	}
	commit, _, err := p.client.GitHub().Git.GetCommit(ctx, run.RepoOwner, run.RepoName, run.SHA)
	if err != nil {
		return "", err
	}
	return commit.GetTree().GetSHA(), nil
}

const queuePlanSourceQuery = `query($owner:String!, $repo:String!, $number:Int!) {
 repository(owner:$owner, name:$repo) {
  pullRequest(number:$number) {
   number headRefOid
   mergeQueueEntry { headCommit { oid } }
  }
 }
}`

type queuePlanSourceResponse struct {
	Data struct {
		Repository struct {
			PullRequest *struct {
				Number          int    `json:"number"`
				HeadRefOID      string `json:"headRefOid"`
				MergeQueueEntry *struct {
					HeadCommit *struct {
						OID string `json:"oid"`
					} `json:"headCommit"`
				} `json:"mergeQueueEntry"`
			} `json:"pullRequest"`
		} `json:"repository"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// QueuedPullRequest reads the PR revision and its active queue commit in one
// query. A removed/rebuilt queue entry cannot authorize a newer PR-head plan for
// an old checkout. Combined changes still have to pass fresh-plan verification.
func (p *Provider) QueuedPullRequest(ctx context.Context, run *provider.Context) (*provider.PullRequestRevision, error) {
	defer perf.Track(nil, "github.Provider.QueuedPullRequest")()
	if !validPlanSourceContext(run) || run.MergeQueue == nil || run.MergeQueue.PullRequestNumber <= 0 {
		return nil, nil
	}
	if err := p.ensureClient(); err != nil {
		return nil, err
	}
	// Derive GraphQL from the configured REST endpoint, retaining the same host
	// and transport. GitHub Enterprise uses /api/graphql rather than /api/v3/.
	endpoint := *p.client.GitHub().BaseURL
	endpoint.Path = strings.TrimSuffix(strings.TrimRight(endpoint.Path, "/"), "/v3") + "/graphql"
	body := map[string]any{"query": queuePlanSourceQuery, "variables": map[string]any{
		"owner": run.RepoOwner, "repo": run.RepoName, "number": run.MergeQueue.PullRequestNumber,
	}}
	request, err := p.client.GitHub().NewRequest(http.MethodPost, endpoint.String(), body)
	if err != nil {
		return nil, err
	}
	var result queuePlanSourceResponse
	if _, err := p.client.GitHub().Do(ctx, request, &result); err != nil {
		return nil, err
	}
	if len(result.Errors) != 0 {
		return nil, fmt.Errorf("%w: %s", errUtils.ErrCIMergedPRLookupFailed, result.Errors[0].Message)
	}
	return result.revision(run), nil
}

func (result *queuePlanSourceResponse) revision(run *provider.Context) *provider.PullRequestRevision {
	pr := result.Data.Repository.PullRequest
	if pr == nil || pr.Number != run.MergeQueue.PullRequestNumber || pr.HeadRefOID == "" ||
		pr.MergeQueueEntry == nil || pr.MergeQueueEntry.HeadCommit == nil ||
		!strings.EqualFold(pr.MergeQueueEntry.HeadCommit.OID, run.SHA) {
		return nil
	}
	return &provider.PullRequestRevision{Number: pr.Number, HeadSHA: pr.HeadRefOID}
}

// ReviewPullRequest resolves the reviewed revision without using a moving PR
// head for an unrelated checkout. Queue identity comes from a live bound entry;
// PR identity comes from the event snapshot that produced the synthetic commit.
func (p *Provider) ReviewPullRequest(ctx context.Context, run *provider.Context) (*provider.PullRequestRevision, error) {
	defer perf.Track(nil, "github.Provider.ReviewPullRequest")()
	if run == nil {
		return nil, nil
	}
	if run.MergeQueue != nil {
		return p.QueuedPullRequest(ctx, run)
	}
	if !isPlanSourcePREvent(run) {
		return nil, nil
	}
	payload, err := readEventPayload()
	if err != nil {
		return nil, err
	}
	return reviewRevisionFromPayload(run, payload), nil
}

func reviewRevisionFromPayload(run *provider.Context, payload map[string]any) *provider.PullRequestRevision {
	pr, _ := payload[payloadKeyPullRequest].(map[string]any)
	head := extractPRHeadSHA(payload)
	merge := extractPRMergeCommitSHA(payload)
	if payloadNumber(pr) != run.PullRequest.Number || head == "" ||
		(run.SHA != head && (merge == "" || run.SHA != merge)) {
		return nil
	}
	return &provider.PullRequestRevision{Number: run.PullRequest.Number, HeadSHA: head}
}

func validPlanSourceContext(run *provider.Context) bool {
	return run != nil && run.SHA != "" && run.RepoOwner != "" && run.RepoName != ""
}

func isPlanSourcePREvent(run *provider.Context) bool {
	return run.PullRequest != nil && run.PullRequest.Number > 0 &&
		(run.EventName == "pull_request" || run.EventName == "pull_request_target")
}
