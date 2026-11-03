package qualitygate

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudposse/atmos/magefiles/bench"
	"github.com/cloudposse/atmos/magefiles/internal/githubapi"
)

const (
	qualityStatusBlockersPrefix = "<!-- atmos-quality-status-blockers:"
	// Marks a report evaluated after reporter runs stopped counting as CI evidence.
	// v2: Quality Gate Signal runs joined the excluded reporter runs.
	qualityReporterRunsExcluded = "<!-- atmos-quality-reporter-runs-excluded:v2 -->"
)

type qualityStatusBlocker struct {
	SHA     string `json:"sha"`
	Context string `json:"context"`
	State   string `json:"state"`
}

// Record only blocking commit statuses. Recovery needs no status reads for an
// old not-ready PR blocked solely by reviews, signatures or merge conflicts.
func qualityStatusBlockersMarker(pr *githubapi.PR, gates []qualityGate) string {
	blockers := []qualityStatusBlocker{}
	for i := range gates {
		gate := &gates[i]
		if gate.ID != "ci" || gate.Passed {
			continue
		}
		for _, signal := range gate.Signals {
			name, sha := signal.Name, pr.Head.SHA
			if merged, ok := strings.CutPrefix(name, "merge: "); ok {
				name, sha = merged, pr.MergeSHA
			}
			if context, ok := strings.CutPrefix(name, "status: "); ok {
				blockers = append(blockers, qualityStatusBlocker{SHA: sha, Context: context, State: signal.State})
			}
		}
	}
	// JSON escapes HTML delimiters in third-party context names as well.
	data, _ := json.Marshal(blockers)
	return qualityStatusBlockersPrefix + string(data) + " -->"
}

// A replaced orchestration run in an old report needs one fresh evaluation.
// Reports predating status snapshots also need one when they contain statuses;
// successful publication adds a snapshot, preventing a recurring full sweep.
// A report blocked by the reporter's own run, including an unapproved Quality
// Gate Signal run, is re-evaluated once at any age. The fresh report carries the
// current marker, so a blocker from an untrusted run of the same path remains in
// that report without selecting the PR on every schedule.
func qualityLegacyStatusReport(body string) bool {
	if !strings.Contains(body, qualityReporterRunsExcluded) {
		for _, workflow := range []string{qualityReportWorkflow, qualityReconcileWorkflow, qualitySignalWorkflow} {
			if strings.Contains(body, "workflow: "+workflow) {
				return true
			}
		}
	}
	return !strings.Contains(body, qualityStatusBlockersPrefix) &&
		(strings.Contains(body, releaseMilestonesWorkflow) || strings.Contains(body, "status: "))
}

// A missing snapshot has no recorded blockers; a malformed one needs a fresh
// evaluation so recovery can replace it with valid evidence.
func qualityRecordedStatusBlockers(body string) ([]qualityStatusBlocker, bool) {
	_, encoded, found := strings.Cut(body, qualityStatusBlockersPrefix)
	if !found {
		return nil, true
	}
	encoded, _, closed := strings.Cut(encoded, " -->")
	if !closed {
		return nil, false
	}
	var blockers []qualityStatusBlocker
	err := json.Unmarshal([]byte(encoded), &blockers)
	return blockers, err == nil
}

func (q *qualityClient) changedBlockingStatus(pr *githubapi.PR, body string) (string, error) {
	blockers, valid := qualityRecordedStatusBlockers(body)
	if !valid {
		return qualityReasonStatusChanged, nil
	}
	current := map[string]map[string]string{}
	for _, blocker := range blockers {
		// Report contents are data, not authority to query arbitrary commits.
		if blocker.SHA != pr.Head.SHA && (pr.MergeSHA == "" || blocker.SHA != pr.MergeSHA) {
			return qualityReasonStatusChanged, nil
		}
		if current[blocker.SHA] == nil {
			states, err := q.currentStatusStates(blocker.SHA)
			if err != nil {
				return qualityReasonUnknown, err
			}
			current[blocker.SHA] = states
		}
		if current[blocker.SHA][blocker.Context] != blocker.State {
			return qualityReasonStatusChanged, nil
		}
	}
	return qualityReasonNone, nil
}

// The combined endpoint returns current contexts, avoiding the unbounded history
// of repeated milestone status publications. Read each head/merge SHA at most once.
func (q *qualityClient) currentStatusStates(sha string) (map[string]string, error) {
	states := map[string]string{}
	for page := 1; page <= githubapi.PageLimit; page++ {
		var response struct {
			Statuses []struct{ Context, State string } `json:"statuses"`
		}
		if err := q.get(fmt.Sprintf("commits/%s/status?per_page=100&page=%d", sha, page), &response); err != nil {
			return nil, err
		}
		for _, status := range response.Statuses {
			states[status.Context] = status.State
		}
		if len(response.Statuses) < githubapi.PageSize {
			return states, nil
		}
	}
	return nil, bench.Error("current status pagination limit exceeded")
}
