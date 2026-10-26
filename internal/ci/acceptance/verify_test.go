package acceptance

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// realRepoRoot resolves this checkout's repository root. Verify's package
// assignment check hardcodes the real ExecPackage and TestHelpersPackage
// import paths (github.com/cloudposse/atmos/...), so it only self-validates
// against this repository's actual package graph, not an arbitrarily-named
// fixture module. These tests exercise Verify against the real thing,
// matching how `go tool mage acceptance:verify` invokes it in CI, rather
// than mocking the module away.
func realRepoRoot(t *testing.T) string {
	t.Helper()
	root, err := FindRepoRoot(".")
	if err != nil {
		t.Fatalf("find repo root: %v", err)
	}
	return root
}

func TestVerifyLinuxTarget(t *testing.T) {
	t.Parallel()

	root := realRepoRoot(t)
	binaryDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(binaryDir, "cmd.test"), []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := Verify(t.Context(), root, TargetLinux, 10, binaryDir); err != nil {
		t.Fatalf("verify linux target: %v", err)
	}

	// A missing cmd.test binary is a real, expected failure mode.
	if err := Verify(t.Context(), root, TargetLinux, 10, t.TempDir()); err == nil {
		t.Fatal("expected an error when cmd.test is missing")
	}
}

func TestVerifyWindowsTarget(t *testing.T) {
	t.Parallel()

	root := realRepoRoot(t)
	binaryDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(binaryDir, "cmd.test.exe"), []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	testsBinary := buildFixtureTestBinary(t, []string{CLICommandsTest, RegistryTest, "TestAlpha", "TestBeta"})
	if err := copyFile(testsBinary, filepath.Join(binaryDir, "tests.test.exe")); err != nil {
		t.Fatal(err)
	}
	execBinary := buildFixtureTestBinary(t, []string{"TestGamma", "TestDelta"})
	if err := copyFile(execBinary, filepath.Join(binaryDir, "internal-exec.test.exe")); err != nil {
		t.Fatal(err)
	}

	if err := Verify(t.Context(), root, TargetWindows, 10, binaryDir); err != nil {
		t.Fatalf("verify windows target: %v", err)
	}
}

func TestVerifyRejectsNonPositiveShardCount(t *testing.T) {
	t.Parallel()

	if err := Verify(t.Context(), realRepoRoot(t), TargetLinux, 0, t.TempDir()); err == nil {
		t.Fatal("expected an error for a non-positive shard count")
	}
}

func TestVerifyPropagatesWorkflowMismatch(t *testing.T) {
	t.Parallel()

	root := realRepoRoot(t)
	binaryDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(binaryDir, "cmd.test"), []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	// The real workflow declares 10 shards; a mismatched count here should
	// surface verifyWorkflow's own shard-count-mismatch error through Verify.
	if err := Verify(t.Context(), root, TargetLinux, 4, binaryDir); err == nil {
		t.Fatal("expected an error from a shard count that doesn't match the real workflow")
	}
}

func TestVerifyWindowsTestsPackageRequiresCoreTests(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "tests"), 0o755); err != nil {
		t.Fatal(err)
	}
	binaryDir := t.TempDir()
	// Missing TestCLICommands/TestTerraformRegistryCache entirely.
	binary := buildFixtureTestBinary(t, []string{"TestAlpha"})
	if err := copyFile(binary, filepath.Join(binaryDir, "tests.test.exe")); err != nil {
		t.Fatal(err)
	}

	err := verifyWindowsTestsPackage(t.Context(), newCommandRunner(), root, binaryDir, 3)
	if err == nil {
		t.Fatal("expected an error when the tests binary lacks the core CLI/registry tests")
	}
}

func TestVerifyWindowsTestRoutesPropagatesMissingBinary(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "tests"), 0o755); err != nil {
		t.Fatal(err)
	}
	// binaryDir has no tests.test.exe at all.
	if err := verifyWindowsTestRoutes(t.Context(), newCommandRunner(), root, t.TempDir(), 3); err == nil {
		t.Fatal("expected an error when tests.test.exe is missing")
	}
}

func TestVerifyWindowsExecPackagePropagatesMissingBinary(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "internal", "exec"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := verifyWindowsExecPackage(t.Context(), newCommandRunner(), root, t.TempDir(), 3); err == nil {
		t.Fatal("expected an error when internal-exec.test.exe is missing")
	}
}

func copyFile(source, destination string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	return os.WriteFile(destination, data, 0o755)
}

func TestBinaryExtension(t *testing.T) {
	t.Parallel()

	if got := binaryExtension(TargetWindows); got != ".exe" {
		t.Fatalf("binaryExtension(windows) = %q, want %q", got, ".exe")
	}
	for _, target := range []Target{TargetLinux, TargetMacOS} {
		if got := binaryExtension(target); got != "" {
			t.Fatalf("binaryExtension(%s) = %q, want empty", target, got)
		}
	}
}

func TestExpectedPackages(t *testing.T) {
	t.Parallel()

	all := []string{TestsPackage, CmdPackage, ExecPackage, "example.com/a", "example.com/b"}

	linux := expectedPackages(all, TargetLinux)
	wantLinux := []string{ExecPackage, "example.com/a", "example.com/b"}
	if !reflect.DeepEqual(linux, wantLinux) {
		t.Fatalf("expectedPackages(linux) = %#v, want %#v", linux, wantLinux)
	}

	windows := expectedPackages(all, TargetWindows)
	wantWindows := []string{"example.com/a", "example.com/b"}
	if !reflect.DeepEqual(windows, wantWindows) {
		t.Fatalf("expectedPackages(windows) = %#v, want %#v", windows, wantWindows)
	}
}

