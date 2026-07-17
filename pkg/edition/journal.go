// Package edition implements date-anchored default resolution ("editions") for Atmos.
//
// Atmos defaults change over time. Each change silently alters behavior for users
// when they upgrade. An edition is a date anchor — `edition: "2026"`, `"2026-07"`,
// or `"2026-07-16"` in atmos.yaml — that freezes defaults as they stood on that date.
//
// The journal below is an append-only record of every change to a previously
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
	"sort"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Kind classifies what a journal entry gates.
type Kind string

const (
	// KindValue marks a change to a config default value (the only kind used today).
	KindValue Kind = "value"
	// KindBehavior is reserved for future entries that gate code paths rather than
	// config values (Rust-edition-style behavior changes). No entries use it yet.
	KindBehavior Kind = "behavior"
)

// Entry records one change to a previously shipped default.
type Entry struct {
	// Date is the day the change shipped, in "YYYY-MM-DD" form (typically the PR merge date).
	Date string `json:"date" yaml:"date"`
	// Key is the Viper config key whose default changed, e.g. "components.helmfile.use_eks".
	Key string `json:"key" yaml:"key"`
	// Kind classifies the entry; only KindValue entries participate in default resolution today.
	Kind Kind `json:"kind" yaml:"kind"`
	// Old is the default value before the change (what an earlier-anchored project gets).
	Old any `json:"old" yaml:"old"`
	// New is the default value after the change; for the latest entry per key this must
	// match the live default in setDefaultConfiguration (enforced by tests).
	New any `json:"new" yaml:"new"`
	// Description explains the change in user-facing terms.
	Description string `json:"description" yaml:"description"`
	// Ref links to the PR or PRD that made the change.
	Ref string `json:"ref" yaml:"ref"`
}

// journal is the append-only record of default changes, in no particular order;
// Journal() returns it sorted. Changing any value in setDefaultConfiguration
// (pkg/config/load.go) requires appending an entry here in the same PR — the
// snapshot guardrail test in pkg/config enforces this.
//
// Old and New are recorded in the type the config field uses TODAY (e.g. the
// pager default is the string "true", not a bool), because Old is re-injected
// via viper.SetDefault when a project pins an earlier edition.
var journal = []Entry{
	{
		Date:        "2025-02-11",
		Key:         "logs.file",
		Kind:        KindValue,
		Old:         "/dev/stdout",
		New:         "/dev/stderr",
		Description: "Logs are written to stderr so they never contaminate pipeable command output on stdout.",
		Ref:         "https://github.com/cloudposse/atmos/pull/1050",
	},
	{
		Date:        "2025-09-23",
		Key:         "logs.level",
		Kind:        KindValue,
		Old:         "Info",
		New:         "Warning",
		Description: "The default log level is Warning; informational log messages are hidden unless requested.",
		Ref:         "https://github.com/cloudposse/atmos/pull/1430",
	},
	{
		Date:        "2025-10-16",
		Key:         "settings.terminal.pager",
		Kind:        KindValue,
		Old:         "true",
		New:         "false",
		Description: "The built-in pager is disabled by default; long output prints directly to the terminal.",
		Ref:         "https://github.com/cloudposse/atmos/pull/1642",
	},
	{
		Date:        "2026-07-16",
		Key:         "stacks.list.format",
		Kind:        KindValue,
		Old:         "table",
		New:         "tree",
		Description: "Stack listings render the import hierarchy as a tree by default instead of a flat table.",
		Ref:         "https://atmos.tools/changelog/editions",
	},
	{
		Date:        "2026-07-16",
		Key:         "list.instances.format",
		Kind:        KindValue,
		Old:         "table",
		New:         "tree",
		Description: "Instance listings render as a tree by default instead of a flat table.",
		Ref:         "https://atmos.tools/changelog/editions",
	},
	{
		Date:        "2026-07-13",
		Key:         "list.error_mode",
		Kind:        KindValue,
		Old:         "strict",
		New:         "warn",
		Description: "List commands substitute (computed) for unresolved YAML function values and continue instead of aborting.",
		Ref:         "https://atmos.tools/changelog/list-describe-graceful-degradation",
	},
	{
		Date:        "2026-07-13",
		Key:         "describe.error_mode",
		Kind:        KindValue,
		Old:         "strict",
		New:         "warn",
		Description: "Describe commands substitute (computed) for unresolved YAML function values and continue instead of aborting.",
		Ref:         "https://atmos.tools/changelog/list-describe-graceful-degradation",
	},
	{
		Date:        "2025-12-06",
		Key:         "stacks.inherit.metadata",
		Kind:        KindValue,
		Old:         false,
		New:         true,
		Description: "Component metadata is inherited from base components like vars and settings; previously metadata was per-component only.",
		Ref:         "https://atmos.tools/changelog/metadata-inheritance",
	},
	{
		Date:        "2026-07-06",
		Key:         "settings.terminal.help.filter",
		Kind:        KindValue,
		Old:         false,
		New:         true,
		Description: "Bare --help shows a focused view without the GLOBAL FLAGS section; --help=all prints the full output.",
		Ref:         "https://github.com/cloudposse/atmos/pull/2696",
	},
	{
		Date:        "2026-07-17",
		Key:         "describe.component.filter",
		Kind:        KindValue,
		Old:         "full",
		New:         "schema",
		Description: "Component descriptions show only stack-manifest sections; set the filter to full for computed internals.",
		Ref:         "https://atmos.tools/changelog/editions",
	},
	{
		Date:        "2026-07-16",
		Key:         "describe.provenance",
		Kind:        KindValue,
		Old:         false,
		New:         true,
		Description: "Component descriptions include provenance annotations (which stack file set each value) by default.",
		Ref:         "https://atmos.tools/changelog/editions",
	},
	{
		Date:        "2026-02-10",
		Key:         "components.helmfile.use_eks",
		Kind:        KindValue,
		Old:         true,
		New:         false,
		Description: "Helmfile EKS integration is opt-in; kubeconfig is no longer downloaded automatically before Helmfile commands.",
		Ref:         "https://github.com/cloudposse/atmos/pull/1903",
	},
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
