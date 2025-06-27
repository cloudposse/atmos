package tests

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/cmd"
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
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	os.Args = []string{"atmos", "validate", "schema"}
	err = cmd.Execute()
	if err != nil {
		t.Fatalf("Failed to execute 'ExecuteTerraform': %v", err)
	}
	// Restore stdout
	err = w.Close()
	assert.NoError(t, err)
	os.Stderr = oldStderr

	// Read the captured output
	var buf bytes.Buffer
	_, err = buf.ReadFrom(r)
	if err != nil {
		t.Fatalf("Failed to read from pipe: %v", err)
	}
	output := buf.String()
	fmt.Println(output)
	// Check the output
	if strings.Contains(output, "ERRO") {
		t.Errorf("should have no validation errors, but got: %s", output)
	}
}