func TestVerifyPropagatesListPackagesError(t *testing.T) {
	t.Parallel()

	if err := Verify(t.Context(), t.TempDir(), TargetLinux, 10, t.TempDir()); err == nil {
		t.Fatal("expected an error discovering packages outside any Go module")
	}
}

func TestVerifyPropagatesWindowsTestRoutesError(t *testing.T) {
	t.Parallel()

	root := realRepoRoot(t)
	binaryDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(binaryDir, "cmd.test.exe"), []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	// tests.test.exe/internal-exec.test.exe are both missing, so Verify should
	// propagate verifyWindowsTestRoutes' own error rather than reaching
	// verifyWorkflow.
	if err := Verify(t.Context(), root, TargetWindows, 10, binaryDir); err == nil {
		t.Fatal("expected an error when the Windows test binaries are missing")
	}
}

func TestVerifyWindowsTestsPackagePropagatesListError(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "tests"), 0o755); err != nil {
		t.Fatal(err)
	}
	binaryDir := t.TempDir()
	writeUnexecutableFile(t, filepath.Join(binaryDir, "tests.test.exe"))
	if err := verifyWindowsTestsPackage(t.Context(), newCommandRunner(), root, binaryDir, 3); err == nil {
		t.Fatal("expected an error when the tests binary can't be listed")
	}
}

func TestVerifyWindowsExecPackagePropagatesListError(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "internal", "exec"), 0o755); err != nil {
		t.Fatal(err)
	}
	binaryDir := t.TempDir()
	writeUnexecutableFile(t, filepath.Join(binaryDir, "internal-exec.test.exe"))
	if err := verifyWindowsExecPackage(t.Context(), newCommandRunner(), root, binaryDir, 3); err == nil {
		t.Fatal("expected an error when the internal/exec binary can't be listed")
	}
}

func TestVerifyExactAssignmentDetectsDuplicatesAndMismatches(t *testing.T) {
	t.Parallel()

	if err := verifyExactAssignment("things", []string{"a", "b"}, []string{"a", "a", "b"}); err == nil {
		t.Fatal("expected an error for a duplicate assignment")
	}
	if err := verifyExactAssignment("things", []string{"a", "b"}, []string{"a"}); err == nil {
		t.Fatal("expected an error for a missing assignment")
	}
	if err := verifyExactAssignment("things", []string{"a"}, []string{"a", "b"}); err == nil {
		t.Fatal("expected an error for an unexpected assignment")
	}
}

func TestVerifyWorkflowRejectsMalformedContent(t *testing.T) {
	t.Parallel()

	validChecks := `name: ${{ matrix.check }}
check: ["Acceptance Tests (linux)", "Acceptance Tests (macos)", "Acceptance Tests (windows)"]
`
	validRoute := "run: go test ./tests -run '^" + RegistryTest + "$'\n"

	testCases := []struct {
		name     string
		workflow string
	}{
		{
			name:     "no shard matrix",
			workflow: validChecks + validRoute,
		},
		{
			name:     "two shard matrices",
			workflow: "shard: [1, 2, 3]\nshard: [1, 2, 3]\n" + validChecks + validRoute,
		},
		{
			name:     "shard positions out of order",
			workflow: "shard: [1, 3, 2]\n" + validChecks + validRoute,
		},
		{
			name:     "no dedicated registry test route",
			workflow: "shard: [1, 2, 3]\n" + validChecks,
		},
		{
			name:     "no check matrix",
			workflow: "shard: [1, 2, 3]\n" + validRoute,
		},
		{
			name:     "two check matrices",
			workflow: "shard: [1, 2, 3]\n" + validChecks + `check: ["a"]` + "\n" + validRoute,
		},
		{
			name:     "wrong number of checks",
			workflow: "shard: [1, 2, 3]\nname: ${{ matrix.check }}\ncheck: [\"Acceptance Tests (linux)\"]\n" + validRoute,
		},
		{
			name:     "check name mismatch",
			workflow: "shard: [1, 2, 3]\nname: ${{ matrix.check }}\ncheck: [\"a\", \"b\", \"c\"]\n" + validRoute,
		},
		{
			name:     "missing matrix.check job name",
			workflow: `shard: [1, 2, 3]` + "\n" + `check: ["Acceptance Tests (linux)", "Acceptance Tests (macos)", "Acceptance Tests (windows)"]` + "\n" + validRoute,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			workflowDir := filepath.Join(root, ".github", "workflows")
			if err := os.MkdirAll(workflowDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(workflowDir, "test.yml"), []byte(testCase.workflow), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := verifyWorkflow(root, 3); err == nil {
				t.Fatalf("expected an error for workflow content: %s", testCase.name)
			}
		})
	}
}

func TestVerifyWorkflowPropagatesReadFileError(t *testing.T) {
	t.Parallel()

	if err := verifyWorkflow(t.TempDir(), 3); err == nil {
		t.Fatal("expected an error when the workflow file doesn't exist")
	}
}

func TestAssignedPackages(t *testing.T) {
	t.Parallel()

	all := []string{TestsPackage, CmdPackage, ExecPackage, TestHelpersPackage, "example.com/a", "example.com/b"}
	assigned := assignedPackages(all, TargetLinux, 3)
	if err := verifyExactAssignment("packages", expectedPackages(all, TargetLinux), assigned); err != nil {
		t.Fatalf("assignedPackages: %v", err)
	}
}
