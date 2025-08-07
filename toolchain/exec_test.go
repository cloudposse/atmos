package toolchain

import (
	"os"
	"reflect"
	"testing"
)

type mockToolRunner struct {
	binaryPath string
}

type mockResolver struct{}

func (m *mockResolver) Resolve(toolName string) (string, string, error) {
	return "test-owner", "test-repo", nil
}

func (m *mockToolRunner) findBinaryPath(owner, repo, version string) (string, error) {
	return m.binaryPath, nil
}
func (m *mockToolRunner) GetResolver() ToolResolver                          { return &mockResolver{} }
func (m *mockToolRunner) createLatestFile(owner, repo, version string) error { return nil }
func (m *mockToolRunner) readLatestFile(owner, repo string) (string, error)  { return "", nil }

func TestExecToolWithInstaller_CallsExecFunc(t *testing.T) {
	// Create a temporary file that actually exists
	tmpFile, err := os.CreateTemp("", "fake-tool-*")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// Save and restore execFunc
	origExecFunc := execFunc
	defer func() { execFunc = origExecFunc }()

	called := false
	var gotPath string
	var gotArgs []string
	var gotEnv []string
	execFunc = func(path string, args []string, env []string) error {
		called = true
		gotPath = path
		gotArgs = args
		gotEnv = env
		return nil
	}

	mockBin := tmpFile.Name()
	installer := &mockToolRunner{binaryPath: mockBin}
	args := []string{"fake-tool@1.2.3", "--foo", "bar"}
	err = execToolWithInstaller(installer, nil, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("execFunc was not called")
	}
	if gotPath != mockBin {
		t.Errorf("execFunc called with wrong path: got %q, want %q", gotPath, mockBin)
	}
	wantArgs := []string{mockBin, "--foo", "bar"}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Errorf("execFunc called with wrong args: got %v, want %v", gotArgs, wantArgs)
	}
	if len(gotEnv) == 0 || os.Getenv("PATH") == "" {
		t.Error("execFunc called with empty or missing environment")
	}
}
