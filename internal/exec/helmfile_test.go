package exec

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	log "github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestExecuteHelmfile_Version(t *testing.T) {
	// Set log level to debug
	log.SetLevel(log.DebugLevel)
	log.SetOutput(os.Stdout)

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
			log.Debug("Starting directory", "dir", startingDir)

			defer func() {
				// Change back to the original working directory after the test
				if err := os.Chdir(startingDir); err != nil {
					t.Fatalf("Failed to change back to the starting directory: %v", err)
				}
			}()

			// Get absolute path to work directory
			workDirAbs, err := filepath.Abs(tt.workDir)
			if err != nil {
				t.Fatalf("Failed to get absolute path for work directory: %v", err)
			}
			log.Debug("Source work directory", "dir", workDirAbs)

			// Change to the work directory
			if err := os.Chdir(workDirAbs); err != nil {
				t.Fatalf("Failed to change directory to %q: %v", workDirAbs, err)
			}

			// Log current working directory for debugging
			currentDir, err := os.Getwd()
			if err != nil {
				t.Fatalf("Failed to get current working directory: %v", err)
			}
			log.Debug("Current working directory", "dir", currentDir)

			// List contents of current directory
			entries, err := os.ReadDir(currentDir)
			if err != nil {
				t.Fatalf("Failed to read directory contents: %v", err)
			}
			log.Debug("Directory contents", "files", func() []string {
				var names []string
				for _, entry := range entries {
					names = append(names, entry.Name())
				}
				return names
			}())

			// Verify the stacks directory exists
			stacksDir := filepath.Join(currentDir, "stacks")
			if _, err := os.Stat(stacksDir); os.IsNotExist(err) {
				t.Fatalf("Stacks directory does not exist at %q", stacksDir)
			}

			// List contents of stacks directory
			stackEntries, err := os.ReadDir(stacksDir)
			if err != nil {
				t.Fatalf("Failed to read stacks directory contents: %v", err)
			}
			log.Debug("Stacks directory contents", "files", func() []string {
				var names []string
				for _, entry := range stackEntries {
					names = append(names, entry.Name())
				}
				return names
			}())

			// set info for ExecuteHelmfile
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
			w.Close()
			os.Stdout = oldStdout

			// Read the captured output
			var buf bytes.Buffer
			_, err = buf.ReadFrom(r)
			if err != nil {
				t.Fatalf("Failed to read from pipe: %v", err)
			}
			output := buf.String()
			log.Debug("Command output", "output", output)

			if !strings.Contains(output, tt.expectedOutput) {
				t.Errorf("%s not found in the output", tt.expectedOutput)
			}
		})
	}
}
