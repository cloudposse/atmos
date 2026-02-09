package tests

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/cloudposse/atmos/cmd"
)

func TestExecuteDescribeComponentCmd_Success_YAMLWithPager(t *testing.T) {
	// This test uses a fixture that downloads from GitHub, so check rate limits first.
	RequireGitHubAccess(t)

	// Capture stdout/stderr to prevent test output pollution.
	origStdout := os.Stdout
	origStderr := os.Stderr
	defer func() {
		os.Stdout = origStdout
		os.Stderr = origStderr
	}()

	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	os.Stdout = wOut
	os.Stderr = wErr

	t.Chdir("./fixtures/scenarios/atmos-include-yaml-function")

	// Use SetArgs for Cobra command testing.
	cmd.RootCmd.SetArgs([]string{"describe", "component", "component-1", "--stack", "nonprod", "--pager=more", "--format", "yaml"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("failed to execute command: %v", err)
	}

	// Close writers and read captured output.
	wOut.Close()
	wErr.Close()
	var bufOut, bufErr bytes.Buffer
	io.Copy(&bufOut, rOut)
	io.Copy(&bufErr, rErr)

	// Optionally validate output here if needed.
	if bufOut.Len() == 0 {
		t.Error("expected output but got none")
	}
}
