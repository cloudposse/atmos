//go:build mage

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/cli/go-gh/v2/pkg/api"

	"github.com/cloudposse/atmos/internal/ci/acceptance"
	"github.com/cloudposse/atmos/internal/ci/rerun"
)

// testShardCountEnv is the workflow-level env test.yml sets for every job.
const testShardCountEnv = "TEST_SHARD_COUNT"

var errMissingShardCount = errors.New("mage: " + testShardCountEnv + " must be set to the number of acceptance shards")

// CheckShardResults is test.yml's per-OS required check ("Acceptance Tests
// (linux)" and friends): it lists the run attempt's jobs live via the GitHub
// REST API, waits for every "Acceptance Tests (<target>, shard N/M)" job to
// have a conclusion (the jobs API is eventually consistent and has returned
// an empty conclusion for a shard that finished minutes earlier), then fails
// unless all of them succeeded. The check argument is the required-check
// name the matrix carries, e.g. "Acceptance Tests (macos)". See
// internal/ci/acceptance.
func (CI) CheckShardResults(repo, runID, runAttempt, check string) error {
	target, err := acceptance.TargetFromCheckName(check)
	if err != nil {
		return err
	}
	count, err := shardCountFromEnv(os.Getenv(testShardCountEnv))
	if err != nil {
		return err
	}
	client, err := api.DefaultRESTClient()
	if err != nil {
		return fmt.Errorf("mage: create GitHub REST client: %w", err)
	}
	return acceptance.CheckShardResults(context.Background(), client, os.Stderr, &acceptance.ShardResultsParams{
		Run:        rerun.RunRef{Repo: repo, RunID: runID, RunAttempt: runAttempt},
		Target:     target,
		ShardCount: count,
	})
}

func shardCountFromEnv(value string) (int, error) {
	if value == "" {
		return 0, errMissingShardCount
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("%w: got %q", errMissingShardCount, value)
	}
	return n, nil
}
