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

	scrubGitConfigKeys(target)

	target["GIT_CONFIG_COUNT"] = strconv.Itoa(len(all))
	for i, entry := range all {
		idx := strconv.Itoa(i)
		target["GIT_CONFIG_KEY_"+idx] = entry.Key
		target["GIT_CONFIG_VALUE_"+idx] = entry.Value
	}
}

// scrubGitConfigKeys deletes every pre-existing GIT_CONFIG_COUNT/KEY_n/VALUE_n entry already in
// target, matched case-insensitively. Append always (re)writes the canonical uppercase spelling
// below; environment variable names are case-insensitive on Windows, so a stale lower- or
// mixed-case variant left behind in target (e.g. a fixture's own git_config_count, or a prior
// Append call in a case-mismatched form) would otherwise still be exported alongside the
// canonical keys this call just wrote -- via t.Setenv, both names resolve to the same underlying
// variable on Windows, so whichever value was set last wins, silently reintroducing the very
// clobbering bug Append exists to prevent.
func scrubGitConfigKeys(target map[string]string) {
	for key := range target {
		upper := strings.ToUpper(key)
		if upper == "GIT_CONFIG_COUNT" || strings.HasPrefix(upper, "GIT_CONFIG_KEY_") || strings.HasPrefix(upper, "GIT_CONFIG_VALUE_") {
			delete(target, key)
		}
	}
}

// readEntries extracts any existing GIT_CONFIG_COUNT/KEY_n/VALUE_n entries
// from an environment slice of "KEY=VALUE" pairs (e.g. os.Environ()).
//
// Environment variable names are case-insensitive on Windows, so os.Environ()
// (and tc.Env in the test harness) can surface lower- or mixed-case
// GIT_CONFIG_* names. The lookup keys are normalized to uppercase before
// matching so those entries are still found; the values themselves are left
// untouched.
func readEntries(env []string) []GitConfigEntry {
	lookup := make(map[string]string, len(env))
	for _, kv := range env {
		key, value, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		lookup[strings.ToUpper(key)] = value
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
