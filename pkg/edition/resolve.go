package edition

import (
	"reflect"
	"sort"
	"time"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Change describes how one key's effective default differs between two anchors.
type Change struct {
	// Key is the Viper config key whose effective default differs.
	Key string `json:"key" yaml:"key"`
	// Kind is the kind of the journal entries behind this change.
	Kind Kind `json:"kind" yaml:"kind"`
	// FromValue is the effective default at the `from` anchor.
	FromValue any `json:"from_value" yaml:"from_value"`
	// ToValue is the effective default at the `to` anchor.
	ToValue any `json:"to_value" yaml:"to_value"`
	// Entries are the journal entries between the two anchors that produced the change.
	Entries []Entry `json:"entries" yaml:"entries"`
}

// Overrides returns the defaults a project anchored at `a` gets instead of the
// current ones: for each key whose default changed after the anchor date, the
// map holds the value from before the earliest post-anchor change. Keys with no
// post-anchor changes are absent — they keep their current defaults.
func Overrides(a Anchor) map[string]any {
	defer perf.Track(nil, "edition.Overrides")()

	overrides := make(map[string]any)
	// Journal() is sorted oldest-first, so the first post-anchor entry per key
	// wins — its Old value is what the project saw at the anchor date, even when
	// the key changed again later (chained changes).
	for _, entry := range Journal() {
		if entry.Kind != KindValue {
			continue
		}
		if !entryDate(&entry).After(a.Date) {
			continue
		}
		if _, seen := overrides[entry.Key]; !seen {
			overrides[entry.Key] = entry.Old
		}
	}
	return overrides
}

// Diff reports every key whose effective default differs between two anchors.
// A nil anchor means "latest" (no pin), so Diff(pin, nil) answers "what would
// change if I unpinned". The anchors may be given in either order.
func Diff(from, to *Anchor) []Change {
	defer perf.Track(nil, "edition.Diff")()

	entriesByKey := make(map[string][]Entry)
	for _, entry := range Journal() {
		if entry.Kind != KindValue {
			continue
		}
		entriesByKey[entry.Key] = append(entriesByKey[entry.Key], entry)
	}

	var changes []Change
	for key, entries := range entriesByKey {
		fromValue := valueAt(entries, from)
		toValue := valueAt(entries, to)
		if reflect.DeepEqual(fromValue, toValue) {
			continue
		}
		changes = append(changes, Change{
			Key:       key,
			Kind:      KindValue,
			FromValue: fromValue,
			ToValue:   toValue,
			Entries:   entriesBetween(entries, from, to),
		})
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].Key < changes[j].Key })
	return changes
}

// valueAt returns one key's effective default at an anchor (nil = latest):
// the New value of the last entry dated on or before the anchor, or the Old
// value of the key's first entry when the anchor predates all of them.
// The entries must be sorted oldest-first (as Journal() returns them).
func valueAt(entries []Entry, a *Anchor) any {
	value := entries[0].Old
	for _, entry := range entries {
		if a == nil || !entryDate(&entry).After(a.Date) {
			value = entry.New
		}
	}
	return value
}

// entriesBetween returns the entries dated after the earlier anchor and on or
// before the later one, regardless of the order the anchors were given in.
func entriesBetween(entries []Entry, from, to *Anchor) []Entry {
	lower, upper := from, to
	// nil means "latest", which sorts after any date.
	if lower == nil || (upper != nil && upper.Date.Before(lower.Date)) {
		lower, upper = upper, lower
	}
	var window []Entry
	for _, entry := range entries {
		date := entryDate(&entry)
		if lower != nil && !date.After(lower.Date) {
			continue
		}
		if upper != nil && date.After(upper.Date) {
			continue
		}
		window = append(window, entry)
	}
	return window
}

// Between returns the value-kind journal entries dated after `from` and on or
// before `to`, oldest first. A nil bound is unbounded (nil `from` = from the
// beginning, nil `to` = through the latest change).
func Between(from, to *Anchor) []Entry {
	defer perf.Track(nil, "edition.Between")()

	var window []Entry
	for _, entry := range Journal() {
		if entry.Kind != KindValue {
			continue
		}
		date := entryDate(&entry)
		if from != nil && !date.After(from.Date) {
			continue
		}
		if to != nil && date.After(to.Date) {
			continue
		}
		window = append(window, entry)
	}
	return window
}

// entryDate parses an entry's date. Malformed dates return the zero time, which
// makes the entry apply at every anchor; journal_invariants_test.go guarantees
// this never happens for real entries.
func entryDate(entry *Entry) time.Time {
	date, err := time.ParseInLocation(dateLayout, entry.Date, time.UTC)
	if err != nil {
		return time.Time{}
	}
	return date
}
