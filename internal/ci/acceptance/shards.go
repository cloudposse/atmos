package acceptance

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/cloudposse/atmos/internal/ci/rerun"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// DefaultShardPolls bounds how many times CheckShardResults re-lists the
	// shards while any of them still lacks a conclusion.
	DefaultShardPolls = 20
	// DefaultShardPollInterval is the wait between those listings.
	DefaultShardPollInterval = 15 * time.Second

	checkNamePrefix = "Acceptance Tests ("
	checkNameSuffix = ")"
)

var (
	errShardsNotSettled = errors.New("acceptance: shard results never settled")
	errShardCount       = errors.New("acceptance: unexpected number of shard jobs")
	errShardsFailed     = errors.New("acceptance: shard jobs did not succeed")
	errInvalidCheckName = errors.New("acceptance: check name is not of the form \"Acceptance Tests (<target>)\"")
	errShardCountValue  = errors.New("acceptance: shard count must be a positive integer")
)

// TargetFromCheckName derives the OS target ("linux") from the legacy
// required-check name ("Acceptance Tests (linux)"). The workflow's
// `test-required` job takes its matrix from those literal names rather than a
// separate field so verify.go's workflowRequiredCheckPattern keeps matching.
func TargetFromCheckName(check string) (string, error) {
	if !strings.HasPrefix(check, checkNamePrefix) || !strings.HasSuffix(check, checkNameSuffix) {
		return "", fmt.Errorf("%w: %q", errInvalidCheckName, check)
	}
	target := strings.TrimSuffix(strings.TrimPrefix(check, checkNamePrefix), checkNameSuffix)
	if target == "" {
		return "", fmt.Errorf("%w: %q", errInvalidCheckName, check)
	}
	return target, nil
}

// ShardResultsParams describes which shard jobs CheckShardResults judges and
// how long it waits for their conclusions to settle.
type ShardResultsParams struct {
	Run rerun.RunRef
	// Target is the OS token in the shard job names ("linux", "macos", "windows").
	Target string
	// ShardCount is how many shard jobs the workflow declares (TEST_SHARD_COUNT).
	ShardCount int
	// Polls and Interval default to DefaultShardPolls and DefaultShardPollInterval.
	Polls    int
	Interval time.Duration
	// Wait is how the poll loop sleeps; nil means a context-aware time.After.
	// Tests inject a recorder.
	Wait func(context.Context, time.Duration) error
}

// ShardResult is one shard job's name and conclusion. Conclusion is "" while
// the GitHub jobs API has not written it yet.
type ShardResult struct {
	Name       string
	Conclusion string
}

// CheckShardResults judges the acceptance shards of one OS target in a
// workflow-run attempt: it lists the attempt's jobs, keeps the ones named
// "Acceptance Tests (<target>, shard N/M)", and fails unless every one of them
// concluded "success".
//
// The jobs API is eventually consistent: it has returned an empty conclusion
// for a shard that had completed seven minutes earlier, and the shell version
// of this check reported that shard as "did not succeed". So an empty
// conclusion means "not settled yet", not "failed": the listing is repeated
// (Polls times, Interval apart) until every shard has a conclusion, and only
// then judged. A listing that never settles fails with the last listing
// printed; a shard count other than ShardCount fails at once, since every
// job of an attempt exists from the start and a mismatch means the workflow
// changed shape.
func CheckShardResults(ctx context.Context, client rerun.RESTClient, w io.Writer, p *ShardResultsParams) error {
	defer perf.Track(nil, "acceptance.CheckShardResults")()

	if p.ShardCount < 1 {
		return fmt.Errorf("%w: got %d", errShardCountValue, p.ShardCount)
	}
	polls, interval, wait := p.polling()

	var shards []ShardResult
	for attempt := 1; attempt <= polls; attempt++ {
		jobs, err := rerun.FetchJobs(ctx, client, p.Run.Repo, p.Run.RunID, p.Run.RunAttempt)
		if err != nil {
			return err
		}
		shards = shardResults(jobs, p.Target)
		if len(shards) != p.ShardCount {
			fmt.Fprintf(w, "expected %d %q shard jobs, found %d:\n", p.ShardCount, p.Target, len(shards))
			writeShardListing(w, shards)
			return fmt.Errorf("%w: expected %d, found %d", errShardCount, p.ShardCount, len(shards))
		}
		unsettled := countUnsettled(shards)
		if unsettled == 0 {
			return judgeShards(w, p.Target, shards)
		}
		fmt.Fprintf(w, "poll %d/%d: %d of %d %q shard jobs have no conclusion yet; retrying in %s\n",
			attempt, polls, unsettled, len(shards), p.Target, interval)
		if attempt < polls {
			if err := wait(ctx, interval); err != nil {
				return fmt.Errorf("acceptance: waiting for shard results: %w", err)
			}
		}
	}

	fmt.Fprintf(w, "%q shard results never settled after %d polls; last listing:\n", p.Target, polls)
	writeShardListing(w, shards)
	return fmt.Errorf("%w: %q after %d polls", errShardsNotSettled, p.Target, polls)
}

// polling returns the poll count, interval and wait function with defaults
// applied.
func (p *ShardResultsParams) polling() (int, time.Duration, func(context.Context, time.Duration) error) {
	polls, interval, wait := p.Polls, p.Interval, p.Wait
	if polls < 1 {
		polls = DefaultShardPolls
	}
	if interval <= 0 {
		interval = DefaultShardPollInterval
	}
	if wait == nil {
		wait = sleepWithContext
	}
	return polls, interval, wait
}

// shardResults keeps the jobs named "Acceptance Tests (<target>, shard ...)".
func shardResults(jobs []rerun.Job, target string) []ShardResult {
	prefix := checkNamePrefix + target + ", shard"
	var out []ShardResult
	for _, j := range jobs {
		if strings.HasPrefix(j.Name, prefix) {
			out = append(out, ShardResult{Name: j.Name, Conclusion: j.Conclusion})
		}
	}
	return out
}

func countUnsettled(shards []ShardResult) int {
	n := 0
	for _, s := range shards {
		if s.Conclusion == "" {
			n++
		}
	}
	return n
}

func judgeShards(w io.Writer, target string, shards []ShardResult) error {
	var failed []ShardResult
	for _, s := range shards {
		if s.Conclusion != "success" {
			failed = append(failed, s)
		}
	}
	if len(failed) > 0 {
		fmt.Fprintf(w, "the following %q shard jobs did not succeed:\n", target)
		writeShardListing(w, failed)
		return fmt.Errorf("%w: %d of %d %q shards", errShardsFailed, len(failed), len(shards), target)
	}
	fmt.Fprintf(w, "all %d %q shard jobs succeeded\n", len(shards), target)
	return nil
}

func writeShardListing(w io.Writer, shards []ShardResult) {
	for _, s := range shards {
		conclusion := s.Conclusion
		if conclusion == "" {
			conclusion = "(no conclusion yet)"
		}
		fmt.Fprintf(w, "  %s\t%s\n", s.Name, conclusion)
	}
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
