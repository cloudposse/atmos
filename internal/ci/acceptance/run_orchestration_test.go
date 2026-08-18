package acceptance

import (
	"os"
	"path/filepath"
	"testing"
)

// newOrchestrationFixtureModule builds a throwaway module shaped like the
// repository layout RunShard/Precompile/runCmdTests expect (a cmd/ package
// and a tests/ package, both with a trivial passing test), plus two more
// top-level packages so shardPackages' bin-packing has something to assign
// to a non-exec/helper shard. Unlike Verify (which hardcodes the real
// atmos ExecPackage/TestHelpersPackage import paths), RunShard's "pkgs" group
// is just whatever shardPackages bin-packs, so any module name works here as
// long as the run avoids landing on the hardcoded exec/helper shard indexes
// (shard 2 and 3 for a 3-shard split) -- shard 1 is used throughout below.
func newOrchestrationFixtureModule(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	write := func(relPath, content string) {
		full := filepath.Join(root, relPath)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	// -coverpkg=./... needs at least one real, executable statement somewhere
	// in the module to produce any covmeta output at all -- a module of only
	// test files and empty package declarations instruments to "[no
	// statements]" and writes nothing under GOCOVERDIR.
	write("go.mod", "module example.com/runfixture\n\ngo 1.21\n")
	write("cmd/cmd.go", "package cmd\n\nfunc Add(a, b int) int {\n\treturn a + b\n}\n")
	write("cmd/cmd_test.go", "package cmd\n\nimport \"testing\"\n\nfunc TestCmdSample(t *testing.T) {}\n")
	write("tests/tests.go", "package tests\n\nfunc Add(a, b int) int {\n\treturn a + b\n}\n")
	write("tests/tests_test.go", "package tests\n\nimport \"testing\"\n\nfunc TestSample(t *testing.T) {}\n")
	// The "pkgs" group's shard assignment (shardPackages' bin-packing) can
	// land on either extra package depending on Count/Index, and a package
	// with no _test.go file at all produces no GOCOVERDIR output even with
	// real statements -- see the CollectCoverage doc comment on this same
	// point. Give both a real test so whichever one is assigned still
	// collects native coverage.
	write("extra1/extra1.go", "package extra1\n\nfunc Add(a, b int) int {\n\treturn a + b\n}\n")
	write("extra1/extra1_test.go", "package extra1\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif Add(1, 2) != 3 {\n\t\tt.Fatal(\"unexpected sum\")\n\t}\n}\n")
	write("extra2/extra2.go", "package extra2\n\nfunc Add(a, b int) int {\n\treturn a + b\n}\n")
	write("extra2/extra2_test.go", "package extra2\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif Add(1, 2) != 3 {\n\t\tt.Fatal(\"unexpected sum\")\n\t}\n}\n")
	return root
}

func TestPrecompileLinux(t *testing.T) {
	t.Parallel()

	root := newOrchestrationFixtureModule(t)
	outputDir := filepath.Join(t.TempDir(), "build")
	if err := Precompile(t.Context(), root, TargetLinux, outputDir); err != nil {
		t.Fatalf("precompile: %v", err)
	}
	if err := requireFile(filepath.Join(outputDir, "cmd.test")); err != nil {
		t.Fatalf("expected cmd.test to be built: %v", err)
	}
}

func TestPrecompileWindows(t *testing.T) {
	t.Parallel()

	root := newOrchestrationFixtureModule(t)
	// Precompile's Windows path additionally builds ./tests and
	// ./internal/exec into standalone binaries.
	if err := os.MkdirAll(filepath.Join(root, "internal", "exec"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "internal", "exec", "exec_test.go"),
		[]byte("package exec\n\nimport \"testing\"\n\nfunc TestExecSample(t *testing.T) {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	outputDir := filepath.Join(t.TempDir(), "build")
	if err := Precompile(t.Context(), root, TargetWindows, outputDir); err != nil {
		t.Fatalf("precompile: %v", err)
	}
	for _, binary := range []string{"cmd.test.exe", "tests.test.exe", "internal-exec.test.exe"} {
		if err := requireFile(filepath.Join(outputDir, binary)); err != nil {
			t.Fatalf("expected %s to be built: %v", binary, err)
		}
	}
}

func TestPrecompilePropagatesBuildFailure(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/broken\n\ngo 1.21\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "cmd"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "cmd", "cmd.go"), []byte("package cmd\n\nthis is not valid go\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := Precompile(t.Context(), root, TargetLinux, t.TempDir()); err == nil {
		t.Fatal("expected an error compiling a package with invalid source")
	}
}

func TestRunCmdTestsPropagatesMissingBinary(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "cmd"), 0o755); err != nil {
		t.Fatal(err)
	}
	options := &RunOptions{RepoRoot: root, Target: TargetLinux, Mode: ModeTest, BuildDir: t.TempDir()}
	if _, err := runCmdTests(t.Context(), newCommandRunner(), options); err == nil {
		t.Fatal("expected an error when cmd.test is missing")
	}
}

