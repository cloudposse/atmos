package tests

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/cmd"
	"github.com/cloudposse/atmos/pkg/utils"
	"github.com/stretchr/testify/assert"
)

func TestCliValidateSchema(t *testing.T) {
	startingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get the current working directory: %v", err)
	}

	defer func() {
		// Change back to the original working directory after the test
		if err := os.Chdir(startingDir); err != nil {
			t.Fatalf("Failed to change back to the starting directory: %v", err)
		}
	}()

	// Define the working directorwy
	workDir := "./fixtures/scenarios/schemas-validation-positive"
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("Failed to change directory to %q: %v", workDir, err)
	}

	// Create a pipe to capture stdout to check if terraform is executed correctly
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	os.Args = []string{"atmos", "validate", "schema"}
	err = cmd.Execute()
	if err != nil {
		t.Fatalf("Failed to execute 'ExecuteTerraform': %v", err)
	}
	// Restore stdout
	err = w.Close()
	assert.NoError(t, err)
	os.Stdout = oldStdout

	// Read the captured output
	var buf bytes.Buffer
	_, err = buf.ReadFrom(r)
	if err != nil {
		t.Fatalf("Failed to read from pipe: %v", err)
	}
	output := buf.String()
	fmt.Println(output)
	// Check the output
	if output != "" {
		t.Errorf("should have no validation errors, but got: %s", output)
	}
}

func TestCliValidateSchemaNegative(t *testing.T) {
	startingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get the current working directory: %v", err)
	}

	defer func() {
		// Change back to the original working directory after the test
		if err := os.Chdir(startingDir); err != nil {
			t.Fatalf("Failed to change back to the starting directory: %v", err)
		}
	}()

	// Define the working directorwy
	workDir := "./fixtures/scenarios/schemas-validation-negative"
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("Failed to change directory to %q: %v", workDir, err)
	}

	// Create a pipe to capture stdout to check if terraform is executed correctly
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	if err != nil {
		t.Fatalf("Failed to execute 'ExecuteTerraform': %v", err)
	}
	// Restore stdout
	err = w.Close()
	assert.NoError(t, err)
	os.Stdout = oldStdout
	exitCalled := false
	utils.OsExit = func(code int) {
		exitCalled = true
		if code != 1 {
			t.Errorf("Expected exit code 1, got %d", code)
		}
	}
	utils.PrintErrorMarkdownAndExitFn = func(title string, err error, suggestion string) {}
	os.Args = []string{"atmos", "validate", "schema"}
	err = cmd.Execute()

	// Read the captured output
	var buf bytes.Buffer
	_, err = buf.ReadFrom(r)
	if err != nil {
		t.Fatalf("Failed to read from pipe: %v", err)
	}
	output := buf.String()
	fmt.Println(output)
	if !exitCalled {
		t.Errorf("Expected OsExit to be called, but it wasn't")
	}
	// Check the output
	if strings.Contains(output, "name is required") {
		t.Errorf("should have no validation errors, but got: %s", output)
	}
}
