package release

import (
	"fmt"
	"strconv"

	"github.com/cloudposse/atmos/magefiles/internal/githubapi"
	"github.com/cloudposse/atmos/magefiles/release/milestone"
)

// releaseRequest contains explicit workflow inputs; it is not an approved plan.
type releaseRequest struct {
	Mode        string
	Milestone   string
	Version     string
	Commit      string
	PullRequest string
	RunID       string
}

func workflowReleaseRequest(env func(string) string) releaseRequest {
	return releaseRequest{
		Mode:        env("RELEASE_MODE"),
		Milestone:   env("RELEASE_MILESTONE"),
		Version:     env("RELEASE_VERSION"),
		Commit:      env("RELEASE_COMMIT"),
		PullRequest: env("RELEASE_PR"),
		RunID:       env("GITHUB_RUN_ID"),
	}
}

func canonicalRunID(raw string) bool {
	id, err := strconv.ParseUint(raw, decimalBase, githubIDBits)
	return err == nil && id > 0 && strconv.FormatUint(id, decimalBase) == raw
}

func (request *releaseRequest) validatePlan(plan *Plan) error {
	if request.Milestone != "" && request.Milestone != fmt.Sprint(plan.Milestone.Number) {
		return fmt.Errorf("%w: selected milestone changed", ErrMilestonePolicy)
	}
	if request.Version != "" && request.Version != plan.Version {
		return fmt.Errorf("%w: milestone version changed", ErrMilestonePolicy)
	}
	if request.Mode != releaseModeCut && request.Mode != releaseModeCandidate {
		return nil
	}
	if request.Milestone == "" || request.Version == "" || request.Commit == "" {
		return fmt.Errorf("%w: explicit milestone, version and SHA required", ErrMilestonePolicy)
	}
	return nil
}

func validatePrepared(p *preparedRelease) error {
	if !githubapi.RepoPattern.MatchString(p.Repository) || !githubapi.SHAPattern.MatchString(p.Commit) || p.Milestone.Number <= 0 || p.Branch == "" {
		return fmt.Errorf("%w: malformed prepared identity", ErrMilestonePolicy)
	}
	v, err := milestone.MilestoneVersion(p.Milestone.Title)
	if err != nil {
		return err
	}
	if p.Version != "v"+v.String() {
		return fmt.Errorf("%w: prepared milestone version mismatch", ErrMilestonePolicy)
	}
	if !canonicalRunID(p.RunID) {
		return fmt.Errorf("%w: invalid prepared run ID", ErrMilestonePolicy)
	}
	expected, err := expectedTag(p)
	if err != nil {
		return err
	}
	if p.Tag != expected {
		return fmt.Errorf("%w: prepared tag differs from selected identity", ErrMilestonePolicy)
	}
	return nil
}

func expectedTag(p *preparedRelease) (string, error) {
	switch p.Mode {
	case releaseModeCut:
		return p.Version, nil
	case releaseModeCandidate:
		return p.Version + "-rc." + p.RunID, nil
	case releaseModeNightly:
		return p.Version + "-nightly." + p.RunID, nil
	case releaseModeFeature:
		if len(p.Entries) != 1 || p.Entries[0].Number <= 0 {
			return "", fmt.Errorf("%w: feature PR missing", ErrMilestonePolicy)
		}
		return p.Version + "-feature." + fmt.Sprint(p.Entries[0].Number) + "." + p.RunID, nil
	default:
		return "", fmt.Errorf("%w: invalid prepared release mode", ErrMilestonePolicy)
	}
}
