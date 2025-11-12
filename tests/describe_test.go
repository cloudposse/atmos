package tests

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/cloudposse/atmos/cmd"
)

func TestDescribeComponentJSON(t *testing.T) {
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

	// Set up the environment variables.
	t.Chdir("./fixtures/scenarios/atmos-providers-section")

	t.Setenv("ATMOS_PAGER", "more")

	// Use SetArgs for Cobra command testing.
	cmd.RootCmd.SetArgs([]string{"describe", "component", "component-1", "--stack", "nonprod", "--format", "json"})

	err := cmd.Execute()

	// Close writers and read captured output.
	wOut.Close()
	wErr.Close()
	var bufOut, bufErr bytes.Buffer
	io.Copy(&bufOut, rOut)
	io.Copy(&bufErr, rErr)

	if err != nil {
		t.Fatalf("Failed to execute command: %v", err)
	}

	// Optionally validate output here if needed.
	if bufOut.Len() == 0 {
		t.Error("expected output but got none")
	}
}

func TestDescribeComponentYAML(t *testing.T) {
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

	// Set up the environment variables.
	t.Chdir("./fixtures/scenarios/atmos-providers-section")

	t.Setenv("ATMOS_PAGER", "more")

	// Use SetArgs for Cobra command testing.
	cmd.RootCmd.SetArgs([]string{"describe", "component", "component-1", "--stack", "nonprod", "--format", "yaml"})

	err := cmd.Execute()

	// Close writers and read captured output.
	wOut.Close()
	wErr.Close()
	var bufOut, bufErr bytes.Buffer
	io.Copy(&bufOut, rOut)
	io.Copy(&bufErr, rErr)

	if err != nil {
		t.Fatalf("Failed to execute command: %v", err)
	}

	// Optionally validate output here if needed.
	if bufOut.Len() == 0 {
		t.Error("expected output but got none")
	}
}
