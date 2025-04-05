package tests

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/cmd"
	"github.com/cloudposse/atmos/pkg/utils"
	"github.com/stretchr/testify/assert"
)

func TestExecuteTerraform_TerraformPlanWithProcessingTemplates(t *testing.T) {
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

	// Define the working directory
	workDir := "./fixtures/scenarios/stack-templates-2"
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("Failed to change directory to %q: %v", workDir, err)
	}

	// Create a pipe to capture stdout to check if terraform is executed correctly
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	os.Args = []string{"atmos", "terraform", "plan", "component-1", "--stack", "nonprod"}
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

	// Check the output
	if !strings.Contains(output, "foo = \"component-1-a\"") {
		t.Errorf("'foo' variable should be 'component-1-a'")
	}
	if !strings.Contains(output, "bar = \"component-1-b\"") {
		t.Errorf("'bar' variable should be 'component-1-b'")
	}
	if !strings.Contains(output, "baz = \"component-1-c\"") {
		t.Errorf("'baz' variable should be 'component-1-c'")
	}
}

func TestExecuteTerraform_TerraformPlanWithoutProcessingTemplates(t *testing.T) {
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

	// Define the working directory
	workDir := "./fixtures/scenarios/stack-templates-2"
	if err = os.Chdir(workDir); err != nil {
		t.Fatalf("Failed to change directory to %q: %v", workDir, err)
	}

	// Create a pipe to capture stdout to check if terraform is executed correctly
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	os.Args = []string{"atmos", "terraform", "plan", "component-1", "--stack", "nonprod", "--process-templates=false"}
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
	// Check the output
	if !strings.Contains(output, "foo = \"{{ .settings.config.a }}\"") {
		t.Errorf("'foo' variable should be '{{ .settings.config.a }}'")
	}
	if !strings.Contains(output, "bar = \"{{ .settings.config.b }}\"") {
		t.Errorf("'bar' variable should be '{{ .settings.config.b }}'")
	}
	if !strings.Contains(output, "baz = \"{{ .settings.config.c }}\"") {
		t.Errorf("'baz' variable should be '{{ .settings.config.c }}'")
	}
}

func TestExecuteTerraform_TerraformWorkspace(t *testing.T) {
	err := os.Setenv("ATMOS_LOGS_LEVEL", "Debug")
	assert.NoError(t, err, "Setting 'ATMOS_LOGS_LEVEL' environment variable should execute without error")

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

	// Define the working directory
	workDir := "../../tests/fixtures/scenarios/stack-templates-2"
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("Failed to change directory to %q: %v", workDir, err)
	}

	// Create a pipe to capture stdout to check if terraform is executed correctly
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	os.Args = []string{"atmos", "terraform", "workspace", "component-1", "--stack", "nonprod"}
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

	// Check the output
	if !strings.Contains(output, "workspace \"nonprod-component-1\"") {
		t.Errorf("The output should contain 'nonprod-component-1'")
	}
}

func TestExecuteTerraform_TerraformPlanWithInvalidTemplates(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	// Capture the starting working directory
	startingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get the current working directory: %v", err)
	}
	oldPrintFunc := utils.PrintErrorMarkdownAndExitFn
	defer func() {
		utils.PrintErrorMarkdownAndExitFn = oldPrintFunc
	}()
	defer func() {
		// Change back to the original working directory after the test
		if err := os.Chdir(startingDir); err != nil {
			t.Fatalf("Failed to change back to the starting directory: %v", err)
		}
	}()
	errFuncationCalled := false
	utils.PrintErrorMarkdownAndExitFn = func(title string, err error, suggestion string) {
		errFuncationCalled = true
		assert.Contains(t, err.Error(), "invalid")
	}
	// Define the working directory
	workDir := "./fixtures/scenarios/invalid-stacks"
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("Failed to change directory to %q: %v", workDir, err)
	}

	os.Args = []string{"atmos", "terraform", "plan", "component-1", "--stack", "nonprod", "--skip=!terraform.output"}
	cmd.Execute()
	assert.True(t, errFuncationCalled, "Error function should be called")
}

func TestExecuteTerraform_TerraformInitWithVarfile(t *testing.T) {
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

	// Define the working directory
	workDir := "./fixtures/scenarios/terraform-init"
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("Failed to change directory to %q: %v", workDir, err)
	}

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	log.SetLevel(log.DebugLevel)
	log.SetOutput(w)
	os.Args = []string{"atmos", "terraform", "init", "component-1", "--stack", "nonprod", "--var-file", "nonprod-component-1.terraform.tfvars.json"}
	err = cmd.Execute()
	if err != nil {
		t.Fatalf("Failed to execute 'ExecuteTerraform': %v", err)
	}

	// Restore stderr
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

	// Check the output
	expected := "init -reconfigure -var-file nonprod-component-1.terraform.tfvars.json"
	if !strings.Contains(output, expected) {
		t.Logf("TestExecuteTerraform_TerraformInitWithVarfile output:\n%s", output)
		t.Errorf("Output should contain '%s'", expected)
	}
}

// TestExecuteTerraform_ExportEnvVar check that when executing the terraform apply command.
// It checks that the environment variables are correctly exported and used.
// Env var `ATMOS_BASE_PATH` and `ATMOS_CLI_CONFIG_PATH` should be exported and used in the terraform apply command.
// Check `ATMOS_BASE_PATH` and `ATMOS_CLI_CONFIG_PATH` refers to directory.
func TestExecuteTerraform_ExportEnvVar(t *testing.T) {
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
	workDir := "./fixtures/scenarios/env"
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("Failed to change directory to %q: %v", workDir, err)
	}

	// Create a pipe to capture stdout to check if terraform is executed correctly
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	os.Args = []string{"atmos", "terraform", "apply", "env-example", "--stack", "dev"}
	err = cmd.Execute()
	if err != nil {
		t.Fatalf("Failed to execute 'ExecuteTerraform': %v", err)
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

	// Check the output ATMOS_CLI_CONFIG_PATH  ATMOS_BASE_PATH exists
	if !strings.Contains(output, "ATMOS_BASE_PATH") {
		t.Errorf("ATMOS_BASE_PATH not found in the output")
	}
	if !strings.Contains(output, "ATMOS_CLI_CONFIG_PATH") {
		t.Errorf("ATMOS_CLI_CONFIG_PATH not found in the output")
	}

	// print values of ATMOS_BASE_PATH ATMOS_CLI_CONFIG_PATH from out
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

// Helper Function to extract key-value pairs from a string.
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
