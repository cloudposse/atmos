package milestone

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMinorMilestoneWindow(t *testing.T) {
	changes, err := MinorMilestoneWindow("v1.233.1", nil)
	require.NoError(t, err)
	require.Equal(t, []MilestoneChange{{0, "v1.234.0"}, {0, "v1.235.0"}, {0, "v1.236.0"}}, changes)
	existing := []Milestone{{1, "1.234.0", "open"}, {2, "v1.235.0", "closed"}, {3, "v1.233.2", "open"}, {4, "v2.0.0", "open"}, {5, "v1.240.0", "open"}}
	changes, err = MinorMilestoneWindow("1.233.1", existing)
	require.NoError(t, err)
	require.Equal(t, []MilestoneChange{{2, "v1.235.0"}, {0, "v1.236.0"}}, changes)
	// A release advances the window without closing unrelated milestones.
	changes, err = MinorMilestoneWindow("1.234.0", []Milestone{{1, "v1.235.0", "open"}, {2, "v1.236.0", "open"}})
	require.NoError(t, err)
	require.Equal(t, []MilestoneChange{{0, "v1.237.0"}}, changes)
	changes, err = MinorMilestoneWindow("2.0.0", nil)
	require.NoError(t, err)
	require.Equal(t, "v2.1.0", changes[0].Title)
	_, err = MinorMilestoneWindow("", nil)
	require.Error(t, err)
	_, err = MinorMilestoneWindow("v1.18446744073709551615.0", nil)
	require.ErrorContains(t, err, "overflow")
	_, err = MinorMilestoneWindow("v1.233.1", []Milestone{{1, "v1.234.0", "open"}, {2, "1.234.0", "closed"}})
	require.ErrorContains(t, err, "duplicate")
}
