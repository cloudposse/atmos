package edition

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmbeddedJournalPreservesScalarTypes(t *testing.T) {
	entries := mustLoadJournal(journalData)
	byKey := make(map[string]Entry)
	for _, entry := range entries {
		byKey[entry.Key] = entry
	}
	require.Contains(t, byKey, "settings.terminal.pager")
	assert.Equal(t, "true", byKey["settings.terminal.pager"].Old)
	assert.Equal(t, "false", byKey["settings.terminal.pager"].New)
	assert.Equal(t, false, byKey["templates.settings.enabled"].Old)
	assert.Equal(t, true, byKey["templates.settings.enabled"].New)
	assert.Equal(t, 1, byKey["vendor.max_concurrency"].Old)
	assert.Equal(t, 4, byKey["vendor.max_concurrency"].New)
}

func TestEmbeddedJournalRejectsInvalidDocuments(t *testing.T) {
	for _, input := range []string{"[", "[]", "- unknown: true", string(journalData) + "\n---\n[]\n"} {
		t.Run(input[:1], func(t *testing.T) {
			assert.Panics(t, func() { mustLoadJournal([]byte(input)) })
		})
	}
}
