package toolchain

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain/installer"
)

// --- Fake Installer for testing ---.
type fakeInstaller struct {
	resolveOwner string
	resolveRepo  string
	resolveErr   error

	binaryPath string
	binaryErr  error
}

func (f *fakeInstaller) GetResolver() ToolResolver {
	return f
}

func (f *fakeInstaller) Resolve(tool string) (string, string, error) {
	return f.resolveOwner, f.resolveRepo, f.resolveErr
}

func (f *fakeInstaller) FindBinaryPath(owner, repo, version string, binaryName ...string) (string, error) {
	return f.binaryPath, f.binaryErr
}

func (f *fakeInstaller) CreateLatestFile(owner, repo, version string) error {
	return nil
}

func (f *fakeInstaller) ReadLatestFile(owner, repo string) (string, error) {
	return "", nil
}

// Resolver interface must match your actual implementation.
type Resolver interface {
	Resolve(tool string) (string, string, error)
}

// --- Mock syscall.Exec ---.
var (
	calledExecPath string
	calledExecArgs []string
	calledExecEnv  []string
)

func mockExec(path string, args []string, env []string) error {
	calledExecPath = path
	calledExecArgs = args
	calledExecEnv = env
	return nil
}

func resetExecMock() {
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{}})
	calledExecPath = ""
	calledExecArgs = nil
	calledExecEnv = nil
}

// --- Tests ---.
func TestRunExecCommand_Success(t *testing.T) {
	resetExecMock()
	execFunc = mockExec // swap syscall.Exec with mock

	// Set explicit install path for predictable test
	tempDir := t.TempDir()
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{InstallPath: filepath.Join(tempDir, ".tools")}})

	fake := &fakeInstaller{
		resolveOwner: "hashicorp",
		resolveRepo:  "terraform",
		binaryPath:   "/fake/path/terraform",
		binaryErr:    nil,
	}

	args := []string{"terraform@1.13.1", "--version"}
	AddToolToVersions(GetToolVersionsFilePath(), "terraform", "1.13.1")
	t.Log(ensureToolInstalled(fake, "terraform@1.13.1"))
	err := RunExecCommand(fake, args)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	binaryName := installer.EnsureWindowsExeExtension("terraform")
	expected := filepath.Join(tempDir, ".tools", "bin", "hashicorp", "terraform", "1.13.1", binaryName)
	if calledExecPath != expected {
		t.Errorf("expected exec path %q, got %q", expected, calledExecPath)
	}

	if calledExecArgs[1] != "--version" {
		t.Errorf("expected '--version' arg, got %v", calledExecArgs)
	}
}

func TestRunExecCommand_InvalidTool(t *testing.T) {
	fake := &fakeInstaller{
		resolveErr: errors.New("tool not found"),
	}

	args := []string{"unknown@1.0.0"}
	err := RunExecCommand(fake, args)
	if err == nil || err.Error() != "invalid tool name: tool not found" {
		t.Fatalf("expected invalid tool error, got %v", err)
	}
}

func TestRunExecCommand_NoArgs(t *testing.T) {
	fake := &fakeInstaller{}
	err := RunExecCommand(fake, []string{})
	if !errors.Is(err, ErrInvalidToolSpec) {
		t.Fatalf("expected ErrInvalidToolSpec, got %v", err)
	}
}

