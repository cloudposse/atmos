package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func executeCommand(root *cobra.Command, args ...string) (output string, err error) {
	buf := new(strings.Builder)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)
	err = root.Execute()
	return buf.String(), err
}

func TestCleanCommand_RemovesToolsDirectory(t *testing.T) {
	// Create a fake .tools directory with files in the current directory
	toolsDir := ".tools"
	binDir := filepath.Join(toolsDir, "bin")
	os.MkdirAll(binDir, 0755)
	ioutil.WriteFile(filepath.Join(binDir, "dummy-tool"), []byte("binary"), 0644)

	// Clean up after the test
	defer func() {
		os.RemoveAll(toolsDir)
	}()

	// Run the clean command
	output, err := executeCommand(rootCmd, "clean")
	if err != nil {
		t.Fatalf("clean command failed: %v", err)
	}

	// Check that .tools directory is deleted
	if _, err := os.Stat(toolsDir); !os.IsNotExist(err) {
		t.Errorf(".tools directory was not deleted")
	}

	// Check that output contains the checkMark
	if !strings.Contains(output, checkMark.Render()) {
		t.Errorf("output does not contain checkMark: %q", output)
	}

	// Check that output reports at least 1 file/directory deleted
	if !strings.Contains(output, "Deleted") || !strings.Contains(output, "files/directories") {
		t.Errorf("output does not report deleted files/directories: %q", output)
	}
}
