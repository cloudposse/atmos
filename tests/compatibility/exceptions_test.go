package main

import (
	"io"
	"reflect"
	"testing"
)

func boltRejection() observation {
	return observation{ExitCode: 1, Stdout: map[string]any{"text": ""}, Stderr: `no filesystem registered for scheme "boltdb"`}
}

func TestBoltDBRemovalIsAnExplicitCandidateException(t *testing.T) {
	for name := range removedBoltDBCases {
		old := map[string]observation{name: {Stdout: map[string]any{"json": "old-value"}}}
		candidate := map[string]observation{name: boltRejection()}
		approved, unapproved := classifyDifferences(old, candidate)
		if !reflect.DeepEqual(approved, []string{name}) || len(unapproved) != 0 {
			t.Fatalf("%s: %v %v", name, approved, unapproved)
		}
		if compareMigration(io.Discard, old, candidate) {
			t.Fatal("approved removal rejected")
		}
		if !compare(io.Discard, old, candidate) {
			t.Fatal("exception leaked into old baseline checks")
		}
	}
}

func TestBoltDBExceptionDoesNotHideOtherFailures(t *testing.T) {
	changes := map[string]func(*observation){
		"success":         func(o *observation) { o.ExitCode = 0 },
		"crash":           func(o *observation) { o.ExitCode = 2 },
		"stdout":          func(o *observation) { o.Stdout = map[string]any{"text": "unexpected"} },
		"wrong error":     func(o *observation) { o.Stderr = "permission denied" },
		"service request": func(o *observation) { o.Requests = []map[string]any{{"path": "/unexpected"}} },
		"file":            func(o *observation) { o.Files = map[string]any{"output": "unexpected"} },
		"notice":          func(o *observation) { o.FileNotices = []string{"Created output"} },
	}
	old := map[string]observation{"boltdb-json": {}}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			candidate := boltRejection()
			change(&candidate)
			approved, unapproved := classifyDifferences(old, map[string]observation{"boltdb-json": candidate})
			if len(approved) != 0 || !reflect.DeepEqual(unapproved, []string{"boltdb-json"}) {
				t.Fatalf("%v %v", approved, unapproved)
			}
		})
	}
	for _, candidate := range []map[string]observation{{}, old} {
		if !compareMigration(io.Discard, old, candidate) {
			t.Fatal("missing case or old success accepted as removal")
		}
	}
	unrelated := map[string]observation{"ssm-string": boltRejection()}
	if !compareMigration(io.Discard, map[string]observation{"ssm-string": {}}, unrelated) {
		t.Fatal("exception applied to unrelated datasource")
	}
	if !compareMigration(io.Discard, map[string]observation{}, map[string]observation{"boltdb-json": boltRejection()}) {
		t.Fatal("extra case silently accepted")
	}
}

func TestCandidateComparisonIgnoresStderrOnly(t *testing.T) {
	old := map[string]observation{"conv-bool": {Stdout: map[string]any{"json": "true"}, Stderr: "# Error"}}
	candidate := map[string]observation{"conv-bool": {Stdout: map[string]any{"json": "true"}, Stderr: "WARN shim"}}
	approved, unapproved := classifyDifferences(old, candidate)
	if len(approved) != 0 || len(unapproved) != 0 {
		t.Fatalf("stderr-only difference reported: %v %v", approved, unapproved)
	}
	if !compare(io.Discard, old, candidate) {
		t.Fatal("stderr exemption leaked into old baseline checks")
	}
	candidate["conv-bool"] = observation{ExitCode: 1, Stdout: map[string]any{"json": "true"}}
	if _, unapproved := classifyDifferences(old, candidate); !reflect.DeepEqual(unapproved, []string{"conv-bool"}) {
		t.Fatalf("exit code change hidden: %v", unapproved)
	}
}

func TestApprovedChangesRequireTheRecordedObservation(t *testing.T) {
	changes := approvedChanges()
	if len(changes) == 0 {
		t.Fatal("approved.json has no entries")
	}
	name := "conv-atoi-invalid"
	change, ok := changes[name]
	if !ok {
		t.Fatalf("%s is not approved", name)
	}
	if change.Reason == "" {
		t.Fatalf("%s has no reason", name)
	}
	old := observation{Stdout: map[string]any{"json": map[string]any{"result": "0"}}, Requests: []map[string]any{}}
	recorded := observation{ExitCode: change.ExitCode, Stdout: change.Stdout, Files: change.Files, FileNotices: change.FileNotices, Stderr: "any diagnostic"}

	approved, unapproved := classifyDifferences(map[string]observation{name: old}, map[string]observation{name: recorded})
	if !reflect.DeepEqual(approved, []string{name}) || len(unapproved) != 0 {
		t.Fatalf("recorded change not approved: %v %v", approved, unapproved)
	}

	mutations := map[string]func(*observation){
		"exit code": func(o *observation) { o.ExitCode = 2 },
		"stdout":    func(o *observation) { o.Stdout = map[string]any{"text": "unexpected"} },
		"requests":  func(o *observation) { o.Requests = []map[string]any{{"method": "HEAD"}} },
		"files":     func(o *observation) { o.Files = map[string]any{"output": "unexpected"} },
	}
	for label, mutate := range mutations {
		t.Run(label, func(t *testing.T) {
			candidate := recorded
			mutate(&candidate)
			approved, unapproved := classifyDifferences(map[string]observation{name: old}, map[string]observation{name: candidate})
			if len(approved) != 0 || !reflect.DeepEqual(unapproved, []string{name}) {
				t.Fatalf("%v %v", approved, unapproved)
			}
		})
	}

	// The same observation for a case that is not approved stays blocking.
	if _, unapproved := classifyDifferences(map[string]observation{"conv-bool": old}, map[string]observation{"conv-bool": recorded}); !reflect.DeepEqual(unapproved, []string{"conv-bool"}) {
		t.Fatalf("approval leaked to another case: %v", unapproved)
	}
}

func TestApprovedRequestsReplaceTheOldRequests(t *testing.T) {
	name := "vault-kv1"
	change, ok := approvedChanges()[name]
	if !ok || change.Requests == nil {
		t.Fatalf("%s has no approved requests", name)
	}
	old := observation{ExitCode: change.ExitCode, Stdout: change.Stdout, Requests: []map[string]any{{"method": "GET", "path": "/v1/secret/config"}}}
	recorded := observation{ExitCode: change.ExitCode, Stdout: change.Stdout, Files: change.Files, FileNotices: change.FileNotices, Requests: *change.Requests}

	approved, unapproved := classifyDifferences(map[string]observation{name: old}, map[string]observation{name: recorded})
	if !reflect.DeepEqual(approved, []string{name}) || len(unapproved) != 0 {
		t.Fatalf("recorded requests not approved: %v %v", approved, unapproved)
	}

	other := recorded
	other.Requests = []map[string]any{{"method": "DELETE", "path": "/v1/secret/config"}}
	if _, unapproved := classifyDifferences(map[string]observation{name: old}, map[string]observation{name: other}); !reflect.DeepEqual(unapproved, []string{name}) {
		t.Fatalf("unrecorded requests approved: %v", unapproved)
	}
}
