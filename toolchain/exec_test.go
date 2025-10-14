package toolchain

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
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

func (f *fakeInstaller) FindBinaryPath(owner, repo, version string) (string, error) {
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

	fake := &fakeInstaller{
		resolveOwner: "hashicorp",
		resolveRepo:  "terraform",
		binaryPath:   "/fake/path/terraform",
		binaryErr:    nil,
	}

	args := []string{"terraform@1.13.1", "--version"}
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{}})
	AddToolToVersions(GetToolVersionsFilePath(), "terraform", "1.13.1")
	t.Log(ensureToolInstalled("terraform@1.13.1"))
	err := RunExecCommand(fake, args)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	expected := filepath.FromSlash(".tools/bin/hashicorp/terraform/1.13.1/terraform")
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
	if err == nil || err.Error() != "no arguments provided. Expected format: tool@version" {
		t.Fatalf("expected missing args error, got %v", err)
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
