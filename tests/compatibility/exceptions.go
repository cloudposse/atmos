package main

import (
	"sort"
	"strings"
)

// The migration review explicitly dropped BoltDB compatibility on 2026-09-16.
// Keep these three cases as rejection tests instead of skipping them. This is
// deliberately an exact case list, not an exemption for other datasources.
var removedBoltDBCases = map[string]bool{
	"boltdb-json":    true,
	"boltdb-plain":   true,
	"boltdb-missing": true,
}

// rejectsBoltDB defines the accepted replacement contract: the CLI rejects the
// removed scheme with exit 1, identifies that scheme in its diagnostic, and
// produces no output, generated files, or service requests. Diagnostic wrapping
// and migration warnings are allowed to differ for this removed feature only.
func rejectsBoltDB(o observation) bool {
	return o.ExitCode == 1 &&
		canonical(o.Stdout) == canonical(map[string]any{"text": ""}) &&
		len(o.Files) == 0 && len(o.FileNotices) == 0 && len(o.Requests) == 0 &&
		strings.Contains(o.Stderr, `no filesystem registered for scheme "boltdb"`)
}

// classifyDifferences applies reviewed exceptions only to candidate comparisons.
// Old-source collection, stability checks and replay always use strict compare.
func classifyDifferences(expected, actual map[string]observation) ([]string, []string) {
	changed := map[string]bool{}
	for _, name := range differences(expected, actual) {
		changed[name] = true
	}
	approved := []string{}
	for name := range removedBoltDBCases {
		if _, exists := expected[name]; !exists {
			continue
		}
		candidate, exists := actual[name]
		if exists && rejectsBoltDB(candidate) {
			if changed[name] {
				approved = append(approved, name)
				delete(changed, name)
			}
		} else {
			// A restored successful read (even identical to the old observation) or a
			// crash is not the approved removal behavior. Fail the rejection contract.
			changed[name] = true
		}
	}
	unapproved := []string{}
	for name := range changed {
		unapproved = append(unapproved, name)
	}
	sort.Strings(approved)
	sort.Strings(unapproved)
	return approved, unapproved
}
