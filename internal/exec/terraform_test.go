package exec

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/tests"
)

// TestExecuteTerraform_ExportEnvVar check that when executing the terraform apply command.
// It checks that the environment variables are correctly exported and used.
// Env var `ATMOS_BASE_PATH` and `ATMOS_CLI_CONFIG_PATH` should be exported and used in the terraform apply command.
// Check that `ATMOS_BASE_PATH` and `ATMOS_CLI_CONFIG_PATH` point to a directory.
func TestExecuteTerraform_ExportEnvVar(t *testing.T) {
	// Skip if terraform is not installed
	tests.RequireTerraform(t)
	// Capture the starting working directory
	startingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get the current working directory: %v", err)
	}

	// Clean up any leftover terraform files from previous test runs to avoid conflicts
	componentPath := filepath.Join(startingDir, "..", "..", "tests", "fixtures", "components", "terraform", "env-example")
	cleanupFiles := []string{
		filepath.Join(componentPath, ".terraform"),
		filepath.Join(componentPath, ".terraform.lock.hcl"),
		filepath.Join(componentPath, "terraform.tfstate.d"),
		filepath.Join(componentPath, "backend.tf.json"),
	}

	// Clean before test
	for _, path := range cleanupFiles {
		os.RemoveAll(path)
	}

	// Also look for and remove any .tfvars.json files
	matches, _ := filepath.Glob(filepath.Join(componentPath, "*.terraform.tfvars.json"))
	for _, match := range matches {
		os.Remove(match)
	}

	defer func() {
		// Clean up after test
		for _, path := range cleanupFiles {
			os.RemoveAll(path)
		}
		matches, _ := filepath.Glob(filepath.Join(componentPath, "*.terraform.tfvars.json"))
		for _, match := range matches {
			os.Remove(match)
		}

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

	// set info for ExecuteTerraform
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

func TestExecuteTerraform_TerraformPlanWithProcessingTemplates(t *testing.T) {
	// Skip if terraform is not installed
	tests.RequireTerraform(t)
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

	info := schema.ConfigAndStacksInfo{
		StackFromArg:     "",
		Stack:            "nonprod",
		StackFile:        "",
		ComponentType:    "terraform",
		ComponentFromArg: "component-1",
		SubCommand:       "plan",
		ProcessTemplates: true,
		ProcessFunctions: true,
	}

	// Create a pipe to capture stdout to check if terraform is executed correctly
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	err = ExecuteTerraform(info)
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
	if !strings.Contains(output, "component-1-a") {
		t.Errorf("'foo' variable should be 'component-1-a'")
	}
	if !strings.Contains(output, "component-1-b") {
		t.Errorf("'bar' variable should be 'component-1-b'")
	}
	if !strings.Contains(output, "component-1-c") {
		t.Errorf("'baz' variable should be 'component-1-c'")
	}
}

func TestExecuteTerraform_TerraformPlanWithoutProcessingTemplates(t *testing.T) {
	// Skip if terraform is not installed
	tests.RequireTerraform(t)
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
	if err = os.Chdir(workDir); err != nil {
		t.Fatalf("Failed to change directory to %q: %v", workDir, err)
	}

	info := schema.ConfigAndStacksInfo{
		StackFromArg:     "",
		Stack:            "nonprod",
		StackFile:        "",
		ComponentType:    "terraform",
		ComponentFromArg: "component-1",
		SubCommand:       "plan",
		ProcessTemplates: false,
		ProcessFunctions: true,
	}

	// Create a pipe to capture stdout to check if terraform is executed correctly
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	err = ExecuteTerraform(info)
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

	t.Log(output)

	// Check the output
	if !strings.Contains(output, "{{ .settings.config.a }}") {
		t.Errorf("'foo' variable should be '{{ .settings.config.a }}'")
	}
	if !strings.Contains(output, "{{ .settings.config.b }}") {
		t.Errorf("'bar' variable should be '{{ .settings.config.b }}'")
	}
	if !strings.Contains(output, "{{ .settings.config.c }}") {
		t.Errorf("'baz' variable should be '{{ .settings.config.c }}'")
	}
}

func TestExecuteTerraform_TerraformWorkspace(t *testing.T) {
	// Skip if terraform is not installed.
	tests.RequireTerraform(t)
	t.Setenv("ATMOS_LOGS_LEVEL", "Debug")

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

	info := schema.ConfigAndStacksInfo{
		StackFromArg:     "",
		Stack:            "nonprod",
		StackFile:        "",
		ComponentType:    "terraform",
		ComponentFromArg: "component-1",
		SubCommand:       "workspace",
		ProcessTemplates: true,
		ProcessFunctions: true,
	}

	// Create a pipe to capture stdout to check if terraform is executed correctly
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	err = ExecuteTerraform(info)
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
	// Skip if terraform is not installed
	tests.RequireTerraform(t)
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
	workDir := "../../tests/fixtures/scenarios/invalid-stacks"
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("Failed to change directory to %q: %v", workDir, err)
	}

	info := schema.ConfigAndStacksInfo{
		StackFromArg:     "",
		Stack:            "nonprod",
		StackFile:        "",
		ComponentType:    "terraform",
		ComponentFromArg: "component-1",
		SubCommand:       "plan",
		ProcessTemplates: true,
		ProcessFunctions: true,
		Skip:             []string{"!terraform.output"},
	}

	err = ExecuteTerraform(info)
	assert.Error(t, err)
	assert.ErrorContains(t, err, "invalid")
}

