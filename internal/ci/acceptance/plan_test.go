package acceptance

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestShardPackagesCompleteness(t *testing.T) {
	t.Parallel()

	all := []string{
		TestsPackage,
		CmdPackage,
		ExecPackage,
		TestHelpersPackage,
		"example.com/package/a",
		"example.com/package/b",
		"example.com/package/c",
		"example.com/package/d",
	}

	for _, target := range []Target{TargetLinux, TargetMacOS, TargetWindows} {
		for _, shardCount := range []int{1, 2, 3, 10, 11} {
			assigned := make([]string, 0)
			for shard := 1; shard <= shardCount; shard++ {
				assigned = append(assigned, shardPackages(all, target, Shard{Index: shard, Count: shardCount})...)
			}
			expected := []string{
				"example.com/package/a",
				"example.com/package/b",
				"example.com/package/c",
				"example.com/package/d",
				TestHelpersPackage,
			}
			if target != TargetWindows {
				expected = append(expected, ExecPackage)
			}
			if err := verifyExactAssignment("packages", expected, assigned); err != nil {
				t.Fatalf("target %s shards %d: %v", target, shardCount, err)
			}
		}
	}
}

func TestWindowsTestRoutesAreCompleteAndDeterministic(t *testing.T) {
	t.Parallel()

	tests := []string{
		RegistryTest,
		"TestZulu",
		CLICommandsTest,
		"ExampleUsage",
		"FuzzParser",
		"TestAlpha",
		"TestMiddle",
	}
	wantExtras := without(tests, RegistryTest, CLICommandsTest)
	for _, shardCount := range []int{1, 2, 3, 10, 11} {
		assigned := make([]string, 0, len(wantExtras))
		for shard := 1; shard <= shardCount; shard++ {
			route := windowsTestsForShard(tests, Shard{Index: shard, Count: shardCount})
			if len(route) == 0 || route[0] != CLICommandsTest {
				t.Fatalf("shard %d/%d does not run %s: %v", shard, shardCount, CLICommandsTest, route)
			}
			if contains(route, RegistryTest) {
				t.Fatalf("shard %d/%d unexpectedly runs %s", shard, shardCount, RegistryTest)
			}
			assigned = append(assigned, route[1:]...)
		}
		if err := verifyExactAssignment("tests", wantExtras, assigned); err != nil {
			t.Fatalf("shards %d: %v", shardCount, err)
		}
	}

	reversed := append([]string(nil), tests...)
	for left, right := 0, len(reversed)-1; left < right; left, right = left+1, right-1 {
		reversed[left], reversed[right] = reversed[right], reversed[left]
	}
	if !reflect.DeepEqual(
		windowsTestsForShard(tests, Shard{Index: 2, Count: 3}),
		windowsTestsForShard(reversed, Shard{Index: 2, Count: 3}),
	) {
		t.Fatal("Windows test assignment depends on input order")
	}
}

func TestTestRunPatternMatchesOnlyAssignedTests(t *testing.T) {
	t.Parallel()

	pattern := testRunPattern([]string{"TestOne", "TestWith[Meta]", "ExampleUsage"})
	for _, test := range []string{"TestOne", "TestWith[Meta]", "ExampleUsage"} {
		if !regexpMustCompile(t, pattern).MatchString(test) {
			t.Fatalf("pattern %q does not match %q", pattern, test)
		}
	}
	for _, test := range []string{"TestOne/Subtest", "TestWithM", RegistryTest} {
		if regexpMustCompile(t, pattern).MatchString(test) {
			t.Fatalf("pattern %q unexpectedly matches %q", pattern, test)
		}
	}
}

func TestVerifyWorkflow(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	workflowDir := filepath.Join(root, ".github", "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatal(err)
	}
	workflow := "shard: [1, 2, 3]\nrun: go test ./tests -run '^TestTerraformRegistryCache$'\n"
	if err := os.WriteFile(filepath.Join(workflowDir, "test.yml"), []byte(workflow), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := verifyWorkflow(root, 3); err != nil {
		t.Fatalf("verify valid workflow: %v", err)
	}
	if err := verifyWorkflow(root, 4); err == nil {
		t.Fatal("expected shard-count mismatch")
	}
}
