package edition

import (
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestJournalInvariants validates the real journal (not a test fixture): every
// entry must be well-formed, per-key chains must be consistent, and the
// `edition` key itself must never be journaled. Cross-checks against the live
// defaults in pkg/config live in that package's tests (pkg/config imports
// pkg/edition, so the dependency cannot point this way).
func TestJournalInvariants(t *testing.T) {
	entries := Journal()
	require.NotEmpty(t, entries, "the journal must contain at least the seed entry")

	byKey := make(map[string][]Entry)
	for _, entry := range entries {
		t.Run(entry.Date+"/"+entry.Key, func(t *testing.T) {
			date, err := time.ParseInLocation(dateLayout, entry.Date, time.UTC)
			require.NoError(t, err, "entry dates must be valid YYYY-MM-DD")
			assert.False(t, date.After(time.Now().UTC()), "entry dates must not be in the future")
			assert.NotEmpty(t, entry.Key)
			assert.NotEqual(t, "edition", entry.Key, "the edition key itself is permanently exempt from journaling")
			assert.Contains(t, []Kind{KindValue, KindBehavior}, entry.Kind)
			assert.Equal(t, KindValue, entry.Kind, "only value entries are supported until behavior gating ships")
			assert.NotEmpty(t, entry.Description)
			assert.NotEmpty(t, entry.Ref)
			assert.False(t, reflect.DeepEqual(entry.Old, entry.New), "an entry must record an actual change")
		})
		byKey[entry.Key] = append(byKey[entry.Key], entry)
	}

	// Chained changes to the same key must be date-ordered with consistent values:
	// each change starts from the value the previous one ended at.
	for key, chain := range byKey {
		for i := 1; i < len(chain); i++ {
			assert.Less(t, chain[i-1].Date, chain[i].Date,
				"entries for %s must have distinct, ascending dates", key)
			assert.Equal(t, chain[i-1].New, chain[i].Old,
				"entry %d for %s must start from the value the previous change ended at", i, key)
		}
	}
}
