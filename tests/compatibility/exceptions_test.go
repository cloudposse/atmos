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
