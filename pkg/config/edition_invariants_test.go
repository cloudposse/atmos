package config

import (
	"bytes"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/edition"
)

// These tests cross-check the editions journal against the OTHER default layers
// in this package. They live here (not in pkg/edition) because pkg/config
// imports pkg/edition, so the dependency cannot point the other way.

// TestJournalMatchesLiveDefaults asserts that each journaled key's newest entry
// ends at the value setDefaultConfiguration ships today — the journal must
// always be a correct history of the live defaults.
func TestJournalMatchesLiveDefaults(t *testing.T) {
	t.Parallel()

	v := viper.New()
	v.SetConfigType("yaml")
	setDefaultConfiguration(v)

	for key, entry := range latestJournalEntryByKey() {
		assert.Equal(t, canonicalYAML(t, entry.New), canonicalYAML(t, v.Get(key)),
			"journal entry for %s ends at %v but setDefaultConfiguration ships %v; append a new journal entry for the change",
			key, entry.New, v.Get(key))
	}
}

// TestJournalKeysNotInEmbeddedConfig asserts that no journaled key is set by the
// embedded atmos.yaml. The embedded config merges into Viper's CONFIG layer,
// which SetDefault cannot roll back — a journaled key there would make the
// edition pin silently ineffective for that key.
func TestJournalKeysNotInEmbeddedConfig(t *testing.T) {
	t.Parallel()

	v := viper.New()
	v.SetConfigType("yaml")
	require.NoError(t, v.ReadConfig(bytes.NewReader(embeddedConfigData)))

	embeddedKeys := make(map[string]bool)
	for _, key := range v.AllKeys() {
		embeddedKeys[key] = true
	}
	require.Positive(t, len(embeddedKeys), "embedded atmos.yaml parsed to no keys; the guard is misconfigured")

	for _, entry := range edition.Journal() {
		assert.False(t, embeddedKeys[entry.Key],
			"journaled key %s is set in the embedded pkg/config/atmos.yaml; move it to setDefaultConfiguration or the edition pin cannot roll it back",
			entry.Key)
	}
}

// TestJournalNeverGatesEditionKey asserts the edition key itself is never journaled.
func TestJournalNeverGatesEditionKey(t *testing.T) {
	t.Parallel()

	for _, entry := range edition.Journal() {
		assert.NotEqual(t, editionKey, entry.Key, "the edition key is permanently exempt from journaling")
	}
}
