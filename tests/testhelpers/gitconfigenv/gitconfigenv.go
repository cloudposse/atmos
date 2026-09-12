// Package gitconfigenv builds the git CLI's GIT_CONFIG_COUNT/GIT_CONFIG_KEY_n/
// GIT_CONFIG_VALUE_n environment protocol (git 2.31+, see `git help config`
// "ENVIRONMENT") additively, instead of overwriting it.
//
// That protocol is positional: whichever call site sets GIT_CONFIG_COUNT last
// wins outright, silently discarding any entries a different call site (or the
// parent process) already exported at slots 0..N. The Atmos CLI test harness
// has two independent producers of git config overrides -- a process-wide git
// mirror redirect (see tests/testhelpers/gitmirror) and a per-test
// credential.helper/extraheader override -- so a single hardcoded slot range
// is not safe. Append merges them.
package gitconfigenv

import (
	"strconv"
	"strings"
)

// GitConfigEntry is a single `git config` override, materialized as one
// GIT_CONFIG_KEY_n/GIT_CONFIG_VALUE_n pair.
type GitConfigEntry struct {
	Key   string
	Value string
}

// Append reads any GIT_CONFIG_COUNT/KEY_n/VALUE_n entries already present in
// base (typically os.Environ()), appends entries after them, and writes the
// resulting, consistently renumbered GIT_CONFIG_COUNT/KEY_n/VALUE_n set into
// target. Existing entries in base always come first, so an owner-scoped
// mirror redirect exported process-wide is never shadowed by a later,
// per-test override that also wants to add git config entries.
//
// Append is a convenience wrapper around AppendEntries for the common case of
// reading existing entries straight from an environment slice with no
// filtering. Callers that need to drop specific existing entries first (e.g.
// a live-GitHub canary removing the process-wide mirror redirect) should call
// ReadEntries, filter with Without, and then call AppendEntries directly.
func Append(target map[string]string, base []string, entries ...GitConfigEntry) {
	AppendEntries(target, ReadEntries(base), entries...)
}

// AppendEntries writes existing followed by entries into target as a fresh,
// consistently renumbered GIT_CONFIG_COUNT/KEY_n/VALUE_n set. It never mutates
// existing or entries, and never observes later mutation of either slice by
// the caller.
func AppendEntries(target map[string]string, existing []GitConfigEntry, entries ...GitConfigEntry) {
	all := make([]GitConfigEntry, 0, len(existing)+len(entries))
	all = append(all, existing...)
	all = append(all, entries...)

	target["GIT_CONFIG_COUNT"] = strconv.Itoa(len(all))
	for i, entry := range all {
		idx := strconv.Itoa(i)
		target["GIT_CONFIG_KEY_"+idx] = entry.Key
		target["GIT_CONFIG_VALUE_"+idx] = entry.Value
	}
}

// Without returns a copy of entries with every entry matching predicate
// removed. Used, for example, to strip the process-wide git mirror insteadOf
// rules (see gitmirror.InsteadOfRules and IsInsteadOfEntry) from a specific
// test case's git config so a live-GitHub canary is not redirected to the
// local mirror.
func Without(entries []GitConfigEntry, predicate func(GitConfigEntry) bool) []GitConfigEntry {
	kept := make([]GitConfigEntry, 0, len(entries))
	for _, entry := range entries {
		if !predicate(entry) {
			kept = append(kept, entry)
		}
	}
	return kept
}

// IsInsteadOfEntry reports whether entry is a `url.<base>.insteadOf` rule
// (the shape gitmirror.InsteadOfRules produces), as opposed to an unrelated
// git config override such as credential.helper or an HTTP extraheader.
//
// The comparison is case-insensitive because git config variable names are themselves
// case-insensitive (see `git help config`): a rule written as `.insteadOf`, `.insteadof`, or any
// other casing all resolve identically to git, so a case-sensitive suffix check here could miss
// an inherited entry and leave a mirror redirect active in a live canary.
func IsInsteadOfEntry(entry GitConfigEntry) bool {
	return strings.HasSuffix(strings.ToLower(entry.Key), ".insteadof")
}

// IsExtraHeaderEntry reports whether entry is an `http.<url>.extraheader` override -- the shape
// used to inject a GitHub Basic-Auth Authorization header (see
// tests/live_github_canary_test.go's githubCanaryEnv and cli_test.go's runCLICommandTest). Used to
// strip an inherited authorization header from a live-GitHub canary that must run unauthenticated.
//
// The comparison is case-insensitive for the same reason as IsInsteadOfEntry: git config variable
// names are case-insensitive, so `.extraHeader` and `.extraheader` are the same key to git.
func IsExtraHeaderEntry(entry GitConfigEntry) bool {
	return strings.HasSuffix(strings.ToLower(entry.Key), ".extraheader")
}

// ReadEntries extracts any existing GIT_CONFIG_COUNT/KEY_n/VALUE_n entries
// from an environment slice of "KEY=VALUE" pairs (e.g. os.Environ()).
// Exported so callers can filter the result (see Without) before
// re-serializing it with AppendEntries.
func ReadEntries(env []string) []GitConfigEntry {
	lookup := make(map[string]string, len(env))
	for _, kv := range env {
		key, value, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		lookup[key] = value
	}

	count, err := strconv.Atoi(lookup["GIT_CONFIG_COUNT"])
	if err != nil || count <= 0 {
		return nil
	}

	entries := make([]GitConfigEntry, 0, count)
	for i := range count {
		idx := strconv.Itoa(i)
		key, hasKey := lookup["GIT_CONFIG_KEY_"+idx]
		value, hasValue := lookup["GIT_CONFIG_VALUE_"+idx]
		if !hasKey || !hasValue {
			continue
		}
		entries = append(entries, GitConfigEntry{Key: key, Value: value})
	}
	return entries
}
