package acceptance

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// helperFailCountEnv names the counter file a TestMain-driven helper process reads to
// decide whether to simulate the transient Windows unlinkat race (see command.go) or
// succeed, letting TestRunRetriesTransientWindowsUnlinkError exercise commandRunner.run's
// retry loop against a real child process rather than a Windows-only race condition.
const helperFailCountEnv = "ATMOS_TEST_ACCEPTANCE_HELPER_FAIL_COUNT_FILE"

// TestMain lets this test binary act as a controllable subprocess for
// TestRunRetriesTransientWindowsUnlinkError, per this repo's cross-platform convention
// of self-exec instead of relying on platform-specific binaries like `false`.
func TestMain(m *testing.M) {
	if countFile := os.Getenv(helperFailCountEnv); countFile != "" {
		os.Exit(runUnlinkRaceHelper(countFile))
	}
	os.Exit(m.Run())
}

// runUnlinkRaceHelper decrements the count stored in countFile on every invocation:
// while positive, it prints the exact Go toolchain diagnostic isTransientWindowsUnlinkError
// matches and exits 1 (simulating the race); once the count reaches zero, it exits 0
// (simulating the retry succeeding).
func runUnlinkRaceHelper(countFile string) int {
	raw, err := os.ReadFile(countFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read fail count: %v\n", err)
		return 2
	}
	remaining, err := strconv.Atoi(string(raw))
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse fail count: %v\n", err)
		return 2
	}
	if remaining <= 0 {
		return 0
	}
	if writeErr := os.WriteFile(countFile, []byte(strconv.Itoa(remaining-1)), 0o600); writeErr != nil {
		fmt.Fprintf(os.Stderr, "write fail count: %v\n", writeErr)
		return 2
	}
	fmt.Fprintln(os.Stderr, "go: unlinkat C:\\Users\\RUNNER~1\\AppData\\Local\\Temp\\go-build0\\b0\\pkg.test.exe: "+
		"The process cannot access the file because it is being used by another process.")
	return 1
}

func TestEnvironment(t *testing.T) {
	t.Setenv("ATMOS_TEST_ACCEPTANCE_ENV_VAR", "value")
	if got := environment("ATMOS_TEST_ACCEPTANCE_ENV_VAR"); got != "value" {
		t.Fatalf("environment() = %q, want %q", got, "value")
	}
	if got := environment("ATMOS_TEST_ACCEPTANCE_ENV_VAR_UNSET"); got != "" {
		t.Fatalf("environment() of an unset variable = %q, want empty", got)
	}
}

func TestFindRepoRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/findroot\n\ngo 1.21\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	found, err := FindRepoRoot(nested)
	if err != nil {
		t.Fatalf("find repo root: %v", err)
	}
	// t.TempDir() can return a path containing symlinks (e.g. macOS's
	// /var -> /private/var); resolve both sides before comparing.
	wantRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	gotRoot, err := filepath.EvalSymlinks(found)
	if err != nil {
		t.Fatal(err)
	}
	if gotRoot != wantRoot {
		t.Fatalf("FindRepoRoot(%q) = %q, want %q", nested, gotRoot, wantRoot)
	}
}

func TestFindRepoRootErrorsOutsideAnyModule(t *testing.T) {
	t.Parallel()

	if _, err := FindRepoRoot(string(filepath.Separator)); err == nil {
		t.Fatal("expected an error finding a repo root from the filesystem root")
	}
}

func TestCommandString(t *testing.T) {
	t.Parallel()

	got := commandString("go", []string{"test", "./..."})
	want := "go test ./..."
	if got != want {
		t.Fatalf("commandString() = %q, want %q", got, want)
	}
}

func TestIsTransientWindowsUnlinkError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{
			name: "real diagnostic",
			output: `go: unlinkat C:\Users\RUNNER~1\AppData\Local\Temp\go-build860437867\b2679\list.test.exe: ` +
				`The process cannot access the file because it is being used by another process.`,
			want: true,
		},
		{name: "empty output", output: "", want: false},
		{
			name:   "real test failure",
			output: "--- FAIL: TestSomething (0.01s)\nFAIL\tgithub.com/cloudposse/atmos/cmd/list\t0.013s\n",
			want:   false,
		},
		{
			name:   "unrelated file-in-use error",
			output: "The process cannot access the file because it is being used by another process.",
			want:   false, // missing "unlinkat" -- not the Go toolchain's own cleanup diagnostic.
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isTransientWindowsUnlinkError(tt.output); got != tt.want {
				t.Fatalf("isTransientWindowsUnlinkError(%q) = %v, want %v", tt.output, got, tt.want)
			}
		})
	}
}

// newHelperRunner builds a commandRunner whose run() re-execs this test binary via
// runUnlinkRaceHelper, with retryDelay zeroed so the test doesn't pay the real
// (Windows-only) retry backoff.
func newHelperRunner(t *testing.T) commandRunner {
	t.Helper()
	var stdout, stderr bytes.Buffer
	return commandRunner{stdout: &stdout, stderr: &stderr, stdin: nil, retryDelay: 0}
}

func TestRunRetriesTransientWindowsUnlinkError(t *testing.T) {
	exePath, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test binary path: %v", err)
	}

	countFile := filepath.Join(t.TempDir(), "fail-count")
	if err := os.WriteFile(countFile, []byte("1"), 0o600); err != nil {
		t.Fatal(err)
	}

	runner := newHelperRunner(t)
	env := []string{helperFailCountEnv + "=" + countFile}
	if err := runner.run(context.Background(), t.TempDir(), env, exePath); err != nil {
		t.Fatalf("run() did not recover from a single transient failure: %v", err)
	}
}

func TestRunGivesUpAfterExhaustingRetries(t *testing.T) {
	exePath, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test binary path: %v", err)
	}

	countFile := filepath.Join(t.TempDir(), "fail-count")
	// One more failure than run() will retry, so the final attempt still fails.
	if err := os.WriteFile(countFile, []byte(strconv.Itoa(transientUnlinkRetries+1)), 0o600); err != nil {
		t.Fatal(err)
	}

	runner := newHelperRunner(t)
	env := []string{helperFailCountEnv + "=" + countFile}
	if err := runner.run(context.Background(), t.TempDir(), env, exePath); err == nil {
		t.Fatal("expected run() to fail once transient retries are exhausted")
	}
}

func TestRunDoesNotRetryUnrelatedFailures(t *testing.T) {
	exePath, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test binary path: %v", err)
	}

	// A missing count file makes the helper exit 2 immediately (an unrelated failure),
	// never printing the transient diagnostic -- run() must not retry this.
	countFile := filepath.Join(t.TempDir(), "fail-count-does-not-exist")

	runner := newHelperRunner(t)
	env := []string{helperFailCountEnv + "=" + countFile}
	if err := runner.run(context.Background(), t.TempDir(), env, exePath); err == nil {
		t.Fatal("expected run() to fail for an unrelated (non-transient) error")
	}
}
