package tests

import (
	"fmt"
	"reflect"
	"testing"
)

func TestAssignWorkdirsToShardsCompleteness(t *testing.T) {
	testCases := []TestCase{
		{Name: "a-1", Workdir: "fixtures/a"},
		{Name: "a-2", Workdir: "fixtures/a"},
		{Name: "a-3", Workdir: "fixtures/a"},
		{Name: "b-1", Workdir: "fixtures/b"},
		{Name: "b-2", Workdir: "fixtures/b"},
		{Name: "c-1", Workdir: "fixtures/c"},
		{Name: "d-1", Workdir: "fixtures/d"},
	}

	for _, shardCount := range []int{1, 2, 3, 10, 11} {
		t.Run(fmt.Sprintf("shards-%d", shardCount), func(t *testing.T) {
			assignment := assignWorkdirsToShards(testCases, shardCount)
			if len(assignment) != 4 {
				t.Fatalf("assigned %d workdirs, want 4", len(assignment))
			}

			seenCases := 0
			for shard := 1; shard <= shardCount; shard++ {
				for i := range testCases {
					assignedShard, ok := assignment[testCases[i].Workdir]
					if !ok {
						t.Fatalf("workdir %q has no assignment", testCases[i].Workdir)
					}
					if assignedShard < 1 || assignedShard > shardCount {
						t.Fatalf("workdir %q assigned to invalid shard %d/%d", testCases[i].Workdir, assignedShard, shardCount)
					}
					if assignedShard == shard {
						seenCases++
					}
				}
			}
			if seenCases != len(testCases) {
				t.Fatalf("assigned %d test cases, want %d", seenCases, len(testCases))
			}
		})
	}
}

func TestAssignWorkdirsToShardsDeterministic(t *testing.T) {
	forward := []TestCase{
		{Name: "a-1", Workdir: "fixtures/a"},
		{Name: "a-2", Workdir: "fixtures/a"},
		{Name: "b-1", Workdir: "fixtures/b"},
		{Name: "c-1", Workdir: "fixtures/c"},
		{Name: "d-1", Workdir: "fixtures/d"},
	}
	reversed := make([]TestCase, len(forward))
	for i := range forward {
		reversed[len(forward)-1-i] = forward[i]
	}

	forwardAssignment := assignWorkdirsToShards(forward, 3)
	reversedAssignment := assignWorkdirsToShards(reversed, 3)
	if !reflect.DeepEqual(forwardAssignment, reversedAssignment) {
		t.Fatalf("assignment depends on input order:\nforward: %#v\nreverse: %#v", forwardAssignment, reversedAssignment)
	}
}

func TestAssignActualCLIWorkdirsToShardsCompleteness(t *testing.T) {
	testSuite, err := loadTestSuites("test-cases")
	if err != nil {
		t.Fatalf("load test suites: %v", err)
	}

	for _, shardCount := range []int{10, 11} {
		t.Run(fmt.Sprintf("shards-%d", shardCount), func(t *testing.T) {
			assignment := assignWorkdirsToShards(testSuite.Tests, shardCount)
			seenCases := 0
			for shard := 1; shard <= shardCount; shard++ {
				for i := range testSuite.Tests {
					assignedShard, ok := assignment[testSuite.Tests[i].Workdir]
					if !ok {
						t.Fatalf("workdir %q has no assignment", testSuite.Tests[i].Workdir)
					}
					if assignedShard == shard {
						seenCases++
					}
				}
			}
			if seenCases != len(testSuite.Tests) {
				t.Fatalf("assigned %d test cases, want %d", seenCases, len(testSuite.Tests))
			}
		})
	}
}
