package qualitygate

import (
	"fmt"
	"slices"

	"github.com/cloudposse/atmos/magefiles/internal/githubapi"
	"github.com/cloudposse/atmos/magefiles/release/milestone"
)

// releasePolicy uses the merge guard's policy and live repository state. A past
// successful commit status cannot establish eligibility after metadata changes.
func (q *qualityClient) releasePolicy(pr *githubapi.PR) (qualityGate, error) {
	releasePR := milestone.ReleasePR{Number: pr.Number, Labels: pr.Labels}
	if pr.Milestone != nil {
		releasePR.Milestone = &milestone.Milestone{
			Number: pr.Milestone.Number, Title: pr.Milestone.Title, State: pr.Milestone.State,
		}
	}
	label, err := milestone.ReleaseLabel(&releasePR)
	if err != nil {
		return releasePolicyFailure(err), nil
	}
	if label == "no-release" {
		return qualityGate{Passed: true, Detail: "no-release: no release milestone required"}, nil
	}
	c := milestone.Client{Context: q.ctx, API: q.api, Repo: q.repo}
	milestones, err := c.ListMilestones()
	if err != nil {
		return qualityGate{}, err
	}
	baseline, err := c.LatestStable()
	if err != nil {
		return qualityGate{}, err
	}
	if err = milestone.CheckMilestonePR(&releasePR, milestones, baseline); err != nil {
		return releasePolicyFailure(err), nil
	}
	return qualityGate{Passed: true, Detail: fmt.Sprintf("%s fits next release %s (published baseline %s)", label, pr.Milestone.Title, baseline)}, nil
}

func releasePolicyFailure(err error) qualityGate {
	return qualityGate{Detail: err.Error(), Problem: "The PR's release label or milestone does not satisfy the release policy."}
}

// Compare only policy labels; readiness and other automation labels do not
// invalidate a report. Sorting also ignores GitHub's label response order.
func releaseLabels(pr *githubapi.PR) []string {
	var labels []string
	for _, label := range pr.Labels {
		switch label.Name {
		case "major", "minor", "patch", "no-release":
			labels = append(labels, label.Name)
		}
	}
	slices.Sort(labels)
	return labels
}

func sameReleaseAssignment(pr, current *githubapi.PR) bool {
	if !slices.Equal(releaseLabels(pr), releaseLabels(current)) {
		return false
	}
	if pr.Milestone == nil || current.Milestone == nil {
		return pr.Milestone == current.Milestone
	}
	return *pr.Milestone == *current.Milestone
}