// TestRunExecCommand_MultiVersionUsesDefaultNotLast reproduces a bug where
// exec (via findBinaryPath, shared with WhichExec) resolved a tool's default
// version using the LAST token in a multi-version .tool-versions line instead
// of the FIRST (asdf convention: first token is the default). Only the first
// version is installed here; if the bug regresses, exec resolves the second
// ("1.6.0", never installed) and wrongly reports "not installed" even after
// attempting to auto-install.
func TestRunExecCommand_MultiVersionUsesDefaultNotLast(t *testing.T) {
	resetExecMock()
	execFunc = mockExec

	tempDir := t.TempDir()
	toolVersionsPath := filepath.Join(tempDir, DefaultToolVersionsFilePath)
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{
		InstallPath:  filepath.Join(tempDir, ".tools"),
		VersionsFile: toolVersionsPath,
	}})

	toolVersions := &ToolVersions{Tools: map[string][]string{"terraform": {"1.5.7", "1.6.0"}}}
	if err := SaveToolVersions(toolVersionsPath, toolVersions); err != nil {
		t.Fatalf("failed to save .tool-versions: %v", err)
	}

	// Only the default (first) version is installed on disk.
	binaryName := installer.EnsureWindowsExeExtension("terraform")
	expected := filepath.Join(tempDir, ".tools", "bin", "hashicorp", "terraform", "1.5.7", binaryName)
	if err := os.MkdirAll(filepath.Dir(expected), 0o755); err != nil {
		t.Fatalf("failed to create mock binary dir: %v", err)
	}
	if err := os.WriteFile(expected, []byte("mock terraform"), 0o755); err != nil {
		t.Fatalf("failed to write mock binary: %v", err)
	}

	fake := &fakeInstaller{resolveOwner: "hashicorp", resolveRepo: "terraform"}

	// No version specifier: must resolve and exec the default (first) version.
	err := RunExecCommand(fake, []string{"terraform", "--version"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if calledExecPath != expected {
		t.Errorf("expected exec path %q (default/first version), got %q", expected, calledExecPath)
	}
}

func TestRunExecCommand_BinaryNotFound(t *testing.T) {
	resetExecMock()
	execFunc = mockExec

	fake := &fakeInstaller{
		resolveOwner: "hashicorp",
		resolveRepo:  "terraform",
		binaryErr:    errors.New("binary not found"),
	}

	args := []string{"unknown@1.9.8"}

	err := RunExecCommand(fake, args)
	if err == nil {
		t.Fatalf("expected error when binary not found, got nil")
	}
}

// TestRunExecCommandWithOptions_DryRun verifies that --dry-run resolves (and, if necessary,
// installs) the real binary path but never invokes execFunc, printing the resolved path and
// args instead of running the tool.
func TestRunExecCommandWithOptions_DryRun(t *testing.T) {
	resetExecMock()
	execFunc = mockExec // Swap the real exec function so a regression would be caught, not silently run.

	tempDir := t.TempDir()
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{InstallPath: filepath.Join(tempDir, ".tools")}})

	fake := &fakeInstaller{
		resolveOwner: "hashicorp",
		resolveRepo:  "terraform",
		binaryPath:   "/fake/path/terraform",
		binaryErr:    nil,
	}

	args := []string{"terraform@1.13.1", "--version"}
	AddToolToVersions(GetToolVersionsFilePath(), "terraform", "1.13.1")
	// Pre-install (mirrors TestRunExecCommand_Success) so the binary is resolvable on disk.
	if _, err := ensureToolInstalled(fake, "terraform@1.13.1"); err != nil {
		t.Fatalf("failed to pre-install fixture tool: %v", err)
	}

	binaryName := installer.EnsureWindowsExeExtension("terraform")
	expectedPath := filepath.Join(tempDir, ".tools", "bin", "hashicorp", "terraform", "1.13.1", binaryName)

	var err error
	output := captureCleanTestOutput(t, func() {
		err = RunExecCommandWithOptions(fake, args, true)
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if calledExecPath != "" {
		t.Errorf("execFunc must not be invoked in dry-run mode, got exec path %q", calledExecPath)
	}
	if !strings.Contains(output, "Would execute") {
		t.Errorf("expected dry-run output to mention 'Would execute', got %q", output)
	}
	if !strings.Contains(output, expectedPath) {
		t.Errorf("expected dry-run output to mention resolved binary path %q, got %q", expectedPath, output)
	}
	if !strings.Contains(output, "--version") {
		t.Errorf("expected dry-run output to mention the passed args, got %q", output)
	}
}

// Note: RunExecCommand's "dryRun=false still executes" behavior is already covered by
// TestRunExecCommand_Success above (RunExecCommand delegates to
// RunExecCommandWithOptions(installer, args, false)), so it isn't duplicated here — doing so
// would just repeat the same real-binary-install fixture cost for no additional coverage.

// TestRunExecCommandWithOptions_DryRun_AutoInstallsMissingTool verifies the documented
// "dry-run auto-installs a missing tool, then reports without executing" behavior. Unlike
// TestRunExecCommandWithOptions_DryRun above (which pre-installs the binary before running
// in dry-run mode), this test starts with nothing installed, so it actually exercises the
// ensureToolInstalled auto-install branch inside RunExecCommandWithOptions.
//
// This intentionally does not wrap the call in captureCleanTestOutput like the sibling test
// above: ensureToolInstalled's auto-install path always runs with showProgressBar=true, and
// its spinner/progress bar goroutine is not guaranteed to fully unwind by the time this
// function returns. Redirecting os.Stderr through a short-lived os.Pipe (as
// captureCleanTestOutput does) around that window risks the OS reusing the pipe's file
// descriptor number for the *next* test's own redirect before the spinner goroutine is done,
// leaking stray bytes into that unrelated test's captured output. The "Would execute" /
// resolved-path message text is already covered by TestRunExecCommandWithOptions_DryRun
// above; this test's unique value is the auto-install side effects below.
func TestRunExecCommandWithOptions_DryRun_AutoInstallsMissingTool(t *testing.T) {
	setupTestIO(t)
	resetExecMock()
	execFunc = mockExec // Swap the real exec function so a regression would be caught, not silently run.

	tempDir := t.TempDir()
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{InstallPath: filepath.Join(tempDir, ".tools")}})

	fake := &fakeInstaller{
		resolveOwner: "hashicorp",
		resolveRepo:  "terraform",
		binaryPath:   "/fake/path/terraform",
		binaryErr:    nil,
	}

	args := []string{"terraform@1.13.1", "--version"}

	binaryName := installer.EnsureWindowsExeExtension("terraform")
	expectedPath := filepath.Join(tempDir, ".tools", "bin", "hashicorp", "terraform", "1.13.1", binaryName)

	// Sanity check: nothing is installed yet.
	if _, err := os.Stat(expectedPath); !os.IsNotExist(err) {
		t.Fatalf("expected binary to not exist before the test runs, got err=%v", err)
	}

	err := RunExecCommandWithOptions(fake, args, true)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if calledExecPath != "" {
		t.Errorf("execFunc must not be invoked in dry-run mode, got exec path %q", calledExecPath)
	}
	if _, statErr := os.Stat(expectedPath); statErr != nil {
		t.Errorf("expected dry-run to auto-install the missing binary at %q, stat error: %v", expectedPath, statErr)
	}
}
