package acceptance

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	TestsPackage       = "github.com/cloudposse/atmos/tests"
	CmdPackage         = "github.com/cloudposse/atmos/cmd"
	ExecPackage        = "github.com/cloudposse/atmos/internal/exec"
	TestHelpersPackage = "github.com/cloudposse/atmos/tests/testhelpers"

	CLICommandsTest = "TestCLICommands"
	RegistryTest    = "TestTerraformRegistryCache"
)

var runnableTestPattern = regexp.MustCompile(`^(Test|Example|Fuzz)`) // Benchmarks do not run by default.

// Target identifies the operating system whose acceptance plan is being built.
type Target string

const (
	TargetLinux   Target = "linux"
	TargetMacOS   Target = "macos"
	TargetWindows Target = "windows"
)

func ParseTarget(value string) (Target, error) {
	target := Target(value)
	switch target {
	case TargetLinux, TargetMacOS, TargetWindows:
		return target, nil
	default:
		return "", fmt.Errorf("%w: target %q", errInvalidConfiguration, value)
	}
}

type Shard struct {
	Index int
	Count int
}

func ParseShard(indexValue, countValue string) (Shard, error) {
	index, err := strconv.Atoi(indexValue)
	if err != nil {
		return Shard{}, fmt.Errorf("%w: shard %s/%s", errInvalidConfiguration, indexValue, countValue)
	}
	count, err := strconv.Atoi(countValue)
	if err != nil || count < 1 || index < 1 || index > count {
		return Shard{}, fmt.Errorf("%w: shard %s/%s", errInvalidConfiguration, indexValue, countValue)
	}
	return Shard{Index: index, Count: count}, nil
}

func listPackages(ctx context.Context, runner commandRunner, repoRoot string) ([]string, error) {
	output, err := runner.output(ctx, repoRoot, nil, "go", "list", "./...")
	if err != nil {
		return nil, err
	}
	return nonEmptyLines(output), nil
}

func generalPackages(all []string) []string {
	packages := make([]string, 0, len(all))
	for _, pkg := range all {
		switch pkg {
		case TestsPackage, CmdPackage, ExecPackage, TestHelpersPackage:
			continue
		default:
			packages = append(packages, pkg)
		}
	}
	return packages
}

func shardPackages(all []string, target Target, shard Shard) []string {
	general := generalPackages(all)
	packages := make([]string, 0, len(general)/shard.Count+2)
	for index, pkg := range general {
		// Preserve the historical allocation: the first eligible package goes to
		// shard 2 because the Bash planner incremented its counter before modulo.
		if (index+1)%shard.Count == shard.Index-1 {
			packages = append(packages, pkg)
		}
	}
	execShard := min(2, shard.Count)
	helperShard := min(3, shard.Count)
	if target != TargetWindows && shard.Index == execShard {
		packages = append(packages, ExecPackage)
	}
	if shard.Index == helperShard {
		packages = append(packages, TestHelpersPackage)
	}
	return packages
}

func sourceTestArgs(shard Shard) []string {
	if shard.Index == 1 {
		return []string{"-skip=^" + RegistryTest + "$"}
	}
	return []string{"-run=^" + CLICommandsTest + "$", "-skip=^" + RegistryTest + "$"}
}

func listRunnableTests(ctx context.Context, runner commandRunner, dir, binary string) ([]string, error) {
	output, err := runner.output(ctx, dir, nil, binary, "-test.list", `^(Test|Example|Fuzz)`)
	if err != nil {
		return nil, err
	}
	tests := make([]string, 0)
	for _, line := range nonEmptyLines(output) {
		if runnableTestPattern.MatchString(line) {
			tests = append(tests, line)
		}
	}
	sort.Strings(tests)
	return tests, nil
}

func assignedTests(all []string, shard Shard, excluded map[string]bool) []string {
	eligible := make([]string, 0, len(all))
	for _, test := range all {
		if !excluded[test] {
			eligible = append(eligible, test)
		}
	}
	sort.Strings(eligible)
	assigned := make([]string, 0, len(eligible)/shard.Count+1)
	for index, test := range eligible {
		if index%shard.Count == shard.Index-1 {
			assigned = append(assigned, test)
		}
	}
	return assigned
}

func testRunPattern(tests []string) string {
	quoted := make([]string, len(tests))
	for index, test := range tests {
		quoted[index] = regexp.QuoteMeta(test)
	}
	return "^(?:" + strings.Join(quoted, "|") + ")$"
}

func windowsTestsForShard(all []string, shard Shard) []string {
	assigned := assignedTests(all, shard, map[string]bool{
		CLICommandsTest: true,
		RegistryTest:    true,
	})
	return append([]string{CLICommandsTest}, assigned...)
}

func nonEmptyLines(value string) []string {
	lines := strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}
