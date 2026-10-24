package benchmarks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Spread the same 200 consumers over 20 caller stacks. Their store requests
// use one fixed target stack and ambient credentials, matching issue #3353.
func storeCallerStacksFixture(tb testing.TB) (fixture, func() int64) {
	tb.Helper()
	f, reads := storeFixture(tb)
	f.ambientAuth = true
	sharedPath := filepath.Join(f.root, "stacks", "catalog", "shared.yaml")
	shared, err := os.ReadFile(sharedPath)
	if err != nil {
		tb.Fatal(err)
	}
	// Plain Go templates still execute when YAML functions are disabled.
	writeFixture(tb, sharedPath, strings.ReplaceAll(string(shared), "!template ", ""))
	for stack := range 2 {
		if err := os.Remove(filepath.Join(f.root, "stacks", "deploy", fmt.Sprintf("s%03d.yaml", stack))); err != nil {
			tb.Fatal(err)
		}
	}
	for stack := range 20 {
		var content strings.Builder
		fmt.Fprintf(&content, "import: [catalog/shared]\nvars:\n  stage: s%03d\ncomponents:\n  terraform:\n", stack)
		for component := range 10 {
			fmt.Fprintf(&content, "    app-%03d:\n      metadata:\n        component: mock\n        inherits: [inherited]\n      vars:\n        name: app-%03d\n", component, component)
		}
		writeFixture(tb, filepath.Join(f.root, "stacks", "deploy", fmt.Sprintf("s%03d.yaml", stack)), content.String())
	}
	fixtureGit(tb, f.root, "add", ".")
	fixtureGit(tb, f.root, "commit", "-qm", "spread store callers")
	fixtureGit(tb, f.root, "worktree", "remove", "--force", f.base)
	fixtureGit(tb, f.root, "worktree", "add", "--detach", f.base, "HEAD")
	writeFixture(tb, filepath.Join(f.root, "components", "terraform", "mock", "main.tf"), "variable \"generation\" { default = 2 }\n")
	fixtureGit(tb, f.root, "add", ".")
	fixtureGit(tb, f.root, "commit", "-qm", "change shared component")
	return f, reads.Load
}

func storeCallerStacksArgs(f fixture) []string {
	return []string{"describe", "affected", "--repo-path", f.base, "--include-dependents=true", "--skip=store.get", "--skip=store", "--process-functions=false"}
}

func benchmarkStoreCallerStacks(b *testing.B, binary string) {
	f, reads := storeCallerStacksFixture(b)
	validateStoreCallerValues(b, binary, f)
	var total int64
	b.ResetTimer()
	for range b.N {
		b.StopTimer()
		home := b.TempDir()
		before := reads()
		b.StartTimer()
		data := runCLI(b, binary, f, home, storeCallerStacksArgs(f))
		b.StopTimer()
		validateStoreCallerStacks(b, data)
		count := reads() - before
		if count < 11 || (os.Getenv("ATMOS_BENCH_OBSERVE_STORE_READS") != "true" && count != 22) {
			b.Fatalf("store reads=%d, want 22 (11 keys per configuration root)", count)
		}
		total += count
		b.StartTimer()
	}
	b.ReportMetric(float64(total)/float64(b.N), "backend-reads/op")
}

func validateStoreCallerStacks(tb testing.TB, data []byte) {
	tb.Helper()
	validateCLI(tb, data, 200, "DescribeAffected")
	// Exact identities matter: callers must survive every comparison pass.
	var rows []dependentRow
	if err := json.Unmarshal(data, &rows); err != nil {
		tb.Fatal(err)
	}
	want := map[string]bool{}
	for stack := range 20 {
		for component := range 10 {
			want[fmt.Sprintf("s%03d/app-%03d", stack, component)] = true
		}
	}
	for _, row := range rows {
		key := row.Stack + "/" + row.Component
		if !want[key] {
			tb.Fatalf("unexpected affected instance: %s", key)
		}
		delete(want, key)
	}
}

func TestStoreCallerStacksCLI(t *testing.T) {
	binary := os.Getenv("ATMOS_BENCH_BINARY")
	if binary == "" {
		t.Skip("set ATMOS_BENCH_BINARY to validate the CLI fixture")
	}
	f, reads := storeCallerStacksFixture(t)
	validateStoreCallerValues(t, binary, f)
	before := reads()
	data := runCLI(t, binary, f, t.TempDir(), storeCallerStacksArgs(f))
	validateStoreCallerStacks(t, data)
	if got := reads() - before; got != 22 {
		t.Fatalf("store reads=%d, want 22", got)
	}
}

// Resolve values outside timing so skipped template work cannot look faster.
func validateStoreCallerValues(tb testing.TB, binary string, f fixture) {
	tb.Helper()
	for _, root := range []string{f.root, f.base} {
		f.root = root
		data := runCLI(tb, binary, f, tb.TempDir(), []string{"describe", "stacks", "--process-functions=false", "--skip=store.get", "--skip=store"})
		var stacks map[string]struct {
			Components struct {
				Terraform map[string]struct{ Vars map[string]any } `json:"terraform"`
			} `json:"components"`
		}
		if err := json.Unmarshal(data, &stacks); err != nil {
			tb.Fatal(err)
		}
		for stack := range 20 {
			for component := range 10 {
				for key := range 11 {
					value := stacks[fmt.Sprintf("s%03d", stack)].Components.Terraform[fmt.Sprintf("app-%03d", component)].Vars[fmt.Sprintf("store_%02d", key)]
					if value != "offline-store-value" {
						tb.Fatalf("unexpected store value: %v", value)
					}
				}
			}
		}
	}
}
