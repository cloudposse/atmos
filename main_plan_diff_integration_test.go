package main

import (
	"os"
	"path"
	"runtime"
	"testing"
	"time"

	u "github.com/cloudposse/atmos/pkg/utils"
)

func TestMainTerraformPlanDiffIntegration(t *testing.T) {
	// We need to intercept calls to os.Exit so the test doesn't fail
	oldOsExit := u.OsExit
	defer func() { u.OsExit = oldOsExit }()

	// Create a channel to communicate the exit code
	exitCodeCh := make(chan int, 1)

	// Mock the OsExit function to capture the exit code
	u.OsExit = func(code int) {
		t.Logf("Exit code set to: %d", code)
		exitCodeCh <- code
		// Do not actually exit the process
	}

	// Helper function to run main and get the exit code
	runMainWithExitCode := func() int {
		// Clear the channel
		select {
		case <-exitCodeCh:
			// Drain any previous value
		default:
			// Channel is empty
		}

		// Create a done channel to signal when main has completed
		done := make(chan struct{})

		// Run main in a goroutine
		go func() {
			defer close(done)
			main()
			// If main returns without calling OsExit, send 0
			select {
			case exitCodeCh <- 0:
			default:
				// Channel already has a value, which means OsExit was called
			}
		}()

		// Handle Windows specially - just wait for exit code
		if runtime.GOOS == "windows" {
			return <-exitCodeCh
		}

		// For non-Windows platforms, use the original logic
		select {
		case code := <-exitCodeCh:
			<-done // Wait for main to finish on non-Windows platforms
			return code
		case <-done:
			// Main completed without calling OsExit
			return <-exitCodeCh
		}
	}

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current working directory: %v", err)
	}
	defer os.Chdir(origDir)

	// Change to the tests/fixtures/scenarios/plan-diff directory
	if err := os.Chdir("tests/fixtures/scenarios/plan-diff"); err != nil {
		t.Fatalf("failed to change to tests/fixtures/scenarios/plan-diff directory: %v", err)
	}

	// Capture the original arguments
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	// Create a temporary directory for plan files
	tmpDir, err := os.MkdirTemp("", "atmos-plan-diff-test")
	if err != nil {
		t.Fatalf("failed to create temporary directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	origPlanFile := path.Join(tmpDir, "orig.plan")
	newPlanFile := path.Join(tmpDir, "new.plan")

	// Generate the original plan
	os.Args = []string{"atmos", "terraform", "plan", "component-1", "-s", "nonprod", "-out=" + origPlanFile}
	exitCode := runMainWithExitCode()
	t.Logf("After first plan, exit code: %d", exitCode)
	if exitCode != 0 {
		t.Fatalf("plan command failed with exit code %d", exitCode)
	}

	// Generate a new plan with a different variable
	os.Args = []string{"atmos", "terraform", "plan", "component-1", "-s", "nonprod", "-out=" + newPlanFile, "-var", "foo=new-value"}
	exitCode = runMainWithExitCode()
	t.Logf("After second plan, exit code: %d", exitCode)
	if exitCode != 0 {
		t.Fatalf("plan command with variable failed with exit code %d", exitCode)
	}

	// Run the plan-diff command
	os.Args = []string{"atmos", "terraform", "plan-diff", "component-1", "-s", "nonprod", "--orig=" + origPlanFile, "--new=" + newPlanFile}
	exitCode = runMainWithExitCode()
	t.Logf("After plan-diff, exit code: %d", exitCode)

	// The plan-diff command should set the exit code to 2 when plans are different
	if exitCode != 2 {
		t.Fatalf("plan-diff command should have returned exit code 2, got %d", exitCode)
	}

	// Add a small delay to ensure Windows file operations are complete
	time.Sleep(500 * time.Millisecond)

	// Test with generating a new plan on the fly
	os.Args = []string{"atmos", "terraform", "plan-diff", "component-1", "-s", "nonprod", "--orig=" + origPlanFile, "-var", "foo=new-value"}
	exitCode = runMainWithExitCode()
	t.Logf("After on-the-fly plan-diff, exit code: %d", exitCode)

	// The plan-diff command should set the exit code to 2 when plans are different
	if exitCode != 2 {
		t.Fatalf("plan-diff command with on-the-fly plan should have returned exit code 2, got %d", exitCode)
	}
}
