package milestone

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func policyPR(label string, milestone *Milestone) ReleasePR {
	pr := ReleasePR{Number: 42, State: "open", Milestone: milestone}
	pr.Labels = append(pr.Labels, struct {
		Name string `json:"name"`
	}{label})
	return pr
}

func TestMilestoneVersion(t *testing.T) {
	for _, title := range []string{"v1.233.1", "1.234.0", "2.0.0"} {
		_, err := MilestoneVersion(title)
		require.NoError(t, err)
	}
	for _, title := range []string{"1", "1.2", "v01.2.3", "1.2.3-rc.1", "1.2.3+build", " v1.2.3", "Backlog", "1.2.3\n", "$(whoami)"} {
		_, err := MilestoneVersion(title)
		require.ErrorIs(t, err, ErrMilestonePolicy)
	}
}

func TestNextMilestone(t *testing.T) {
	milestones := []Milestone{{1, "v1.10.0", "open"}, {2, "v1.9.0", "open"}, {3, "v1.8.0", "closed"}, {4, "Backlog", "open"}}
	next, err := NextMilestone(milestones)
	require.NoError(t, err)
	require.Equal(t, 2, next.Number)
	milestones = append(milestones, Milestone{5, "1.9.0", "open"})
	_, err = NextMilestone(milestones)
	require.ErrorContains(t, err, "duplicate version")
	_, err = NextMilestone(nil)
	require.ErrorContains(t, err, "no open")
}

func TestMilestoneLabelCompatibility(t *testing.T) {
	for _, tc := range []struct {
		target, label string
		allowed       bool
	}{
		{"v1.233.1", "patch", true},
		{"v1.233.1", "minor", false},
		{"v1.233.1", "major", false},
		{"v1.234.0", "patch", true},
		{"v1.234.0", "minor", true},
		{"v1.234.0", "major", false},
		{"v2.0.0", "patch", true},
		{"v2.0.0", "minor", true},
		{"v2.0.0", "major", true},
	} {
		t.Run(tc.target+"/"+tc.label, func(t *testing.T) {
			m := Milestone{1, tc.target, "open"}
			pr := policyPR(tc.label, &m)
			err := CheckMilestonePR(&pr, []Milestone{m}, "v1.233.0")
			if tc.allowed {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, "cannot enter")
			}
		})
	}
}

func TestMilestonePolicyFailures(t *testing.T) {
	first := Milestone{1, "v1.233.1", "open"}
	later := Milestone{2, "v1.234.0", "open"}
	closed := first
	closed.State = "closed"
	invalid := first
	invalid.Title = "Backlog"
	stale := first
	stale.Title = "v1.233.2"
	for _, tc := range []struct {
		name     string
		pr       ReleasePR
		baseline string
	}{
		{"missing milestone", policyPR("patch", nil), "v1.233.0"},
		{"future milestone", policyPR("patch", &later), "v1.233.0"},
		{"closed", policyPR("patch", &closed), "v1.233.0"},
		{"non version", policyPR("patch", &invalid), "v1.233.0"},
		{"renamed during read", policyPR("patch", &stale), "v1.233.0"},
		{"missing label", policyPR("bug", &first), "v1.233.0"},
		{"published milestone", policyPR("patch", &first), "v1.233.1"},
		{"missing baseline", policyPR("patch", &first), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.ErrorIs(t, CheckMilestonePR(&tc.pr, []Milestone{first, later}, tc.baseline), ErrMilestonePolicy)
		})
	}
	pr := policyPR("no-release", nil)
	require.NoError(t, CheckMilestonePR(&pr, nil, ""))
	pr.Labels = append(pr.Labels, struct {
		Name string `json:"name"`
	}{"minor"})
	require.ErrorContains(t, CheckMilestonePR(&pr, nil, ""), "exactly one")
}

func TestMilestoneBlockedMessage(t *testing.T) {
	first := Milestone{13, "1.233.2", "open"}
	later := Milestone{9, "1.234.0", "open"}
	pr := policyPR("minor", &later)
	err := CheckMilestonePR(&pr, []Milestone{later, first}, "1.233.1")
	require.EqualError(t, err, "Milestone 1.234.0 is blocked by 1.233.2")
	require.ErrorIs(t, err, ErrMilestonePolicy)
}
