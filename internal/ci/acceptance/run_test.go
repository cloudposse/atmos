package acceptance

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseMode(t *testing.T) {
	t.Parallel()

	for _, mode := range []Mode{ModeTest, ModeCoverage} {
		got, err := ParseMode(string(mode))
		if err != nil {
			t.Fatalf("parse mode %q: %v", mode, err)
		}
		if got != mode {
			t.Fatalf("parse mode %q = %q, want %q", mode, got, mode)
		}
	}

	if _, err := ParseMode("bogus"); err == nil {
		t.Fatal("expected an error for an unknown mode")
	}
}

func TestRequireFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	file := filepath.Join(dir, "artifact")
	if err := os.WriteFile(file, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := requireFile(file); err != nil {
		t.Fatalf("requireFile on a real file: %v", err)
	}
	if err := requireFile(dir); err == nil {
		t.Fatal("expected an error when the path is a directory")
	}
	if err := requireFile(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("expected an error for a missing path")
	}
}

func TestRunWindowsTests(t *testing.T) {
	t.Parallel()

	binary := buildFixtureTestBinary(t, []string{CLICommandsTest, RegistryTest, "TestAlpha", "TestBeta"})
	repoRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoRoot, "tests"), 0o755); err != nil {
		t.Fatal(err)
	}
	options := &RunOptions{
		RepoRoot:    repoRoot,
		TestsBinary: binary,
		Shard:       Shard{Index: 1, Count: 2},
	}
	if err := runWindowsTests(t.Context(), newCommandRunner(), options); err != nil {
		t.Fatalf("run windows tests: %v", err)
	}
}

func TestRunWindowsExecTests(t *testing.T) {
	t.Parallel()

	binary := buildFixtureTestBinary(t, []string{"TestAlpha", "TestBeta"})
	repoRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoRoot, "internal", "exec"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Shard 1/2 gets at least one of the two tests assigned -- exercises the
	// real run path.
	options := &RunOptions{
		RepoRoot:       repoRoot,
		ExecTestBinary: binary,
		Shard:          Shard{Index: 1, Count: 2},
	}
	if err := runWindowsExecTests(t.Context(), newCommandRunner(), options); err != nil {
		t.Fatalf("run windows exec tests: %v", err)
	}

	// A shard index past the number of tests gets nothing assigned -- exercises
	// the early nil return without invoking the binary.
	emptyOptions := &RunOptions{
		RepoRoot:       repoRoot,
		ExecTestBinary: binary,
		Shard:          Shard{Index: 5, Count: 5},
	}
	if err := runWindowsExecTests(t.Context(), newCommandRunner(), emptyOptions); err != nil {
		t.Fatalf("run windows exec tests with no assigned tests: %v", err)
	}
}

func TestRunWindowsTestsPropagatesMissingBinary(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoRoot, "tests"), 0o755); err != nil {
		t.Fatal(err)
	}
	options := &RunOptions{RepoRoot: repoRoot, TestsBinary: filepath.Join(t.TempDir(), "missing.exe")}
	if err := runWindowsTests(t.Context(), newCommandRunner(), options); err == nil {
		t.Fatal("expected an error when the tests binary is missing")
	}
}

func TestRunWindowsExecTestsPropagatesMissingBinary(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoRoot, "internal", "exec"), 0o755); err != nil {
		t.Fatal(err)
	}
	options := &RunOptions{RepoRoot: repoRoot, ExecTestBinary: filepath.Join(t.TempDir(), "missing.exe")}
	if err := runWindowsExecTests(t.Context(), newCommandRunner(), options); err == nil {
		t.Fatal("expected an error when the internal/exec binary is missing")
	}
}

func TestDefaultRunOptions(t *testing.T) {
	t.Setenv("BUILD_DIR", "")
	t.Setenv("COVERAGE_ROOT", "")
	t.Setenv("GO_TEST_TIMEOUT", "")
	t.Setenv("CMD_TEST_BIN", "")
	t.Setenv("TESTS_TEST_BIN", "")
	t.Setenv("INTERNAL_EXEC_TEST_BIN", "")

	shard := Shard{Index: 2, Count: 10}
	options := DefaultRunOptions("/repo", ModeCoverage, TargetLinux, shard)
	want := RunOptions{
		RepoRoot:      "/repo",
		Mode:          ModeCoverage,
		Target:        TargetLinux,
		Shard:         shard,
		BuildDir:      ".",
		CoverageRoot:  "coverage",
		GoTestTimeout: defaultTestTimeout,
	}
	if options != want {
		t.Fatalf("DefaultRunOptions() with unset env = %#v, want %#v", options, want)
	}

	t.Setenv("BUILD_DIR", "build-out")
	t.Setenv("COVERAGE_ROOT", "cov-out")
	t.Setenv("GO_TEST_TIMEOUT", "20m")
	t.Setenv("CMD_TEST_BIN", "cmd.test")
	t.Setenv("TESTS_TEST_BIN", "tests.test")
	t.Setenv("INTERNAL_EXEC_TEST_BIN", "exec.test")

	overridden := DefaultRunOptions("/repo", ModeTest, TargetWindows, shard)
	wantOverridden := RunOptions{
		RepoRoot:       "/repo",
		Mode:           ModeTest,
		Target:         TargetWindows,
		Shard:          shard,
		BuildDir:       "build-out",
		CoverageRoot:   "cov-out",
		GoTestTimeout:  "20m",
		CmdTestBinary:  "cmd.test",
		TestsBinary:    "tests.test",
		ExecTestBinary: "exec.test",
	}
	if overridden != wantOverridden {
		t.Fatalf("DefaultRunOptions() with env overrides = %#v, want %#v", overridden, wantOverridden)
	}
}

func TestSplitCommandLinePreservesQuoting(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		value string
		want  []string
	}{
		{name: "empty", value: "", want: []string{}},
		{name: "simple flags", value: "-v -short", want: []string{"-v", "-short"}},
		{
			name:  "quoted run pattern",
			value: `-run '^TestFoo$' -v`,
			want:  []string{"-run", "^TestFoo$", "-v"},
		},
		{
			name:  "double quoted value with spaces",
			value: `-run "^TestFoo bar$"`,
			want:  []string{"-run", "^TestFoo bar$"},
		},
		{
			name:  "newline separated",
			value: "-run\n^TestFoo$",
			want:  []string{"-run", "^TestFoo$"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := SplitCommandLine(testCase.value)
			if err != nil {
				t.Fatalf("split command line %q: %v", testCase.value, err)
			}
			if !reflect.DeepEqual(got, testCase.want) {
				t.Fatalf("split command line %q = %#v, want %#v", testCase.value, got, testCase.want)
			}
		})
	}
}

func TestSplitCommandLineRejectsUnterminatedQuotes(t *testing.T) {
	t.Parallel()

	if _, err := SplitCommandLine(`-run "^TestFoo$`); err == nil {
		t.Fatal("expected an error for an unterminated quote")
	}
}