func TestRunCmdTests(t *testing.T) {
	t.Parallel()

	root := newOrchestrationFixtureModule(t)
	buildDir := filepath.Join(t.TempDir(), "build")
	if err := Precompile(t.Context(), root, TargetLinux, buildDir); err != nil {
		t.Fatalf("precompile fixture cmd.test: %v", err)
	}

	// ModeTest: runs the binary directly, no coverage output.
	testOptions := &RunOptions{RepoRoot: root, Target: TargetLinux, Mode: ModeTest, BuildDir: buildDir}
	coverDir, err := runCmdTests(t.Context(), newCommandRunner(), testOptions)
	if err != nil {
		t.Fatalf("run cmd tests (test mode): %v", err)
	}
	if coverDir != "" {
		t.Fatalf("expected no coverage directory in test mode, got %q", coverDir)
	}

	// ModeCoverage: same binary, but collects GOCOVERDIR output.
	coverageRoot := t.TempDir()
	coverageOptions := &RunOptions{
		RepoRoot: root, Target: TargetLinux, Mode: ModeCoverage,
		BuildDir: buildDir, CoverageRoot: coverageRoot, Shard: Shard{Index: 1, Count: 1},
	}
	coverDir, err = runCmdTests(t.Context(), newCommandRunner(), coverageOptions)
	if err != nil {
		t.Fatalf("run cmd tests (coverage mode): %v", err)
	}
	if !hasCoverageMetadata(coverDir) {
		t.Fatalf("expected native coverage metadata under %s", coverDir)
	}
}

func TestRunSourceTestGroupModeTest(t *testing.T) {
	t.Parallel()

	root := newOrchestrationFixtureModule(t)
	options := &RunOptions{RepoRoot: root, Mode: ModeTest, GoTestTimeout: "1m"}
	group := sourceTestGroup{name: "extras", packages: []string{"./extra1", "./extra2"}}
	if _, err := runSourceTestGroup(t.Context(), newCommandRunner(), options, group); err != nil {
		t.Fatalf("run source test group (test mode): %v", err)
	}
}

func TestRunSourceTestGroupModeCoverage(t *testing.T) {
	t.Parallel()

	root := newOrchestrationFixtureModule(t)
	options := &RunOptions{
		RepoRoot: root, Mode: ModeCoverage, GoTestTimeout: "1m",
		CoverageRoot: t.TempDir(), Shard: Shard{Index: 1, Count: 1},
	}
	group := sourceTestGroup{name: "extras", packages: []string{"./cmd", "./tests"}}
	dataDir, err := runSourceTestGroup(t.Context(), newCommandRunner(), options, group)
	if err != nil {
		t.Fatalf("run source test group (coverage mode): %v", err)
	}
	if !hasCoverageMetadata(dataDir) {
		t.Fatalf("expected native coverage metadata under %s", dataDir)
	}
}

func TestCollectCoverage(t *testing.T) {
	t.Parallel()

	root := newOrchestrationFixtureModule(t)
	dataOut := filepath.Join(t.TempDir(), "merged")
	textOut := filepath.Join(t.TempDir(), "coverage.out")
	err := CollectCoverage(t.Context(), &CoverageOptions{
		RepoRoot: root,
		Dir:      filepath.Join(t.TempDir(), "work"),
		DataOut:  dataOut,
		TextOut:  textOut,
		Timeout:  "1m",
	}, []string{"./cmd", "./tests"}, nil)
	if err != nil {
		t.Fatalf("collect coverage: %v", err)
	}
	if !hasCoverageMetadata(dataOut) {
		t.Fatalf("expected native coverage metadata under %s", dataOut)
	}
	if _, statErr := os.Stat(textOut); statErr != nil {
		t.Fatalf("expected a coverage text report at %s: %v", textOut, statErr)
	}
}

func TestRunShardLinux(t *testing.T) {
	t.Parallel()

	root := newOrchestrationFixtureModule(t)
	buildDir := filepath.Join(t.TempDir(), "build")
	if err := Precompile(t.Context(), root, TargetLinux, buildDir); err != nil {
		t.Fatalf("precompile: %v", err)
	}

	options := &RunOptions{
		RepoRoot: root, Mode: ModeTest, Target: TargetLinux,
		Shard: Shard{Index: 1, Count: 3}, BuildDir: buildDir, GoTestTimeout: "1m",
	}
	if err := RunShard(t.Context(), options); err != nil {
		t.Fatalf("run shard (linux, test mode): %v", err)
	}

	coverageOptions := &RunOptions{
		RepoRoot: root, Mode: ModeCoverage, Target: TargetLinux,
		Shard: Shard{Index: 1, Count: 3}, BuildDir: buildDir,
		CoverageRoot: t.TempDir(), GoTestTimeout: "1m",
	}
	if err := RunShard(t.Context(), coverageOptions); err != nil {
		t.Fatalf("run shard (linux, coverage mode): %v", err)
	}
	merged := filepath.Join(coverageOptions.CoverageRoot, "shard-1")
	if !hasCoverageMetadata(merged) {
		t.Fatalf("expected merged native coverage metadata under %s", merged)
	}
}

func TestRunShardWindows(t *testing.T) {
	t.Parallel()

	root := newOrchestrationFixtureModule(t)
	if err := os.MkdirAll(filepath.Join(root, "internal", "exec"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "internal", "exec", "exec_test.go"),
		[]byte("package exec\n\nimport \"testing\"\n\nfunc TestExecSample(t *testing.T) {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	buildDir := filepath.Join(t.TempDir(), "build")
	if err := Precompile(t.Context(), root, TargetWindows, buildDir); err != nil {
		t.Fatalf("precompile: %v", err)
	}

	options := &RunOptions{
		RepoRoot: root, Mode: ModeTest, Target: TargetWindows,
		Shard: Shard{Index: 1, Count: 3}, BuildDir: buildDir, GoTestTimeout: "1m",
	}
	if err := RunShard(t.Context(), options); err != nil {
		t.Fatalf("run shard (windows, test mode): %v", err)
	}
}
