package edition

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBehaviorChanged(t *testing.T) {
	setTestJournal(t, chainedJournal())
	for _, tc := range []struct {
		pin  string
		want bool
	}{
		{"", true},
		{"2025-06-14", false},
		{"2025-06-15", true},
		{"2025-06-16", true},
		{"2025-05", false},
		{"2025-06", true},
		{"2024", false},
		{"2025", true},
	} {
		t.Run(tc.pin, func(t *testing.T) {
			changed, err := BehaviorChanged(tc.pin, "behavior.demo.eval")
			require.NoError(t, err)
			assert.Equal(t, tc.want, changed)
		})
	}
	for _, pin := range []string{"", "2025-01"} {
		for _, key := range []string{"", "misspelled.behavior", "components.demo.mode"} {
			_, err := BehaviorChanged(pin, key)
			assert.ErrorIs(t, err, ErrUnknownBehavior)
		}
	}
	_, err := BehaviorChanged("2025-02-30", "behavior.demo.eval")
	assert.ErrorIs(t, err, ErrInvalidEdition)
}

func TestBehaviorInspectionMatchesExecution(t *testing.T) {
	setTestJournal(t, chainedJournal())
	before := mustAnchor(t, "2025-06-14")
	on := mustAnchor(t, "2025-06-15")
	changes := Diff(&before, &on)
	require.Len(t, changes, 2)
	assert.Equal(t, Change{
		Key: "behavior.demo.eval", Kind: KindBehavior,
		FromValue: "eager", ToValue: "lazy", Entries: []Entry{chainedJournal()[3]},
	}, changes[0])
	reversed := Diff(&on, &before)
	require.Len(t, reversed, 2)
	assert.Equal(t, "lazy", reversed[0].FromValue)
	assert.Equal(t, "eager", reversed[0].ToValue)
	assert.Equal(t, changes[0].Entries, reversed[0].Entries)
	assert.NotContains(t, Overrides(before), "behavior.demo.eval")
	assert.Equal(t, []Entry{chainedJournal()[3], chainedJournal()[2]}, Between(&before, &on))
	assert.Empty(t, Between(&on, &on), "the lower date is exclusive")
}
