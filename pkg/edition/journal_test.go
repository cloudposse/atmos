package edition

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJournalSortedOldestFirst(t *testing.T) {
	setTestJournal(t, chainedJournal())

	entries := Journal()
	require.Len(t, entries, 4)
	assert.Equal(t, "2025-03-01", entries[0].Date)
	assert.Equal(t, "2025-09-01", entries[len(entries)-1].Date)
	// Same-date entries are ordered by key.
	assert.Equal(t, "behavior.demo.eval", entries[1].Key)
	assert.Equal(t, "settings.demo.enabled", entries[2].Key)
}

func TestJournalReturnsIsolatedCopy(t *testing.T) {
	setTestJournal(t, chainedJournal())

	first := Journal()
	first[0].Key = "mutated"
	first[0].Old = "mutated"

	second := Journal()
	assert.Equal(t, "components.demo.mode", second[0].Key, "mutating the returned slice must not affect the journal")
	assert.Equal(t, "A", second[0].Old)
}
