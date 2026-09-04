package rerun

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"

	"github.com/cli/go-gh/v2/pkg/api"

	"github.com/cloudposse/atmos/pkg/perf"
)

// stillRunningPattern matches the GitHub API's rerun-failed-jobs error
// message when a run has not finished yet - it cannot be rerun until it does,
// but that is not a failure worth surfacing as one: the run will report its
// own success or failure once it completes.
var stillRunningPattern = regexp.MustCompile(`(?i)already running|in progress`)

// RerunFailedJobs requests a rerun of every non-success job in runID
// (repos/{repo}/actions/runs/{runID}/rerun-failed-jobs), mirroring
// `gh run rerun <runID> --failed`. When the API rejects the request because
// the run has not finished yet, it reports stillRunning=true instead of
// returning an error.
func RerunFailedJobs(ctx context.Context, client RESTClient, repo, runID string) (stillRunning bool, err error) {
	defer perf.Track(nil, "rerun.RerunFailedJobs")()

	path := fmt.Sprintf("repos/%s/actions/runs/%s/rerun-failed-jobs", repo, runID)
	resp, err := client.RequestWithContext(ctx, http.MethodPost, path, nil)
	if err != nil {
		var httpErr *api.HTTPError
		if errors.As(err, &httpErr) && stillRunningPattern.MatchString(httpErr.Message) {
			return true, nil
		}
		return false, fmt.Errorf("rerun: rerun failed jobs for run %s: %w", runID, err)
	}
	defer resp.Body.Close()
	return false, nil
}
