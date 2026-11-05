package benchmarks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Each stack has one root, nine branches, and ninety leaves. A catalog edit
// affects all 1,000 instances, exercising shared stack resolution, reverse-index
// lookups, and recursive expansion without requiring an Atmos Pro upload.
func dependentsFixture(tb testing.TB) fixture {
	tb.Helper()
	f := makeFixture(tb, 1000, "none")
	for stack := range 10 {
		path := filepath.Join(f.root, "stacks", "deploy", fmt.Sprintf("s%03d.yaml", stack))
		data, err := os.ReadFile(path)
		if err != nil {
			tb.Fatal(err)
		}
		content := string(data)
		for component := 1; component < 100; component++ {
			parent := dependentParent(component)
			name := fmt.Sprintf("        name: app-%03d\n", component)
			dependency := fmt.Sprintf("      dependencies:\n        components:\n          - component: app-%03d\n            stack: s%03d\n", parent, stack)
			content = strings.Replace(content, name, name+dependency, 1)
		}
		writeFixture(tb, path, content)
	}
	fixtureGit(tb, f.root, "add", ".")
	fixtureGit(tb, f.root, "commit", "-qm", "dependency tree")
	fixtureGit(tb, f.root, "worktree", "remove", "--force", f.base)
	fixtureGit(tb, f.root, "worktree", "add", "--detach", f.base, "HEAD")
	path := filepath.Join(f.root, "stacks", "catalog", "shared.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		tb.Fatal(err)
	}
	writeFixture(tb, path, strings.Replace(string(data), "generation: 1", "generation: 2", 1))
	fixtureGit(tb, f.root, "add", ".")
	fixtureGit(tb, f.root, "commit", "-qm", "catalog change")
	return f
}

func dependentParent(component int) int {
	if component < 10 {
		return 0
	}
	return 1 + (component-10)%9
}

func benchmarkDependentsCLI(b *testing.B, binary string) {
	b.Helper()
	f := dependentsFixture(b)
	args := []string{"describe", "affected", "--repo-path", f.base, "--include-dependents"}
	b.ResetTimer()
	for range b.N {
		b.StopTimer()
		home := b.TempDir()
		b.StartTimer()
		data := runCLI(b, binary, f, home, args)
		b.StopTimer()
		validateCLI(b, data, f.instances, "DescribeAffected")
		var rows []dependentRow
		if err := json.Unmarshal(data, &rows); err != nil {
			b.Fatal(err)
		}
		for _, row := range rows {
			validateDependentTree(b, row)
		}
		b.StartTimer()
	}
}

type dependentRow struct {
	Stack      string         `json:"stack"`
	Component  string         `json:"component"`
	Dependents []dependentRow `json:"dependents"`
}

func validateDependentTree(tb testing.TB, row dependentRow) {
	tb.Helper()
	component, err := strconv.Atoi(strings.TrimPrefix(row.Component, "app-"))
	if err != nil || component < 0 || component >= 100 {
		tb.Fatalf("unexpected dependent component: %s", row.Component)
	}
	want := 0
	if component == 0 {
		want = 9
	} else if component < 10 {
		want = 10
	}
	if len(row.Dependents) != want {
		tb.Fatalf("%s/%s has %d dependents, want %d", row.Stack, row.Component, len(row.Dependents), want)
	}
	seen := map[string]bool{}
	for _, child := range row.Dependents {
		id, err := strconv.Atoi(strings.TrimPrefix(child.Component, "app-"))
		if err != nil || id <= component || id >= 100 || dependentParent(id) != component || child.Stack != row.Stack || seen[child.Component] {
			tb.Fatalf("unexpected dependent %s/%s under %s/%s", child.Stack, child.Component, row.Stack, row.Component)
		}
		seen[child.Component] = true
		validateDependentTree(tb, child)
	}
}
