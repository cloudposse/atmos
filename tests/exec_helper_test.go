package tests

import (
	"bytes"
	"os"
	"testing"

	log "github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/cmd"
	"github.com/cloudposse/atmos/pkg/utils"
)

func executeCommand(t *testing.T, tc TestCase) (exitCode int, output bytes.Buffer, outputErr bytes.Buffer) {
	for k, v := range tc.Env {
		os.Setenv(k, v)
	}
	log.SetReportTimestamp(false)
	var stdoutBuf, stderrBuf bytes.Buffer

	// Redirect os.Stdout and os.Stderr
	oldStdout := os.Stdout
	oldStderr := os.Stderr
	defer func() {
		os.Stdout = oldStdout // Restore after test
		os.Stderr = oldStderr
	}()
	// Create pipes to capture output
	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	os.Stdout = wOut
	os.Stderr = wErr

	// Set Cobra's output to use the redirected os.Stdout and os.Stderr
	cmd.RootCmd.SetOut(os.Stdout)
	cmd.RootCmd.SetErr(os.Stderr)

	// Execute the command with arguments
	cmd.RootCmd.SetArgs(tc.Args)

	utils.OsExit = func(code int) {
		exitCode = code
		// Close the writers and read the output
		wOut.Close()
		wErr.Close()

		_, err := stdoutBuf.ReadFrom(rOut)
		if err != nil {
			t.Fatalf("Failed to read stdout: %v", err)
		}
		_, err = stderrBuf.ReadFrom(rErr)
		if err != nil {
			t.Fatalf("Failed to read stderr: %v", err)
		}
		output = stdoutBuf
		outputErr = stderrBuf
		panic("just to stop everything")
	}
	defer func() {
		recover()
		utils.OsExit = os.Exit
		return
	}()

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Command execution failed: %v", err)
	}
	// Close the writers and read the output
	wOut.Close()
	wErr.Close()

	_, err = stdoutBuf.ReadFrom(rOut)
	if err != nil {
		t.Fatalf("Failed to read stdout: %v", err)
	}
	_, err = stderrBuf.ReadFrom(rErr)
	if err != nil {
		t.Fatalf("Failed to read stderr: %v", err)
	}
	output = stdoutBuf
	outputErr = stderrBuf
	return
}