func TestExecuteTerraform_TerraformInitWithVarfile(t *testing.T) {
	// Skip if terraform is not installed
	tests.RequireTerraform(t)
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
	workDir := "../../tests/fixtures/scenarios/terraform-init"
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("Failed to change directory to %q: %v", workDir, err)
	}

	info := schema.ConfigAndStacksInfo{
		StackFromArg:     "",
		Stack:            "nonprod",
		StackFile:        "",
		ComponentType:    "terraform",
		ComponentFromArg: "component-1",
		SubCommand:       "init",
		ProcessTemplates: true,
		ProcessFunctions: true,
	}

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	log.SetLevel(log.DebugLevel)
	log.SetOutput(w)

	err = ExecuteTerraform(info)
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

func TestExecuteTerraform_OpaValidation(t *testing.T) {
	// Skip if terraform is not installed
	tests.RequireTerraform(t)
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
	workDir := "../../tests/fixtures/scenarios/atmos-stacks-validation"
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("Failed to change directory to %q: %v", workDir, err)
	}

	// Test `terraform plan`
	info := schema.ConfigAndStacksInfo{
		StackFromArg:     "",
		Stack:            "nonprod",
		StackFile:        "",
		ComponentType:    "terraform",
		ComponentFromArg: "component-1",
		SubCommand:       "plan",
		ProcessTemplates: true,
		ProcessFunctions: true,
	}
	err = ExecuteTerraform(info)
	assert.NoError(t, err)

	// Test `terraform apply`
	info.SubCommand = "apply"
	err = ExecuteTerraform(info)
	assert.ErrorContains(t, err, "the component can't be applied if the 'foo' variable is set to 'foo'")
}

func TestExecuteTerraform_Version(t *testing.T) {
	// Skip if terraform is not installed
	tests.RequireTerraform(t)
	tests := []struct {
		name           string
		workDir        string
		expectedOutput string
	}{
		{
			name:           "terraform version",
			workDir:        "../../tests/fixtures/scenarios/atmos-terraform-version",
			expectedOutput: "Terraform v",
		},
		{
			name:           "tofu version",
			workDir:        "../../tests/fixtures/scenarios/atmos-tofu-version",
			expectedOutput: "OpenTofu v",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set info for ExecuteTerraform.
			info := schema.ConfigAndStacksInfo{
				SubCommand: "version",
			}

			testCaptureCommandOutput(t, tt.workDir, func() error {
				return ExecuteTerraform(info)
			}, tt.expectedOutput)
		})
	}
}

