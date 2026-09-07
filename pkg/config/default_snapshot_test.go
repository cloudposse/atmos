package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/edition"
)

// regenerateSnapshotEnvVar regenerates testdata/default-config-snapshot.yaml when set:
//
//	ATMOS_REGENERATE_DEFAULTS_SNAPSHOT=true go test ./pkg/config -run TestDefaultConfigurationSnapshot
const regenerateSnapshotEnvVar = "ATMOS_REGENERATE_DEFAULTS_SNAPSHOT"

// dynamicDefaultKeys are defaults whose values legitimately differ between
// builds and therefore cannot be snapshotted. Keep this list minimal — every
// entry here is exempt from the editions guardrail.
var dynamicDefaultKeys = map[string]bool{
	"components.terraform.append_user_agent": true, // Embeds version.Version.
}

// flattenDefaults runs setDefaultConfiguration into a fresh Viper instance and
// returns its defaults as a flat dotted-key -> canonical-YAML-value map.
func flattenDefaults(t *testing.T) map[string]string {
	t.Helper()
	v := viper.New()
	v.SetConfigType("yaml")
	setDefaultConfiguration(v)

	flat := make(map[string]string)
	for _, key := range v.AllKeys() {
		if dynamicDefaultKeys[key] {
			continue
		}
		flat[key] = canonicalYAML(t, v.Get(key))
	}
	return flat
}

// canonicalYAML renders a value in its canonical single-line YAML form, which
// distinguishes types ("false" renders quoted, false does not).
func canonicalYAML(t *testing.T, value any) string {
	t.Helper()
	out, err := yaml.Marshal(value)
	require.NoError(t, err)
	return strings.TrimSuffix(string(out), "\n")
}

func snapshotPath(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	return filepath.Join(cwd, "testdata", "default-config-snapshot.yaml")
}

// TestDefaultConfigurationSnapshot is the editions guardrail: it fails whenever
// setDefaultConfiguration drifts from the committed snapshot, and a CHANGED
// value additionally requires a matching journal entry in pkg/edition. This
// makes an unjournaled default change un-mergeable.
func TestDefaultConfigurationSnapshot(t *testing.T) {
	current := flattenDefaults(t)
	require.Positive(t, len(current), "setDefaultConfiguration produced no defaults; the guardrail is misconfigured")

	path := snapshotPath(t)
	raw, err := os.ReadFile(path)
	require.NoError(t, err, "missing defaults snapshot; regenerate with %s=true", regenerateSnapshotEnvVar)

	var snapshotValues map[string]any
	require.NoError(t, yaml.Unmarshal(raw, &snapshotValues))
	require.Positive(t, len(snapshotValues), "defaults snapshot is empty; regenerate with %s=true", regenerateSnapshotEnvVar)

	snapshot := make(map[string]string, len(snapshotValues))
	for key, value := range snapshotValues {
		snapshot[key] = canonicalYAML(t, value)
	}

	if os.Getenv(regenerateSnapshotEnvVar) != "" {
		violations := validateSnapshotRegeneration(snapshot, current, latestJournalEntryByKey())
		require.Empty(t, violations, strings.Join(violations, "\n"))
		regenerateSnapshot(t, path, current)
		return
	}

	journalByKey := latestJournalEntryByKey()

	for key, snapshotValue := range snapshot {
		currentValue, exists := current[key]
		if !exists {
			assert.Fail(t, "default removed without snapshot regeneration",
				"key %s exists in the snapshot but not in setDefaultConfiguration. Removing a default is out of the editions journal's scope: document the removal and regenerate the snapshot with %s=true.",
				key, regenerateSnapshotEnvVar)
			continue
		}
		if currentValue == snapshotValue {
			continue
		}

		entry, journaled := journalByKey[key]
		if !journaled {
			assert.Fail(t, "default changed without a journal entry",
				"key %s changed from %s to %s. Changing a shipped default requires BOTH: (1) a dated entry in pkg/edition/journal.go with Old/New matching this change, and (2) snapshot regeneration with %s=true.",
				key, snapshotValue, currentValue, regenerateSnapshotEnvVar)
			continue
		}
		assert.Equal(t, snapshotValue, canonicalYAML(t, entry.Old),
			"journal entry for %s has Old=%v but the snapshot says the previous default was %s", key, entry.Old, snapshotValue)
		assert.Equal(t, currentValue, canonicalYAML(t, entry.New),
			"journal entry for %s has New=%v but the live default is %s", key, entry.New, currentValue)
		assert.Fail(t, "default changed; snapshot needs regeneration",
			"key %s changed and is journaled — now regenerate the snapshot with %s=true to accept it.", key, regenerateSnapshotEnvVar)
	}

	for key := range current {
		if _, exists := snapshot[key]; !exists {
			// New defaults are never journal-gated; they only need snapshot regeneration.
			assert.Fail(t, "new default missing from snapshot",
				"key %s is a new default. New keys need no journal entry — regenerate the snapshot with %s=true.",
				key, regenerateSnapshotEnvVar)
		}
	}
}

