// Package benchmarks measures complete, offline Atmos CLI invocations.
package benchmarks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

type fixture struct {
	env        []string
	root, base string
	instances  int
}

func writeFixture(tb testing.TB, path, content string) {
	tb.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		tb.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		tb.Fatal(err)
	}
}

func fixtureGit(tb testing.TB, dir string, args ...string) {
	tb.Helper()
	cmd := exec.Command("git", append([]string{"-c", "core.hooksPath=/dev/null", "-c", "commit.gpgsign=false", "-c", "user.name=Benchmark", "-c", "user.email=benchmark@example.invalid"}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
	if out, err := cmd.CombinedOutput(); err != nil {
		tb.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func makeFixture(tb testing.TB, instances int, change string) fixture {
	tb.Helper()
	parent := tb.TempDir()
	f := fixture{root: filepath.Join(parent, "current"), base: filepath.Join(parent, "base"), instances: instances}
	writeFixture(tb, filepath.Join(f.root, "atmos.yaml"), `base_path: .
components:
  terraform:
    base_path: components/terraform
stacks:
  base_path: stacks
  included_paths: ["deploy/**/*"]
  name_template: "{{ .vars.stage }}"
templates:
  settings:
    enabled: true
    sprig:
      enabled: true
settings:
  telemetry:
    enabled: false
  terminal:
    color: false
`)
	writeFixture(tb, filepath.Join(f.root, "components", "terraform", "mock", "main.tf"), "variable \"generation\" { default = 1 }\n")
	writeFixture(tb, filepath.Join(f.root, "stacks", "catalog", "values.yaml"), "value: offline\n")
	shared := `locals:
  namespace: acme
  prefix: '{{ .locals.namespace }}-benchmark'
components:
  terraform:
    base:
      metadata:
        type: abstract
        component: mock
      vars:
        generation: 1
        prefix: '{{ .locals.prefix }}'
        local_value: !env ATMOS_BENCH_VALUE
    inherited:
      metadata:
        type: abstract
        component: mock
        inherits: [base]
      vars:
        tags:
          Purpose: performance
`
	writeFixture(tb, filepath.Join(f.root, "stacks", "catalog", "shared.yaml"), shared)
	// 100 component instances per stack keeps stack and component counts growing
	// together while repeatedly importing and inheriting the shared catalog.
	for stack := range instances / 100 {
		var content strings.Builder
		fmt.Fprintf(&content, "import: [catalog/shared]\nvars:\n  stage: s%03d\ncomponents:\n  terraform:\n", stack)
		for component := range 100 {
			fmt.Fprintf(&content, "    app-%03d:\n      metadata:\n        component: mock\n        inherits: [inherited]\n      vars:\n        name: app-%03d\n", component, component)
		}
		writeFixture(tb, filepath.Join(f.root, "stacks", "deploy", fmt.Sprintf("s%03d.yaml", stack)), content.String())
	}
	fixtureGit(tb, f.root, "init", "-b", "main")
	// Atmos parses the origin URL, but --repo-path ensures no remote is fetched.
	fixtureGit(tb, f.root, "remote", "add", "origin", "https://github.com/atmos-benchmark/offline-fixture.git")
	fixtureGit(tb, f.root, "add", ".")
	fixtureGit(tb, f.root, "commit", "-qm", "baseline")
	fixtureGit(tb, f.root, "worktree", "add", "--detach", f.base, "HEAD")
	switch change {
	case "catalog":
		writeFixture(tb, filepath.Join(f.root, "stacks", "catalog", "shared.yaml"), strings.Replace(shared, "generation: 1", "generation: 2", 1))
	case "component":
		writeFixture(tb, filepath.Join(f.root, "components", "terraform", "mock", "main.tf"), "variable \"generation\" { default = 2 }\n")
	case "none":
	default:
		tb.Fatalf("unknown change: %s", change)
	}
	if change != "none" {
		fixtureGit(tb, f.root, "add", ".")
		fixtureGit(tb, f.root, "commit", "-qm", change)
	}
	return f
}

func cliEnv(home string) []string {
	// Pass only host execution essentials; developer/CI Atmos and cloud settings
	// must not change the workload or supply credentials.
	env := []string{
		"HOME=" + home, "XDG_CONFIG_HOME=" + home, "XDG_CACHE_HOME=" + home, "XDG_DATA_HOME=" + home,
		"GOMAXPROCS=2", "ATMOS_BENCH_VALUE=offline", "ATMOS_TELEMETRY_ENABLED=false", "ATMOS_VERSION_CHECK_ENABLED=false", "ATMOS_PRO_ENABLED=false", "NO_COLOR=1", "TERM=dumb", "AWS_EC2_METADATA_DISABLED=true", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null",
	}
	for _, key := range []string{"PATH", "TMPDIR", "SYSTEMROOT", "TEMP", "TMP"} {
		if value := os.Getenv(key); value != "" {
			env = append(env, key+"="+value)
		}
	}
	return env
}

func runCLI(tb testing.TB, binary string, f fixture, home string, args []string) []byte {
	tb.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, append(args, "--identity=false", "--format=json")...)
	cmd.Dir, cmd.Env = f.root, append(cliEnv(home), f.env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		tb.Fatalf("atmos %v: %v\n%s", args, err, stderr.String())
	}
	return stdout.Bytes()
}

func validateCLI(tb testing.TB, data []byte, count int, command string) {
	tb.Helper()
	if command == "DescribeStacks" {
		validateStacks(tb, data, count)
		return
	}

	var rows []struct {
		Stack     string `json:"stack"`
		Component string `json:"component"`
	}
	if err := json.Unmarshal(data, &rows); err != nil {
		tb.Fatalf("result JSON: %v\n%.1000s", err, data)
	}
	if len(rows) != count {
		tb.Fatalf("got %d instances, want %d\n%.1000s", len(rows), count, data)
	}
	seen := map[string]bool{}
	for _, row := range rows {
		key := row.Stack + "/" + row.Component
		if seen[key] || !strings.HasPrefix(row.Stack, "s") || !strings.HasPrefix(row.Component, "app-") {
			tb.Fatalf("unexpected/duplicate instance: %s", key)
		}
		seen[key] = true
	}
}

func BenchmarkCLI(b *testing.B) {
	binary := os.Getenv("ATMOS_BENCH_BINARY")
	if binary == "" {
		b.Skip("run through go tool mage bench:run, or set ATMOS_BENCH_BINARY")
	}
	b.Run("DescribeAffected/Dependents/instances=1000", func(b *testing.B) { benchmarkDependentsCLI(b, binary) })
	for _, command := range []string{"DescribeAffected", "DescribeStacks"} {
		b.Run(command+"/StoreRepeated/calls=2200/keys=11", func(b *testing.B) { benchmarkStoreCLI(b, binary, command) })
	}
	for _, size := range []int{100, 1000, 10000} {
		for _, change := range []string{"none", "catalog", "component"} {
			b.Run("DescribeAffected/instances="+strconv.Itoa(size)+"/change="+change, func(b *testing.B) {
				f := makeFixture(b, size, change)
				args := []string{"describe", "affected", "--repo-path", f.base}
				expected := size
				if change == "none" {
					expected = 0
				}
				benchmarkCLI(b, binary, f, args, expected, "DescribeAffected")
			})
		}
	}
	for _, command := range []string{"DescribeStacks", "ListInstances"} {
		b.Run(command+"/instances=1000", func(b *testing.B) {
			f := makeFixture(b, 1000, "none")
			args := []string{"describe", "stacks"}
			if command == "ListInstances" {
				args = []string{"list", "instances"}
			}
			benchmarkCLI(b, binary, f, args, 1000, command)
		})
	}
}

func benchmarkCLI(b *testing.B, binary string, f fixture, args []string, expected int, command string) {
	b.Helper()
	b.ResetTimer()
	for range b.N {
		b.StopTimer()
		home := b.TempDir()
		b.StartTimer()
		data := runCLI(b, binary, f, home, args)
		b.StopTimer()
		validateCLI(b, data, expected, command)
		b.StartTimer()
	}
}

func validateStacks(tb testing.TB, data []byte, count int) {
	tb.Helper()
	var stacks map[string]struct {
		Components struct {
			Terraform map[string]json.RawMessage `json:"terraform"`
		} `json:"components"`
	}
	if err := json.Unmarshal(data, &stacks); err != nil {
		tb.Fatalf("stacks JSON: %v\n%.1000s", err, data)
	}
	n := 0
	for stack, value := range stacks {
		for component := range value.Components.Terraform {
			if strings.HasPrefix(component, "app-") {
				if !strings.HasPrefix(stack, "s") {
					tb.Fatalf("unexpected stack: %s", stack)
				}
				validateStackValues(tb, value.Components.Terraform[component])
				n++
			}
		}
	}
	if n != count {
		tb.Fatalf("got %d component instances, want %d", n, count)
	}
}

func validateStackValues(tb testing.TB, data json.RawMessage) {
	tb.Helper()
	var component struct {
		Vars map[string]any `json:"vars"`
	}
	if err := json.Unmarshal(data, &component); err != nil {
		tb.Fatal(err)
	}
	if _, ok := component.Vars["store_00"]; ok {
		for key := range 11 {
			if component.Vars[fmt.Sprintf("store_%02d", key)] != "offline-store-value" {
				tb.Fatalf("unexpected store value for key %d", key)
			}
		}
	}
	for key, expected := range map[string]any{"generation": float64(1), "prefix": "acme-benchmark", "local_value": "offline"} {
		if component.Vars[key] != expected {
			tb.Fatalf("fixture %s = %v, want %v", key, component.Vars[key], expected)
		}
	}
}
