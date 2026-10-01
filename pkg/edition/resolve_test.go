package edition

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setTestJournal swaps the package journal for the duration of one test.
func setTestJournal(t *testing.T, entries []Entry) {
	t.Helper()
	saved := journal
	journal = entries
	t.Cleanup(func() { journal = saved })
}

// chainedJournal models one key changing twice (A -> B -> C) plus an unrelated
// single change and a behavior entry that must never affect value resolution.
func chainedJournal() []Entry {
	return []Entry{
		{Date: "2025-09-01", Key: "components.demo.mode", Kind: KindValue, Old: "B", New: "C", Description: "second change", Ref: "https://example.com/2"},
		{Date: "2025-03-01", Key: "components.demo.mode", Kind: KindValue, Old: "A", New: "B", Description: "first change", Ref: "https://example.com/1"},
		{Date: "2025-06-15", Key: "settings.demo.enabled", Kind: KindValue, Old: true, New: false, Description: "flip", Ref: "https://example.com/3"},
		{Date: "2025-06-15", Key: "behavior.demo.eval", Kind: KindBehavior, Old: "eager", New: "lazy", Description: "behavior", Ref: "https://example.com/4"},
	}
}

func mustAnchor(t *testing.T, s string) Anchor {
	t.Helper()
	anchor, err := ParseAnchor(s)
	require.NoError(t, err)
	return anchor
}

func TestOverrides(t *testing.T) {
	setTestJournal(t, chainedJournal())

	tests := []struct {
		name   string
		anchor string
		want   map[string]any
	}{
		{
			name:   "anchor before all changes rolls every key back to its oldest value",
			anchor: "2025-01",
			want:   map[string]any{"components.demo.mode": "A", "settings.demo.enabled": true},
		},
		{
			name:   "anchor between chained changes rolls back to the intermediate value",
			anchor: "2025-05",
			want:   map[string]any{"components.demo.mode": "B", "settings.demo.enabled": true},
		},
		{
			name:   "anchor exactly on a change date includes that change",
			anchor: "2025-06-15",
			want:   map[string]any{"components.demo.mode": "B"},
		},
		{
			name:   "anchor after all changes yields no overrides",
			anchor: "2025-12",
			want:   map[string]any{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Overrides(mustAnchor(t, tt.anchor))
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestOverridesIgnoresBehaviorEntries(t *testing.T) {
	setTestJournal(t, chainedJournal())

	overrides := Overrides(mustAnchor(t, "2025-01"))
	_, present := overrides["behavior.demo.eval"]
	assert.False(t, present, "behavior entries must not produce value overrides")
}

func TestDescribePin(t *testing.T) {
	setTestJournal(t, chainedJournal())

	t.Run("empty pin is unpinned", func(t *testing.T) {
		pin, err := DescribePin("", "")
		require.NoError(t, err)
		assert.False(t, pin.Pinned)
		assert.Empty(t, pin.Overrides)
	})
	t.Run("valid pin reports its resolved anchor and overrides", func(t *testing.T) {
		pin, err := DescribePin("2025-05", "config")
		require.NoError(t, err)
		assert.True(t, pin.Pinned)
		assert.Equal(t, "2025-05", pin.Edition)
		assert.Equal(t, "2025-05-31", pin.ResolvedDate)
		assert.Equal(t, GranularityMonth, pin.Granularity)
		assert.Equal(t, "config", pin.Source)
		require.Len(t, pin.Overrides, 2)
	})
	t.Run("invalid pin returns the anchor validation error", func(t *testing.T) {
		_, err := DescribePin("not-a-date", "env")
		require.ErrorIs(t, err, ErrInvalidEdition)
	})
}

func TestDiff(t *testing.T) {
	setTestJournal(t, chainedJournal())

	earliest := mustAnchor(t, "2025-01")
	middle := mustAnchor(t, "2025-05")
	latestPin := mustAnchor(t, "2025-12")

	t.Run("pin to latest spans the whole chain", func(t *testing.T) {
		changes := Diff(&earliest, nil)
		require.Len(t, changes, 2)
		assert.Equal(t, "components.demo.mode", changes[0].Key)
		assert.Equal(t, "A", changes[0].FromValue)
		assert.Equal(t, "C", changes[0].ToValue)
		require.Len(t, changes[0].Entries, 2)
		assert.Equal(t, "2025-03-01", changes[0].Entries[0].Date)
		assert.Equal(t, "2025-09-01", changes[0].Entries[1].Date)
		assert.Equal(t, "settings.demo.enabled", changes[1].Key)
		assert.Equal(t, true, changes[1].FromValue)
		assert.Equal(t, false, changes[1].ToValue)
	})

	t.Run("reverse order swaps from and to values", func(t *testing.T) {
		changes := Diff(nil, &earliest)
		require.Len(t, changes, 2)
		assert.Equal(t, "C", changes[0].FromValue)
		assert.Equal(t, "A", changes[0].ToValue)
	})

	t.Run("window between two pins only includes changes inside it", func(t *testing.T) {
		changes := Diff(&middle, &latestPin)
		require.Len(t, changes, 2)
		assert.Equal(t, "components.demo.mode", changes[0].Key)
		assert.Equal(t, "B", changes[0].FromValue)
		assert.Equal(t, "C", changes[0].ToValue)
		require.Len(t, changes[0].Entries, 1)
		assert.Equal(t, "2025-09-01", changes[0].Entries[0].Date)
	})

	t.Run("window bounded above excludes a later chained entry", func(t *testing.T) {
		// components.demo.mode has two chained entries (2025-03-01, 2025-09-01).
		// Bounding the window at `middle` (2025-05) must include the March entry
		// but exclude the later September one — covers entriesBetween's
		// upper-bound continue, which Between()'s own inlined loop doesn't share.
		changes := Diff(&earliest, &middle)
		require.Len(t, changes, 1)
		assert.Equal(t, "components.demo.mode", changes[0].Key)
		assert.Equal(t, "A", changes[0].FromValue)
		assert.Equal(t, "B", changes[0].ToValue)
		require.Len(t, changes[0].Entries, 1)
		assert.Equal(t, "2025-03-01", changes[0].Entries[0].Date)
	})

	t.Run("identical anchors produce no changes", func(t *testing.T) {
		assert.Empty(t, Diff(&middle, &middle))
	})

	t.Run("both latest produces no changes", func(t *testing.T) {
		assert.Empty(t, Diff(nil, nil))
	})

	t.Run("pin equal to latest state produces no changes", func(t *testing.T) {
		assert.Empty(t, Diff(&latestPin, nil))
	})
}

func TestBetweenAndEntryDateBoundaries(t *testing.T) {
	setTestJournal(t, chainedJournal())

	from := mustAnchor(t, "2025-03")
	to := mustAnchor(t, "2025-06-15")
	entries := Between(&from, &to)
	require.Len(t, entries, 1)
	assert.Equal(t, "settings.demo.enabled", entries[0].Key)

	all := Between(nil, nil)
	require.Len(t, all, 3, "behavior entries are not part of the default journal")

	assert.True(t, entryDate(&Entry{Date: "not-a-date"}).IsZero())
}
