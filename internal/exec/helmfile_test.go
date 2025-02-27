package exec

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
)

// test ExecuteHelmfile export env var ATMOS_CLI_CONFIG_PATH  ATMOS_BASE_PATH .
func TestExecuteHelmfile(t *testing.T) {

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
	workDir := "../../tests/fixtures/scenarios/env"
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("Failed to change directory to %q: %v", workDir, err)
	}
	//set info for test ExecuteHelmfile
	info := schema.ConfigAndStacksInfo{
		StackFromArg:     "",
		Stack:            "dev",
		StackFile:        "",
		ComponentType:    "terraform",
		ComponentFromArg: "env-example",
		SubCommand:       "apply",
	}
	// Create a pipe to capture stdout to check if terraform is executed correctly
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	err = ExecuteTerraform(info)
	if err != nil {
		t.Fatalf("Failed to execute helmfile: %v", err)
	}
	// Restore stdout
	w.Close()
	os.Stdout = oldStdout

	// Read the captured output
	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()
	// Check the output ATMOS_CLI_CONFIG_PATH  ATMOS_BASE_PATH exists
	if !strings.Contains(output, "ATMOS_BASE_PATH") {
		t.Errorf("ATMOS_BASE_PATH not found in the output")
	}
	if !strings.Contains(output, "ATMOS_CLI_CONFIG_PATH") {
		t.Errorf("ATMOS_CLI_CONFIG_PATH not found in the output")
	}
	// print values of ATMOS_BASE_PATH ATMOS_CLI_CONFIG_PATH from out
	//atmos_base_path = "/home/haitham/go_projects/src/atmos/tests/fixtures/scenarios/env"
	//  atmos_cli_config_path = "/home/haitham/go_projects/src/atmos/tests/fixtures/scenarios/env"
	m := extractKeyValuePairs(output)
	// Print the extracted values
	basePath, ok := m["atmos_base_path"]
	if !ok {
		t.Errorf("atmos_base_path not found in the output")
	}
	configPath, ok := m["atmos_cli_config_path"]
	if !ok {
		t.Errorf("atmos_cli_config_path not found in the output")
	}
	statBase, err := os.Stat(basePath)
	if err != nil {
		t.Errorf("Failed to stat atmos_base_path: %v", err)
	}
	// check bathPath is Dir
	if !statBase.IsDir() {
		t.Errorf("atmos_base_path is not a directory")
	}

	// configPath path is Dir
	statConfigPath, err := os.Stat(configPath)
	if err != nil {
		t.Errorf("Failed to stat atmos_cli_config_path: %v", err)
	}
	if !statConfigPath.IsDir() {
		t.Errorf("atmos_cli_config_path is not a directory")
	}
	t.Logf("atmos_base_path: %s", basePath)
	t.Logf("atmos_cli_config_path: %s", configPath)
}

// Function to extract key-value pairs from a string
func extractKeyValuePairs(input string) map[string]string {
	// Split the input into lines
	lines := strings.Split(input, "\n")

	// Create a map to store key-value pairs
	config := make(map[string]string)

	for _, line := range lines {
		// Trim whitespace and skip empty lines
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Split the line by "="
		parts := strings.Split(line, "=")
		if len(parts) != 2 {
			continue
		}

		// Extract key and value
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Remove surrounding quotes from the value
		value = strings.Trim(value, `"`)

		// Store in the map
		config[key] = value
	}

	return config
}
