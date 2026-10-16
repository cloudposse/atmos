package ci

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/cloudposse/atmos/pkg/perf"
)

type labelsKey struct {
	provider, server, owner, repo string
	number                        int
}

var labelSnapshots = struct {
	sync.Mutex
	values map[labelsKey][]string
}{values: make(map[labelsKey][]string)}

// ResetLabelsCache starts a fresh invocation's snapshot. The CLI resets it at entry and exit;
// embedded callers should reset it between invocations, after all readers have completed.
func ResetLabelsCache() {
	defer perf.Track(nil, "ci.ResetLabelsCache")()

	labelSnapshots.Lock()
	defer labelSnapshots.Unlock()
	clear(labelSnapshots.values)
}

// Labels returns sorted live labels for the current CI review, or an empty list without one.
// Successful results are snapshotted for the invocation and returned as independent copies.
// Detection and ordinary Context calls never fetch labels; this is an explicit opt-in read.
func Labels(ctx context.Context) ([]string, error) {
	defer perf.Track(nil, "ci.Labels")()

	p := ResolveProvider()
	if p == nil {
		return []string{}, nil
	}
	run, err := p.Context()
	if err != nil {
		return nil, fmt.Errorf("%w: resolving review context: %w", errUtils.ErrCILabelsFetchFailed, err)
	}
	if run == nil || (run.PullRequest == nil && run.MergeQueue == nil) {
		return []string{}, nil
	}
	reader, ok := p.(LabelReader)
	if !ok {
		return nil, fmt.Errorf("%w: %s cannot read review labels", errUtils.ErrCIOperationNotSupported, p.Name())
	}
	if err := provider.ValidateLabelsContext(run); err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrCILabelsFetchFailed, err)
	}
	number := provider.LabelsReviewNumber(run)
	key := labelsKey{p.Name(), run.ServerURL, run.RepoOwner, run.RepoName, number}
	// Serialize the first read so concurrent component evaluations share one complete snapshot.
	labelSnapshots.Lock()
	defer labelSnapshots.Unlock()
	if labels, ok := labelSnapshots.values[key]; ok {
		return slices.Clone(labels), nil
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	labels, err := reader.Labels(ctx, run)
	if err != nil {
		return nil, fmt.Errorf("%w: %s %s/%s review %d: %w", errUtils.ErrCILabelsFetchFailed, p.Name(), run.RepoOwner, run.RepoName, number, err)
	}
	snapshot := append([]string{}, labels...)
	slices.Sort(snapshot)
	snapshot = slices.Compact(snapshot)
	labelSnapshots.values[key] = snapshot
	return slices.Clone(snapshot), nil
}
