package exec

import (
	"bytes"
	"os"
	osexec "os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestExecuteHelmfile_Version(t *testing.T) {
	// Skip if helmfile is not installed
	if _, err := osexec.LookPath("helmfile"); err != nil {
		t.Skipf("Skipping test: helmfile is not installed or not in PATH")
	}
	tests := []struct {
		name           string
		workDir        string
		expectedOutput string
	}{
		{
			name:           "helmfile version",
			workDir:        "../../tests/fixtures/scenarios/atmos-helmfile-version",
			expectedOutput: "helmfile",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture the starting working directory
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

			// Define the work directory and change to it
			if err := os.Chdir(tt.workDir); err != nil {
				t.Fatalf("Failed to change directory to %q: %v", tt.workDir, err)
			}

			// set info for ExecuteTerraform
			info := schema.ConfigAndStacksInfo{
				SubCommand: "version",
			}

			// Create a pipe to capture stdout
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			err = ExecuteHelmfile(info)
			if err != nil {
				t.Fatalf("Failed to execute 'ExecuteHelmfile': %v", err)
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

			if !strings.Contains(output, tt.expectedOutput) {
				t.Errorf("%s not found in the output", tt.expectedOutput)
			}
		})
	}
}
