package benchmarks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Consumers use one lookup kind each so a stale cache in one path cannot be
// hidden by another path detecting the same target change. All targets are
// static: this exercises masking and deferred describes without Terraform or AWS.
func functionsFixture(tb testing.TB, change string) fixture {
	tb.Helper()
	f := makeFixture(tb, 200, "none")
	for stack := range 2 {
		path := filepath.Join(f.root, "stacks", "deploy", fmt.Sprintf("s%03d.yaml", stack))
		data, err := os.ReadFile(path)
		if err != nil {
			tb.Fatal(err)
		}
		content := string(data)
		for component := range 100 {
			lookup := functionLookup(stack*100 + component)
			name := fmt.Sprintf("        name: app-%03d\n", component)
			values := fmt.Sprintf("        lookup_a: %s\n        lookup_b: %s\n        lookup_c: %s\n", lookup, lookup, lookup)
			dependency := "      dependencies:\n        components:\n          - component: app-target\n            stack: s000\n"
			content = strings.Replace(content, name, name+values+dependency, 1)
		}
		if stack == 0 {
			content += `    app-target:
      metadata:
        component: mock
      remote_state_backend_type: static
      remote_state_backend:
        static:
          value: baseline-value
      vars:
        version: baseline-value
`
		}
		writeFixture(tb, path, content)
	}
	fixtureGit(tb, f.root, "add", ".")
	fixtureGit(tb, f.root, "commit", "-qm", "repeated component functions")
	fixtureGit(tb, f.root, "worktree", "remove", "--force", f.base)
	fixtureGit(tb, f.root, "worktree", "add", "--detach", f.base, "HEAD")
	if change != "none" {
		path := filepath.Join(f.root, "stacks", "deploy", "s000.yaml")
		data, err := os.ReadFile(path)
		if err != nil {
			tb.Fatal(err)
		}
		writeFixture(tb, path, strings.ReplaceAll(string(data), "baseline-value", "changed-value"))
		fixtureGit(tb, f.root, "add", ".")
		fixtureGit(tb, f.root, "commit", "-qm", "change referenced static value")
	}
	return f
}

func functionLookup(component int) string {
	switch component % 3 {
	case 0:
		return "!terraform.output app-target s000 value"
	case 1:
		return "!terraform.state app-target s000 value"
	default:
		return `'{{ (atmos.Component "app-target" "s000").outputs.value }}'`
	}
}

// TestFunctionsCLI validates all workload results without collecting timings.
func TestFunctionsCLI(t *testing.T) {
	binary := os.Getenv("ATMOS_BENCH_BINARY")
	if binary == "" {
		t.Skip("set ATMOS_BENCH_BINARY to validate the CLI fixture")
	}
	for _, change := range []string{"none", "target", "dependents"} {
		t.Run(change, func(t *testing.T) {
			f := functionsFixture(t, change)
			validateFunctionRepositories(t, binary, f, change)
			data := runCLI(t, binary, f, t.TempDir(), functionAffectedArgs(f, change))
			validateFunctionAffected(t, data, change)
		})
	}
}

func functionAffectedArgs(f fixture, change string) []string {
	args := []string{"describe", "affected", "--repo-path", f.base, "--process-functions=true"}
	if change == "dependents" {
		args = append(args, "--include-dependents")
	}
	return args
}

func benchmarkFunctionsCLI(b *testing.B, binary, change string) {
	b.Helper()
	f := functionsFixture(b, change)
	args := functionAffectedArgs(f, change)
	// Check actual resolved values in both repositories outside timing. This
	// also guards against accidentally benchmarking skipped function processing.
	validateFunctionRepositories(b, binary, f, change)
	b.ResetTimer()
	for range b.N {
		b.StopTimer()
		home := b.TempDir()
		b.StartTimer()
		data := runCLI(b, binary, f, home, args)
		b.StopTimer()
		validateFunctionAffected(b, data, change)
		b.StartTimer()
	}
}

func validateFunctionRepositories(tb testing.TB, binary string, f fixture, change string) {
	tb.Helper()
	expected := "baseline-value"
	if change != "none" {
		expected = "changed-value"
	}
	data := runCLI(tb, binary, f, tb.TempDir(), []string{"describe", "stacks", "--process-functions=true"})
	validateFunctionValues(tb, data, expected)
	f.root = f.base
	data = runCLI(tb, binary, f, tb.TempDir(), []string{"describe", "stacks", "--process-functions=true"})
	validateFunctionValues(tb, data, "baseline-value")
}

func validateFunctionValues(tb testing.TB, data []byte, expected string) {
	tb.Helper()
	var stacks map[string]struct {
		Components struct {
			Terraform map[string]struct{ Vars map[string]any } `json:"terraform"`
		} `json:"components"`
	}
	if err := json.Unmarshal(data, &stacks); err != nil {
		tb.Fatal(err)
	}
	for stack := range 2 {
		components := stacks[fmt.Sprintf("s%03d", stack)].Components.Terraform
		for component := range 100 {
			name := fmt.Sprintf("app-%03d", component)
			for _, key := range []string{"lookup_a", "lookup_b", "lookup_c"} {
				if got := components[name].Vars[key]; got != expected {
					tb.Fatalf("s%03d/%s %s = %v, want %s", stack, name, key, got, expected)
				}
			}
		}
	}
}

func validateFunctionAffected(tb testing.TB, data []byte, change string) {
	tb.Helper()
	var rows []dependentRow
	if err := json.Unmarshal(data, &rows); err != nil {
		tb.Fatal(err)
	}
	want := map[string]bool{}
	if change != "none" {
		want["s000/app-target"] = true
		for stack := range 2 {
			for component := range 100 {
				want[fmt.Sprintf("s%03d/app-%03d", stack, component)] = true
			}
		}
	}
	if len(rows) != len(want) {
		tb.Fatalf("affected instances = %d, want %d\n%.1000s", len(rows), len(want), data)
	}
	for _, row := range rows {
		key := row.Stack + "/" + row.Component
		if !want[key] {
			tb.Fatalf("unexpected/duplicate affected instance: %s", key)
		}
		delete(want, key)
		validateFunctionDependents(tb, row, change)
	}
}

func validateFunctionDependents(tb testing.TB, row dependentRow, change string) {
	tb.Helper()
	want := map[string]bool{}
	if change == "dependents" && row.Component == "app-target" {
		for stack := range 2 {
			for component := range 100 {
				want[fmt.Sprintf("s%03d/app-%03d", stack, component)] = true
			}
		}
	}
	if len(row.Dependents) != len(want) {
		tb.Fatalf("%s/%s dependents = %d, want %d", row.Stack, row.Component, len(row.Dependents), len(want))
	}
	for _, child := range row.Dependents {
		key := child.Stack + "/" + child.Component
		if !want[key] || len(child.Dependents) != 0 {
			tb.Fatalf("unexpected/duplicate dependent: %s", key)
		}
		delete(want, key)
	}
}