func TestExecuteTerraform_TerraformPlanWithSkipPlanfile(t *testing.T) {
	// Skip if terraform is not installed
	tests.RequireTerraform(t)
	workDir := "../../tests/fixtures/scenarios/terraform-cloud"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", workDir)
	t.Setenv("ATMOS_BASE_PATH", workDir)
	t.Setenv("ATMOS_LOGS_LEVEL", "Debug")

	info := schema.ConfigAndStacksInfo{
		StackFromArg:     "",
		Stack:            "nonprod",
		StackFile:        "",
		ComponentType:    "terraform",
		ComponentFromArg: "cmp-1",
		SubCommand:       "plan",
		ProcessTemplates: true,
		ProcessFunctions: true,
	}

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	log.SetLevel(log.DebugLevel)

	// Create a buffer to capture the output
	var buf bytes.Buffer
	log.SetOutput(&buf)

	err := ExecuteTerraform(info)
	if err != nil {
		t.Fatalf("Failed to execute 'ExecuteTerraform': %v", err)
	}
	// Restore stderr
	err = w.Close()
	assert.NoError(t, err)
	os.Stderr = oldStderr

	// Read the captured output
	_, err = buf.ReadFrom(r)
	if err != nil {
		t.Fatalf("Failed to read from pipe: %v", err)
	}
	output := buf.String()

	// Check the output
	expected := "plan -var-file nonprod-cmp-1.terraform.tfvars.json"
	notExpected := "-out nonprod-cmp-1.planfile"

	if !strings.Contains(output, expected) {
		t.Logf("TestExecuteTerraform_TerraformPlanWithSkipPlanfile output:\n%s", output)
		t.Errorf("Output should contain '%s'", expected)
	}

	if strings.Contains(output, notExpected) {
		t.Logf("TestExecuteTerraform_TerraformPlanWithSkipPlanfile output:\n%s", output)
		t.Errorf("Output should not contain '%s'", notExpected)
	}
}

