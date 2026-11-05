package qualitygate

import (
	"context"
	"strconv"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/magefile/mage/mg"

	"github.com/cloudposse/atmos/magefiles/bench"
	"github.com/cloudposse/atmos/magefiles/internal/githubapi"
)

// Bench adds the trusted publisher to the benchmark namespace.
type Bench mg.Namespace

// Report validates a benchmark run and updates the composite quality gate.
func (Bench) Report(repo, runID string) error {
	if !githubapi.RepoPattern.MatchString(repo) {
		return bench.Error("invalid repository")
	}
	id, err := strconv.ParseInt(runID, bench.Decimal, bench.FloatBits)
	if err != nil || id <= 0 {
		return bench.Error("invalid run ID")
	}
	client, err := api.DefaultRESTClient()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	q := qualityClient{ctx: ctx, api: client, repo: repo}
	return bench.ReportRun(ctx, client, repo, id, q.refresh)
}