// latestJournalEntryByKey returns the newest value-kind journal entry per key
// (only the newest can match a snapshot-vs-live diff).
func latestJournalEntryByKey() map[string]edition.Entry {
	byKey := make(map[string]edition.Entry)
	// Journal() is sorted oldest-first, so later iterations overwrite earlier ones.
	for _, entry := range edition.Journal() {
		if entry.Kind != edition.KindValue {
			continue
		}
		byKey[entry.Key] = entry
	}
	return byKey
}

// validateSnapshotRegeneration prevents a regeneration from accepting an
// unjournaled change to a previously shipped default.
func validateSnapshotRegeneration(snapshot, current map[string]string, journalByKey map[string]edition.Entry) []string {
	var violations []string
	for key, previous := range snapshot {
		latest, exists := current[key]
		if !exists || latest == previous {
			continue
		}

		entry, journaled := journalByKey[key]
		if !journaled {
			violations = append(violations, fmt.Sprintf("default changed without a journal entry: key %s changed from %s to %s", key, previous, latest))
			continue
		}
		if journalOld := canonicalYAMLValue(entry.Old); journalOld != previous {
			violations = append(violations, fmt.Sprintf("journal entry for %s has Old=%s but the snapshot says %s", key, journalOld, previous))
		}
		if journalNew := canonicalYAMLValue(entry.New); journalNew != latest {
			violations = append(violations, fmt.Sprintf("journal entry for %s has New=%s but the live default is %s", key, journalNew, latest))
		}
	}
	return violations
}

func canonicalYAMLValue(value any) string {
	encoded, err := yaml.Marshal(value)
	if err != nil {
		return fmt.Sprintf("<invalid YAML: %v>", err)
	}
	return strings.TrimSuffix(string(encoded), "\n")
}

func TestValidateSnapshotRegeneration(t *testing.T) {
	snapshot := map[string]string{"settings.example.enabled": "false"}
	current := map[string]string{"settings.example.enabled": "true"}
	matchingJournal := map[string]edition.Entry{
		"settings.example.enabled": {Old: false, New: true},
	}

	t.Run("accepts a matching journal entry", func(t *testing.T) {
		assert.Empty(t, validateSnapshotRegeneration(snapshot, current, matchingJournal))
	})
	t.Run("rejects an unjournaled changed default", func(t *testing.T) {
		assert.Contains(t, strings.Join(validateSnapshotRegeneration(snapshot, current, nil), "\n"), "without a journal entry")
	})
	t.Run("rejects a journal entry with the wrong old value", func(t *testing.T) {
		journal := map[string]edition.Entry{
			"settings.example.enabled": {Old: true, New: true},
		}
		assert.Contains(t, strings.Join(validateSnapshotRegeneration(snapshot, current, journal), "\n"), "has Old")
	})
}

func regenerateSnapshot(t *testing.T, path string, current map[string]string) {
	t.Helper()
	keys := make([]string, 0, len(current))
	for key := range current {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var builder strings.Builder
	builder.WriteString("# Snapshot of setDefaultConfiguration (pkg/config/load.go), the editions guardrail baseline.\n")
	builder.WriteString("# Do not edit by hand. Regenerate with:\n")
	builder.WriteString("#   " + regenerateSnapshotEnvVar + "=true go test ./pkg/config -run TestDefaultConfigurationSnapshot\n")
	builder.WriteString("# Changing an existing value here requires a journal entry in pkg/edition/journal.go.\n")
	for _, key := range keys {
		builder.WriteString(key + ": " + current[key] + "\n")
	}

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(builder.String()), 0o644))
	t.Logf("regenerated defaults snapshot at %s", path)
}
