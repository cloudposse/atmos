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
	// Define the working directorwy
	workDir := "./fixtures/scenarios/schemas-validation-positive"
	t.Chdir(workDir)

	// Create a pipe to capture stderr to check if validation is executed correctly
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	// Use SetArgs for Cobra command testing.
	cmd.RootCmd.SetArgs([]string{"validate", "schema"})

	err := cmd.Execute()
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
