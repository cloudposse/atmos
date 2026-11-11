package main

import (
	_ "embed"
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

// approvedJSON records reviewed, intended behavior changes. Each entry is the
// exact new observation for one case (exit code, stdout and generated files)
// and the reason it was approved; anything else that differs stays blocking.
//
//go:embed approved.json
var approvedJSON []byte

// approvedChange is the reviewed candidate behavior for one case. Requests is
// set only when the review approved different service traffic; otherwise the
// candidate's requests must match the old observation.
type approvedChange struct {
	Reason      string            `json:"reason"`
	ExitCode    int               `json:"exit_code"`
	Stdout      map[string]any    `json:"stdout"`
	Files       map[string]any    `json:"files"`
	FileNotices []string          `json:"file_notices"`
	Requests    *[]map[string]any `json:"requests,omitempty"`
}

// approvedChanges decodes approved.json. A malformed file is a harness bug.
func approvedChanges() map[string]approvedChange {
	changes := map[string]approvedChange{}
	if err := decode(approvedJSON, &changes); err != nil {
		panic("approved.json: " + err.Error())
	}
	return changes
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

// matchesApprovedChange reports whether candidate is exactly the reviewed new
// behavior. Unless the review recorded new requests, service requests must
// still match the old observation: approving a rendered value never approves
// different traffic implicitly.
func matchesApprovedChange(old, candidate observation, change approvedChange) bool {
	requests := old.Requests
	if change.Requests != nil {
		requests = *change.Requests
	}
	approved := normalized(observation{
		ExitCode:    change.ExitCode,
		Stdout:      change.Stdout,
		Requests:    requests,
		Files:       change.Files,
		FileNotices: change.FileNotices,
	})
	return canonical(approved) == canonical(normalized(withoutStderr(candidate)))
}

// withoutStderr drops stderr for candidate comparisons. Error formatting,
// template line numbers and deprecation warnings legitimately change between
// releases; exit codes, output, generated files and service requests still
// must match.
func withoutStderr(o observation) observation {
	o.Stderr = ""
	return o
}

// normalized treats absent and empty collections alike, so an approved
// observation compares equal however the runner encoded an empty value.
func normalized(o observation) observation {
	if o.Requests == nil {
		o.Requests = []map[string]any{}
	}
	if o.Files == nil {
		o.Files = map[string]any{}
	}
	if o.FileNotices == nil {
		o.FileNotices = []string{}
	}
	return o
}

// candidateDifferences lists cases whose observations differ when stderr is
// ignored.
func candidateDifferences(expected, actual map[string]observation) []string {
	strip := func(observations map[string]observation) map[string]observation {
		stripped := make(map[string]observation, len(observations))
		for name, o := range observations {
			stripped[name] = normalized(withoutStderr(o))
		}
		return stripped
	}
	return differences(strip(expected), strip(actual))
}

// classifyDifferences applies reviewed exceptions only to candidate comparisons.
// Old-source collection, stability checks and replay always use strict compare.
func classifyDifferences(expected, actual map[string]observation) ([]string, []string) {
	changed := map[string]bool{}
	for _, name := range candidateDifferences(expected, actual) {
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
	for name, change := range approvedChanges() {
		old, oldOK := expected[name]
		candidate, candidateOK := actual[name]
		if changed[name] && oldOK && candidateOK && matchesApprovedChange(old, candidate, change) {
			approved = append(approved, name)
			delete(changed, name)
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

// approvalReason explains why a case's difference is accepted.
func approvalReason(name string) string {
	if removedBoltDBCases[name] {
		return "BoltDB support intentionally removed; unsupported-scheme rejection verified."
	}
	if change, ok := approvedChanges()[name]; ok {
		return change.Reason
	}
	return "approved"
}
