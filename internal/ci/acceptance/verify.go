package acceptance

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var workflowShardPattern = regexp.MustCompile(`(?m)^\s*shard:\s*\[([^]]+)]\s*$`)

// Verify checks that the repository's packages, tests, and workflow matrix are assigned exactly once.
func Verify(ctx context.Context, repoRoot string, target Target, shardCount int, binaryDir string) error {
	if shardCount < 1 {
		return fmt.Errorf("%w: shard count must be positive", errInvalidConfiguration)
	}
	runner := newCommandRunner()
	allPackages, err := listPackages(ctx, runner, repoRoot)
	if err != nil {
		return err
	}
	expected := expectedPackages(allPackages, target)
	assigned := assignedPackages(allPackages, target, shardCount)
	if err := verifyExactAssignment("packages", expected, assigned); err != nil {
		return err
	}

	binaryDir = absoluteFromRoot(repoRoot, defaultValue(binaryDir, "build"))
	if err := requireFile(filepath.Join(binaryDir, "cmd.test"+binaryExtension(target))); err != nil {
		return err
	}
	if target == TargetWindows {
		if err := verifyWindowsTestRoutes(ctx, runner, repoRoot, binaryDir, shardCount); err != nil {
			return err
		}
	}
	if err := verifyWorkflow(repoRoot, shardCount); err != nil {
		return err
	}
	return writeStatus("Verified %d package assignments across %d shard(s) for %s\n", len(expected), shardCount, target)
}

func expectedPackages(all []string, target Target) []string {
	expected := make([]string, 0, len(all))
	for _, pkg := range all {
		switch pkg {
		case TestsPackage, CmdPackage:
			continue
		case ExecPackage:
			if target != TargetWindows {
				expected = append(expected, pkg)
			}
		default:
			expected = append(expected, pkg)
		}
	}
	return expected
}

func assignedPackages(all []string, target Target, shardCount int) []string {
	assigned := make([]string, 0, len(all))
	for index := 1; index <= shardCount; index++ {
		assigned = append(assigned, shardPackages(all, target, Shard{Index: index, Count: shardCount})...)
	}
	return assigned
}

func binaryExtension(target Target) string {
	if target == TargetWindows {
		return ".exe"
	}
	return ""
}

func verifyWindowsTestRoutes(ctx context.Context, runner commandRunner, repoRoot, binaryDir string, shardCount int) error {
	if err := verifyWindowsTestsPackage(ctx, runner, repoRoot, binaryDir, shardCount); err != nil {
		return err
	}
	return verifyWindowsExecPackage(ctx, runner, repoRoot, binaryDir, shardCount)
}

func verifyWindowsTestsPackage(ctx context.Context, runner commandRunner, repoRoot, binaryDir string, shardCount int) error {
	testsBinary := filepath.Join(binaryDir, "tests.test.exe")
	if err := requireFile(testsBinary); err != nil {
		return err
	}
	tests, err := listRunnableTests(ctx, runner, filepath.Join(repoRoot, "tests"), testsBinary)
	if err != nil {
		return err
	}
	if !contains(tests, CLICommandsTest) || !contains(tests, RegistryTest) {
		return fmt.Errorf("%w: tests binary must contain %s and %s", errShardPlan, CLICommandsTest, RegistryTest)
	}
	expectedTests := without(tests, CLICommandsTest, RegistryTest)
	assigned := make([]string, 0, len(expectedTests))
	for index := 1; index <= shardCount; index++ {
		route := windowsTestsForShard(tests, Shard{Index: index, Count: shardCount})
		if len(route) == 0 || route[0] != CLICommandsTest {
			return fmt.Errorf("%w: Windows tests route %d does not include %s", errShardPlan, index, CLICommandsTest)
		}
		assigned = append(assigned, route[1:]...)
	}
	return verifyExactAssignment("tests package top-level tests", expectedTests, assigned)
}

func verifyWindowsExecPackage(ctx context.Context, runner commandRunner, repoRoot, binaryDir string, shardCount int) error {
	execBinary := filepath.Join(binaryDir, "internal-exec.test.exe")
	if err := requireFile(execBinary); err != nil {
		return err
	}
	execTests, err := listRunnableTests(ctx, runner, filepath.Join(repoRoot, "internal", "exec"), execBinary)
	if err != nil {
		return err
	}
	assignedExec := make([]string, 0, len(execTests))
	for index := 1; index <= shardCount; index++ {
		assignedExec = append(assignedExec, assignedTests(execTests, Shard{Index: index, Count: shardCount}, nil)...)
	}
	return verifyExactAssignment("internal/exec top-level tests", execTests, assignedExec)
}

func verifyExactAssignment(kind string, expected, assigned []string) error {
	want := append([]string(nil), expected...)
	got := append([]string(nil), assigned...)
	sort.Strings(want)
	sort.Strings(got)
	duplicates := make([]string, 0)
	for index := 1; index < len(got); index++ {
		if got[index] == got[index-1] && (len(duplicates) == 0 || duplicates[len(duplicates)-1] != got[index]) {
			duplicates = append(duplicates, got[index])
		}
	}
	if len(duplicates) > 0 {
		return fmt.Errorf("%w: %s assigned more than once: %s", errShardPlan, kind, strings.Join(duplicates, ", "))
	}
	wantSet := sliceSet(want)
	gotSet := sliceSet(got)
	missing := setDifference(wantSet, gotSet)
	unexpected := setDifference(gotSet, wantSet)
	if len(missing) > 0 || len(unexpected) > 0 {
		return fmt.Errorf("%w: %s missing=[%s] unexpected=[%s]",
			errShardPlan, kind, strings.Join(missing, ", "), strings.Join(unexpected, ", "))
	}
	return nil
}

func verifyWorkflow(repoRoot string, shardCount int) error {
	workflowPath := filepath.Join(repoRoot, ".github", "workflows", "test.yml")
	content, err := os.ReadFile(workflowPath)
	if err != nil {
		return fmt.Errorf("read test workflow: %w", err)
	}
	matches := workflowShardPattern.FindAllSubmatch(content, -1)
	if len(matches) != 1 {
		return fmt.Errorf("%w: could not identify exactly one explicit workflow shard matrix", errShardPlan)
	}
	values := strings.Split(string(matches[0][1]), ",")
	if len(values) != shardCount {
		return fmt.Errorf("%w: workflow has %d shards; expected %d", errShardPlan, len(values), shardCount)
	}
	for index, value := range values {
		actual, parseErr := strconv.Atoi(strings.TrimSpace(value))
		if parseErr != nil || actual != index+1 {
			return fmt.Errorf("%w: workflow shard position %d contains %q", errShardPlan, index+1, strings.TrimSpace(value))
		}
	}
	if !strings.Contains(string(content), "run: go test ./tests -run '^"+RegistryTest+"$'") {
		return fmt.Errorf("%w: %s has no dedicated workflow route", errShardPlan, RegistryTest)
	}
	return nil
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func without(values []string, excluded ...string) []string {
	excludedSet := sliceSet(excluded)
	result := make([]string, 0, len(values))
	for _, value := range values {
		if !excludedSet[value] {
			result = append(result, value)
		}
	}
	return result
}

func sliceSet(values []string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}

func setDifference(left, right map[string]bool) []string {
	result := make([]string, 0)
	for value := range left {
		if !right[value] {
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}