func TestExecuteTerraform_DeploymentStatus(t *testing.T) {
	startingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get the current working directory: %v", err)
	}
	defer func() {
		if err := os.Chdir(startingDir); err != nil {
			t.Fatalf("Failed to change back to the starting directory: %v", err)
		}
	}()

	workDir := "../../tests/fixtures/scenarios/atmos-pro"
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("Failed to change directory to %q: %v", workDir, err)
	}

	// Set up test environment.
	t.Setenv("ATMOS_LOGS_LEVEL", "Debug")

	testCases := []struct {
		name              string
		stack             string
		component         string
		uploadStatus      bool
		proEnabled        bool
		checkProWarning   bool
		checkDetailedExit bool
		exitCode          int
	}{
		{
			name:              "drift results enabled and pro disabled",
			stack:             "nonprod",
			component:         "mock/disabled",
			uploadStatus:      true,
			proEnabled:        false,
			checkProWarning:   true,
			checkDetailedExit: true,
			exitCode:          0,
		},
		{
			name:              "drift results enabled and pro enabled with drift",
			stack:             "nonprod",
			component:         "mock/drift",
			uploadStatus:      true,
			proEnabled:        true,
			checkProWarning:   false,
			checkDetailedExit: true,
			exitCode:          2, // Simulate drift detected
		},
		{
			name:              "drift results enabled and pro enabled without drift",
			stack:             "nonprod",
			component:         "mock/nodrift",
			uploadStatus:      true,
			proEnabled:        true,
			checkProWarning:   false,
			checkDetailedExit: true,
			exitCode:          0, // Simulate no drift
		},
		{
			name:              "drift results enabled and pro enabled with drift in prod",
			stack:             "prod",
			component:         "mock/drift",
			uploadStatus:      true,
			proEnabled:        true,
			checkProWarning:   false,
			checkDetailedExit: true,
			exitCode:          2, // Simulate drift detected
		},
		{
			name:              "drift results enabled and pro enabled without drift in prod",
			stack:             "prod",
			component:         "mock/nodrift",
			uploadStatus:      true,
			proEnabled:        true,
			checkProWarning:   false,
			checkDetailedExit: true,
			exitCode:          0, // Simulate no drift
		},
		{
			name:              "upload status explicitly disabled",
			stack:             "nonprod",
			component:         "mock/nodrift",
			uploadStatus:      false,
			proEnabled:        true,
			checkProWarning:   false,
			checkDetailedExit: false,
			exitCode:          0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create test directories
			stackDir := filepath.Join(workDir, "stacks", tc.stack)
			if err := os.MkdirAll(stackDir, 0o755); err != nil {
				t.Fatalf("Failed to create stack dir: %v", err)
			}

			// Create component directory
			componentDir := filepath.Join(workDir, "components", "terraform", tc.component)
			if err := os.MkdirAll(componentDir, 0o755); err != nil {
				t.Fatalf("Failed to create component dir: %v", err)
			}

			// Create stack file
			stackFile := filepath.Join(stackDir, "mock.yaml")
			stackContent := fmt.Sprintf("components:\n  terraform:\n    %s:\n      settings:\n        pro:\n          enabled: %v\n      vars:\n        foo: %s-a\n        bar: %s-b\n        baz: %s-c",
				tc.component, tc.proEnabled, tc.component, tc.component, tc.component)
			if err := os.WriteFile(stackFile, []byte(stackContent), 0o644); err != nil {
				t.Fatalf("Failed to write stack file: %v", err)
			}
			defer os.Remove(stackFile)

			// Create a minimal terraform configuration
			mainTf := filepath.Join(componentDir, "main.tf")
			mainTfContent := `output "foo" { value = "test" }`
			if err := os.WriteFile(mainTf, []byte(mainTfContent), 0o644); err != nil {
				t.Fatalf("Failed to write main.tf: %v", err)
			}
			defer os.Remove(mainTf)

			info := schema.ConfigAndStacksInfo{
				Stack:            tc.stack,
				ComponentType:    "terraform",
				ComponentFromArg: tc.component,
				SubCommand:       "plan",
				ProcessTemplates: true,
				ProcessFunctions: true,
			}
			if tc.uploadStatus {
				info.AdditionalArgsAndFlags = append(info.AdditionalArgsAndFlags, "--upload-status")
			} else {
				info.AdditionalArgsAndFlags = append(info.AdditionalArgsAndFlags, "--upload-status=false")
			}

			// Create a pipe to capture stdout and stderr
			oldStdout := os.Stdout
			oldStderr := os.Stderr
			r, w, _ := os.Pipe()
			os.Stdout = w
			os.Stderr = w

			// Save original logger and set up test logger
			originalLogger := log.Default()
			logger := log.New()
			logger.SetOutput(w)
			log.SetDefault(logger)
			defer log.SetDefault(originalLogger)

			// Create a channel to signal when the pipe is closed
			done := make(chan struct{})
			go func() {
				defer close(done)
				defer w.Close()
				_ = ExecuteTerraform(info)
			}()

			// Read the output
			var buf bytes.Buffer
			_, err = buf.ReadFrom(r)
			if err != nil {
				t.Fatalf("Failed to read from pipe: %v", err)
			}
			output := buf.String()

			// Restore stdout, stderr, and logger
			os.Stdout = oldStdout
			os.Stderr = oldStderr
			log.SetDefault(log.Default())

			// Wait for the command to finish
			<-done

			// Check the output for drift/no drift and pro warning
			assert.Contains(t, output, "Changes to Outputs", "Expected 'Changes to Outputs' in output")
			if tc.checkProWarning {
				assert.Contains(t, output, "Pro is not enabled. Skipping upload of Terraform result.")
			} else {
				assert.NotContains(t, output, "Pro is not enabled. Skipping upload of Terraform result.")
			}
		})
	}
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

