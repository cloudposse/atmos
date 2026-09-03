package rerun

//go:generate go run go.uber.org/mock/mockgen -package rerun -destination mock_gh.go github.com/cloudposse/atmos/internal/ci/rerun RESTClient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"

	"github.com/cloudposse/atmos/pkg/perf"
)

// RESTClient is the slice of *api.RESTClient's (github.com/cli/go-gh/v2/pkg/api)
// method set that this package needs: one authenticated request, returning
// the raw response so callers can read pagination headers. Its narrowness
// lets tests inject a fake instead of a live GitHub API.
type RESTClient interface {
	RequestWithContext(ctx context.Context, method, path string, body io.Reader) (*http.Response, error)
}

// nextLinkPattern extracts the next-page URL from a GitHub Link header, e.g.
// `<https://api.github.com/...&page=2>; rel="next", <...>; rel="last"`.
var nextLinkPattern = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)

// FetchJobs pages through repos/{repo}/actions/runs/{runID}/attempts/{runAttempt}/jobs
// (per_page=100), following the Link header until it stops offering a "next"
// page - the same request `gh api ... --paginate` makes. It is the only
// exported function in this package that talks to the network on the read
// path; classify.go's Classify/Verdict/etc. remain pure.
func FetchJobs(ctx context.Context, client RESTClient, repo, runID, runAttempt string) ([]Job, error) {
	defer perf.Track(nil, "rerun.FetchJobs")()

	path := fmt.Sprintf("repos/%s/actions/runs/%s/attempts/%s/jobs?per_page=100", repo, runID, runAttempt)
	var jobs []Job
	for path != "" {
		resp, err := client.RequestWithContext(ctx, http.MethodGet, path, nil)
		if err != nil {
			return nil, fmt.Errorf("rerun: fetch jobs: %w", err)
		}
		page, err := decodeJobsResponse(resp)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, page...)
		path = nextLink(resp.Header.Get("Link"))
	}
	return jobs, nil
}

func decodeJobsResponse(resp *http.Response) ([]Job, error) {
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("rerun: read jobs response: %w", err)
	}
	return decodePage(body)
}

// nextLink returns the URL of the "next" relation in a GitHub Link header, or
// "" when there is none (the last page).
func nextLink(header string) string {
	m := nextLinkPattern.FindStringSubmatch(header)
	if m == nil {
		return ""
	}
	return m[1]
}

// PRHeadSHA returns the current head commit SHA of pull request number in
// repo (owner/name form).
func PRHeadSHA(ctx context.Context, client RESTClient, repo string, number int) (string, error) {
	defer perf.Track(nil, "rerun.PRHeadSHA")()

	path := fmt.Sprintf("repos/%s/pulls/%d", repo, number)
	resp, err := client.RequestWithContext(ctx, http.MethodGet, path, nil)
	if err != nil {
		return "", fmt.Errorf("rerun: get pull request %d: %w", number, err)
	}
	defer resp.Body.Close()

	var pr struct {
		Head struct {
			SHA string `json:"sha"`
		} `json:"head"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return "", fmt.Errorf("rerun: decode pull request %d: %w", number, err)
	}
	return pr.Head.SHA, nil
}
