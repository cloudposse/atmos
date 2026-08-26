package config

import (
	"bytes"
	"encoding/json"
	"strings"
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
	for _, entry := range edition.Journal() {
		assert.NotEqual(t, editionKey, entry.Key, "the edition key is permanently exempt from journaling")
	}
}

// TestJournalAgreesWithDefaultCliConfig asserts that defaultCliConfig (the
// fallback applied when no atmos.yaml exists) states the same current value as
// each journaled key's newest entry, so both code paths ship one default. A
// key that's genuinely absent from the marshaled struct is accepted — it means
// the struct simply doesn't state that field. A key that IS present (even as a
// zero value like false or "") must agree with the journal, since a struct
// literal with no omitempty tag serializes its zero value explicitly and that
// value competes with (and can silently override) the journaled default.
func TestJournalAgreesWithDefaultCliConfig(t *testing.T) {
	// Load defaultCliConfig the same way mergeDefaultConfig does.
	j, err := json.Marshal(defaultCliConfig)
	require.NoError(t, err)
	v := viper.New()
	v.SetConfigType("json")
	require.NoError(t, v.ReadConfig(bytes.NewReader(j)))

	// Parse into a raw map so we can distinguish "key absent" from "key present
	// but falsy" — viper.Get(key) == false conflates the two.
	var raw map[string]any
	require.NoError(t, json.Unmarshal(j, &raw))

	for key, entry := range latestJournalEntryByKey() {
		if !jsonPathPresent(raw, key) {
			continue // Field not stated by the struct; nothing to disagree with.
		}
		structValue := v.Get(key)
		assert.Equal(t, canonicalYAML(t, entry.New), canonicalYAML(t, structValue),
			"defaultCliConfig states %v for %s but the journal's current value is %v; align them (see the use_eks drift this feature fixed)",
			structValue, key, entry.New)
	}
}

// jsonPathPresent reports whether a dot-separated key path is present in a map
// decoded from JSON (e.g. "settings.terminal.help.filter"), regardless of
// whether its value is a zero value like false, "", or 0.
func jsonPathPresent(raw map[string]any, key string) bool {
	parts := strings.Split(key, ".")
	current := raw
	for i, part := range parts {
		value, ok := current[part]
		if !ok {
			return false
		}
		if i == len(parts)-1 {
			return true
		}
		next, ok := value.(map[string]any)
		if !ok {
			return false
		}
		current = next
	}
	return true
}
