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
func Append(target map[string]string, base []string, entries ...GitConfigEntry) {
	all := append(readEntries(base), entries...)

	target["GIT_CONFIG_COUNT"] = strconv.Itoa(len(all))
	for i, entry := range all {
		idx := strconv.Itoa(i)
		target["GIT_CONFIG_KEY_"+idx] = entry.Key
		target["GIT_CONFIG_VALUE_"+idx] = entry.Value
	}
}

// readEntries extracts any existing GIT_CONFIG_COUNT/KEY_n/VALUE_n entries
// from an environment slice of "KEY=VALUE" pairs (e.g. os.Environ()).
func readEntries(env []string) []GitConfigEntry {
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