// TestExecuteTerraform_OpaValidationFunctionality tests the OPA validation functionality by using validate component directly.
func TestExecuteTerraform_OpaValidationFunctionality(t *testing.T) {
	// Capture the starting working directory.
	startingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get the current working directory: %v", err)
	}

	defer func() {
		// Change back to the original working directory after the test.
		if err := os.Chdir(startingDir); err != nil {
			t.Fatalf("Failed to change back to the starting directory: %v", err)
		}
	}()

	// Define the working directory.
	workDir := "../../tests/fixtures/scenarios/atmos-stacks-validation"
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("Failed to change directory to %q: %v", workDir, err)
	}

	tests := []struct {
		name          string
		component     string
		stack         string
		envVars       map[string]string
		shouldFail    bool
		expectedError string
		description   string
	}{
		{
			name:        "test process_env validation - should pass",
			component:   "component-test-process-env",
			stack:       "nonprod",
			envVars:     map[string]string{"ATMOS_TEST_VAR": "test_value"},
			shouldFail:  false,
			description: "Test that process_env section is properly populated and validated",
		},
		{
			name:          "test process_env validation - should fail when ATMOS_TEST_VAR missing",
			component:     "component-test-process-env",
			stack:         "nonprod",
			envVars:       map[string]string{},
			shouldFail:    true,
			expectedError: "ATMOS_TEST_VAR environment variable is missing from process_env in test mode",
			description:   "Test that validation fails when required env var is missing",
		},
		{
			name:        "test cli_args validation - should pass",
			component:   "component-test-cli-args",
			stack:       "nonprod",
			shouldFail:  false,
			description: "Test that cli_args section contains proper terraform command structure",
		},
		{
			name:        "test tf_cli_vars validation with TF_CLI_ARGS variables",
			component:   "component-test-tf-cli-vars",
			stack:       "nonprod",
			envVars:     map[string]string{"TF_CLI_ARGS": "-var test_var=test_value -var count=5"},
			shouldFail:  false,
			description: "Test that tf_cli_vars are properly parsed from TF_CLI_ARGS",
		},
		{
			name:          "test tf_cli_vars validation - should fail when test_var missing",
			component:     "component-test-tf-cli-vars",
			stack:         "nonprod",
			envVars:       map[string]string{"TF_CLI_ARGS": "-var other_var=other_value"},
			shouldFail:    true,
			expectedError: "test_var is missing from env_tf_cli_vars when test_tf_cli_vars is enabled",
			description:   "Test that validation fails when expected test_var is missing from TF_CLI_ARGS",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment variables for this test using t.Setenv for automatic cleanup.
			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}

			// Test validation directly using ExecuteValidateComponent instead of ExecuteTerraform
			// to avoid TF_CLI_ARGS conflicts with actual terraform execution
			info := schema.ConfigAndStacksInfo{
				ComponentFromArg: tt.component,
				Stack:            tt.stack,
				ComponentType:    "terraform",
			}

			// Initialize Atmos config
			atmosConfig, err := cfg.InitCliConfig(info, true)
			if err != nil {
				t.Fatalf("Failed to initialize Atmos config: %v", err)
			}

			// Execute validation directly
			_, err = ExecuteValidateComponent(&atmosConfig, info, tt.component, tt.stack, "", "", []string{}, 0)

			if tt.shouldFail {
				assert.Error(t, err, "Expected test to fail for %s", tt.description)
				if tt.expectedError != "" {
					assert.ErrorContains(t, err, tt.expectedError, "Expected specific error message for %s", tt.description)
				}
			} else {
				assert.NoError(t, err, "Expected test to pass for %s", tt.description)
			}
		})
	}
}
