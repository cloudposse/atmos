package milestone

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Masterminds/semver/v3"
)

// ErrMilestonePolicy means a PR is not eligible for the current release train.
var ErrMilestonePolicy = errors.New("release milestone policy")

// Milestone identifies a GitHub milestone independently of its mutable title.
type Milestone struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	State  string `json:"state"`
}

// ReleasePR is the live metadata needed by the merge and release checks.
type ReleasePR struct {
	Number    int        `json:"number"`
	State     string     `json:"state"`
	Milestone *Milestone `json:"milestone"`
	Labels    []struct {
		Name string `json:"name"`
	} `json:"labels"`
	Head struct {
		SHA string `json:"sha"`
	} `json:"head"`
	Base struct {
		Ref string `json:"ref"`
	} `json:"base"`
}

// MilestoneVersion accepts complete stable versions, with an optional v prefix.
// Prereleases derive from a stable milestone; planning milestones are not versions.
func MilestoneVersion(title string) (*semver.Version, error) {
	v, err := semver.StrictNewVersion(strings.TrimPrefix(title, "v"))
	if err != nil || v == nil || v.Prerelease() != "" || v.Metadata() != "" {
		return nil, fmt.Errorf("%w: milestone %q must be a stable X.Y.Z or vX.Y.Z version", ErrMilestonePolicy, title)
	}
	return v, nil
}

// NextMilestone returns the lowest open stable version. Non-versioned planning
// milestones are ignored. Equivalent version titles are ambiguous, never ordered
// by creation time, due date, API order or lexical comparison.
func NextMilestone(milestones []Milestone) (*Milestone, error) {
	var next *Milestone
	var lowest *semver.Version
	seen := map[string]int{}
	for _, m := range milestones {
		if m.State != "open" {
			continue
		}
		v, err := MilestoneVersion(m.Title)
		if err != nil {
			continue
		}
		if prior, ok := seen[v.String()]; ok {
			return nil, fmt.Errorf("%w: duplicate version %s in milestones #%d and #%d", ErrMilestonePolicy, v, prior, m.Number)
		}
		seen[v.String()] = m.Number
		if lowest == nil || v.LessThan(lowest) {
			next, lowest = &m, v
		}
	}
	if next == nil {
		return nil, fmt.Errorf("%w: no open versioned milestone", ErrMilestonePolicy)
	}
	return next, nil
}

// RequiredBump compares the milestone with the published stable baseline. Labels
// constrain compatibility; they never select, increment or replace this version.
func RequiredBump(baseline, target string) (string, error) {
	old, err := MilestoneVersion(baseline)
	if err != nil {
		return "", err
	}
	next, err := MilestoneVersion(target)
	if err != nil {
		return "", err
	}
	if !next.GreaterThan(old) {
		return "", fmt.Errorf("%w: milestone %s must exceed published %s", ErrMilestonePolicy, target, baseline)
	}
	if next.Major() > old.Major() {
		return "major", nil
	}
	if next.Minor() > old.Minor() {
		return "minor", nil
	}
	return "patch", nil
}

func ReleaseLabel(pr *ReleasePR) (string, error) {
	label := ""
	for _, l := range pr.Labels {
		switch l.Name {
		case "patch", "minor", "major", "no-release":
			if label != "" {
				return "", fmt.Errorf("%w: PR #%d needs exactly one release label", ErrMilestonePolicy, pr.Number)
			}
			label = l.Name
		}
	}
	if label == "" {
		return "", fmt.Errorf("%w: PR #%d needs patch, minor, major or no-release", ErrMilestonePolicy, pr.Number)
	}
	return label, nil
}

// CheckMilestonePR enforces both ordering and minimum compatibility level.
// The no-release label is the sole exemption; drafts and bot PRs have no implicit bypass.
func CheckMilestonePR(pr *ReleasePR, milestones []Milestone, baseline string) error {
	label, err := ReleaseLabel(pr)
	if err != nil {
		return err
	}
	if label == "no-release" {
		return nil
	}
	next, err := requiredPRMilestone(pr, milestones)
	if err != nil {
		return err
	}
	bump, err := RequiredBump(baseline, next.Title)
	if err != nil {
		return err
	}
	rank := map[string]int{"patch": 1, "minor": 2, "major": 3}
	if rank[label] > rank[bump] {
		return fmt.Errorf("%w: PR #%d labeled %s cannot enter %s milestone %s", ErrMilestonePolicy, pr.Number, label, bump, next.Title)
	}
	return nil
}

func requiredPRMilestone(pr *ReleasePR, milestones []Milestone) (*Milestone, error) {
	if pr.Milestone == nil {
		return nil, fmt.Errorf("%w: PR #%d requires a release milestone", ErrMilestonePolicy, pr.Number)
	}
	if _, err := MilestoneVersion(pr.Milestone.Title); err != nil {
		return nil, err
	}
	next, err := NextMilestone(milestones)
	if err != nil {
		return nil, err
	}
	if pr.Milestone.State != "open" || pr.Milestone.Number != next.Number || pr.Milestone.Title != next.Title {
		return nil, fmt.Errorf("%w: PR #%d must target lowest open milestone %s (#%d)", ErrMilestonePolicy, pr.Number, next.Title, next.Number)
	}
	return next, nil
}
