package acceptance

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
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
	workflow := `shard: [1, 2, 3]
name: ${{ matrix.check }}
check: ["Acceptance Tests (linux)", "Acceptance Tests (macos)", "Acceptance Tests (windows)"]
run: go test ./tests -run '^TestTerraformRegistryCache$'
`
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

func TestParseTarget(t *testing.T) {
	t.Parallel()

	for _, target := range []Target{TargetLinux, TargetMacOS, TargetWindows} {
		got, err := ParseTarget(string(target))
		if err != nil {
			t.Fatalf("parse target %q: %v", target, err)
		}
		if got != target {
			t.Fatalf("parse target %q = %q, want %q", target, got, target)
		}
	}

	if _, err := ParseTarget("solaris"); err == nil {
		t.Fatal("expected an error for an unknown target")
	}
}

func TestParseShard(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		index   string
		count   string
		want    Shard
		wantErr bool
	}{
		{name: "valid", index: "3", count: "10", want: Shard{Index: 3, Count: 10}},
		{name: "index not a number", index: "x", count: "10", wantErr: true},
		{name: "count not a number", index: "3", count: "x", wantErr: true},
		{name: "count zero", index: "1", count: "0", wantErr: true},
		{name: "index zero", index: "0", count: "10", wantErr: true},
		{name: "index exceeds count", index: "11", count: "10", wantErr: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseShard(testCase.index, testCase.count)
			if testCase.wantErr {
				if err == nil {
					t.Fatalf("parse shard %s/%s: expected an error", testCase.index, testCase.count)
				}
				return
			}
			if err != nil {
				t.Fatalf("parse shard %s/%s: %v", testCase.index, testCase.count, err)
			}
			if got != testCase.want {
				t.Fatalf("parse shard %s/%s = %#v, want %#v", testCase.index, testCase.count, got, testCase.want)
			}
		})
	}
}

func TestNonEmptyLines(t *testing.T) {
	t.Parallel()

	got := nonEmptyLines("a\r\n\r\n  b  \nc\n\n")
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("nonEmptyLines() = %#v, want %#v", got, want)
	}

	if got := nonEmptyLines("   \n\n  "); len(got) != 0 {
		t.Fatalf("nonEmptyLines() of blank input = %#v, want empty", got)
	}
}

func TestSourceTestArgs(t *testing.T) {
	t.Parallel()

	shard1 := sourceTestArgs(Shard{Index: 1, Count: 10})
	want1 := []string{"-skip=^" + RegistryTest + "$"}
	if !reflect.DeepEqual(shard1, want1) {
		t.Fatalf("sourceTestArgs(shard 1) = %#v, want %#v", shard1, want1)
	}

	shard2 := sourceTestArgs(Shard{Index: 2, Count: 10})
	want2 := []string{"-run=^" + CLICommandsTest + "$", "-skip=^" + RegistryTest + "$"}
	if !reflect.DeepEqual(shard2, want2) {
		t.Fatalf("sourceTestArgs(shard 2) = %#v, want %#v", shard2, want2)
	}
}

// TestListPackagesRunsGoList exercises listPackages against a tiny standalone
// fixture module instead of this repository, keeping the real `go list`
// subprocess call (this package's established convention, see
// TestGoCommandEnvironmentDisablesCGO) fast and independent of atmos's own
// package graph.
func TestListPackagesRunsGoList(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/fixture\n\ngo 1.21\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "sub.go"), []byte("package sub\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	packages, err := listPackages(t.Context(), newCommandRunner(), root)
	if err != nil {
		t.Fatalf("list packages: %v", err)
	}
	want := []string{"example.com/fixture", "example.com/fixture/sub"}
	if !reflect.DeepEqual(packages, want) {
		t.Fatalf("listPackages() = %#v, want %#v", packages, want)
	}
}

func TestListRunnableTests(t *testing.T) {
	t.Parallel()

	binary := buildFixtureTestBinary(t, []string{"TestBeta", "TestAlpha", "Example_usage", "BenchmarkSkipped"})
	tests, err := listRunnableTests(t.Context(), newCommandRunner(), t.TempDir(), binary)
	if err != nil {
		t.Fatalf("list runnable tests: %v", err)
	}
	want := []string{"Example_usage", "TestAlpha", "TestBeta"}
	if !reflect.DeepEqual(tests, want) {
		t.Fatalf("listRunnableTests() = %#v, want %#v", tests, want)
	}
}

func TestListPackagesPropagatesGoListErrors(t *testing.T) {
	t.Parallel()

	// A directory with no go.mod anywhere in its ancestry (t.TempDir() is
	// under the OS temp root, not this repo) makes `go list ./...` fail.
	if _, err := listPackages(t.Context(), newCommandRunner(), t.TempDir()); err == nil {
		t.Fatal("expected an error listing packages outside any Go module")
	}
}

func TestGoCommandEnvironmentDisablesCGO(t *testing.T) {
	t.Setenv("CGO_ENABLED", "1")

	output, err := newCommandRunner().output(t.Context(), t.TempDir(), goCommandEnvironment(), "go", "env", "CGO_ENABLED")
	if err != nil {
		t.Fatalf("read Go environment: %v", err)
	}
	if got := strings.TrimSpace(output); got != "0" {
		t.Fatalf("CGO_ENABLED = %q, want 0", got)
	}
}
