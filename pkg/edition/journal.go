// Package edition implements date-anchored default resolution ("editions") for Atmos.
//
// Atmos defaults change over time. Each change silently alters behavior for users
// when they upgrade. An edition is a date anchor — `edition: "2026"`, `"2026-07"`,
// or `"2026-07-16"` in atmos.yaml — that freezes defaults as they stood on that date.
//
// The shared journal.yaml is an append-only record of every change to a previously
// shipped default. Resolution is a rollback overlay: for a project anchored at
// date D, every journal entry dated after D applies its Old value instead of the
// current default. Entries dated on or before D, and keys with no post-anchor
// entries, keep their current defaults.
//
// Brand-new defaults introduced by new features are never journal-gated — a newly
// introduced key supersedes nothing, so the feature loads with its initial default
// regardless of the anchor. Only changes to previously shipped defaults enter the
// journal.
//
// The `edition` key itself is permanently exempt from journaling (enforced by
// journal_invariants_test.go); otherwise a pin could alter what pinning means.
package edition

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"sort"

	"go.yaml.in/yaml/v3"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Kind classifies what a journal entry gates.
type Kind string

const (
	// KindValue marks a change to a config default value.
	KindValue Kind = "value"
	// KindBehavior marks a dated transition between code paths. Each behavior key
	// identifies one transition; Old and New describe those paths for inspection.
	KindBehavior Kind = "behavior"
)

// Entry records one change to a previously shipped default.
type Entry struct {
	// Date is the day the change shipped, in "YYYY-MM-DD" form (typically the PR merge date).
	Date string `json:"date" yaml:"date"`
	// Key is a Viper config key for KindValue or a unique transition name for KindBehavior.
	Key string `json:"key" yaml:"key"`
	// Kind classifies the entry; only KindValue entries participate in default resolution today.
	Kind Kind `json:"kind" yaml:"kind"`
	// Old is the previous default value, or a description of the old behavior.
	Old any `json:"old" yaml:"old"`
	// New is the current default value, or a description of the new behavior. For
	// the latest KindValue entry per key it must match setDefaultConfiguration.
	New any `json:"new" yaml:"new"`
	// Description explains the change in user-facing terms.
	Description string `json:"description" yaml:"description"`
	// Ref links to the PR or PRD that made the change.
	Ref string `json:"ref" yaml:"ref"`
}

// journalData is the shared source for the CLI journal and the website's edition index.
// Old/New scalar types must be preserved: the string "true" is not a boolean.
//
//go:embed journal.yaml
var journalData []byte

var journal = mustLoadJournal(journalData)

func mustLoadJournal(data []byte) []Entry {
	var entries []Entry
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&entries); err != nil {
		panic(fmt.Errorf("invalid embedded edition journal: %w", err))
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		panic("edition journal must contain exactly one YAML document")
	}
	if len(entries) == 0 {
		panic("edition journal must not be empty")
	}
	return entries
}

// Journal returns a copy of the journal sorted by date (oldest first), then key.
func Journal() []Entry {
	defer perf.Track(nil, "edition.Journal")()

	entries := make([]Entry, len(journal))
	copy(entries, journal)
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Date != entries[j].Date {
			return entries[i].Date < entries[j].Date
		}
		return entries[i].Key < entries[j].Key
	})
	return entries
}
