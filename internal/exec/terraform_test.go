package exec

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
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
	// Clean up any leftover terraform files from previous test runs to avoid conflicts
	startingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get the current working directory: %v", err)
	}
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
	}()

	// Define the work directory and change to it
	workDir := "../../tests/fixtures/scenarios/env"
	t.Chdir(workDir)

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

	// Define the working directory
	workDir := "../../tests/fixtures/scenarios/stack-templates-2"
	t.Chdir(workDir)

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
	err := ExecuteTerraform(info)
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

	// Define the working directory
	workDir := "../../tests/fixtures/scenarios/stack-templates-2"
	t.Chdir(workDir)

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
	err := ExecuteTerraform(info)
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

	// Define the working directory
	workDir := "../../tests/fixtures/scenarios/stack-templates-2"
	t.Chdir(workDir)

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
	err := ExecuteTerraform(info)
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

	// Define the working directory
	workDir := "../../tests/fixtures/scenarios/invalid-stacks"
	t.Chdir(workDir)

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

	err := ExecuteTerraform(info)
	assert.Error(t, err)
	assert.ErrorContains(t, err, "invalid")
}

func TestExecuteTerraform_TerraformInitWithVarfile(t *testing.T) {
	// Skip if terraform is not installed
	tests.RequireTerraform(t)

	// Define the working directory
	workDir := "../../tests/fixtures/scenarios/terraform-init"
	t.Chdir(workDir)

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

	err := ExecuteTerraform(info)
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

	// Define the working directory
	workDir := "../../tests/fixtures/scenarios/atmos-stacks-validation"
	t.Chdir(workDir)

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
	err := ExecuteTerraform(info)
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
	workDir := "../../tests/fixtures/scenarios/atmos-pro"
	t.Chdir(workDir)

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
			_, err := buf.ReadFrom(r)
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
				assert.Contains(t, output, "Atmos Pro is not enabled for this component. Skipping upload of Terraform plan result.")
			} else {
				assert.NotContains(t, output, "Atmos Pro is not enabled for this component. Skipping upload of Terraform plan result.")
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
	// Define the working directory.
	workDir := "../../tests/fixtures/scenarios/atmos-stacks-validation"
	t.Chdir(workDir)

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

// TestExecuteTerraform_AuthPreHookErrorPropagation verifies that errors from the auth pre-hook
// are properly propagated and cause terraform execution to abort.
// This ensures that when authentication fails (e.g., user presses Ctrl+C during SSO),
// the terraform command does not continue executing.
//
// This test verifies the fix in terraform.go:236 where auth pre-hook errors were logged
// but not returned, causing terraform execution to continue even when authentication failed.
func TestExecuteTerraform_AuthPreHookErrorPropagation(t *testing.T) {
	defer perf.Track(nil, "exec.TestExecuteTerraform_AuthPreHookErrorPropagation")()

	// Use the existing atmos-auth fixture which has valid auth configuration.
	workDir := "../../tests/fixtures/scenarios/atmos-auth"
	t.Chdir(workDir)

	// Create a stack that references a nonexistent identity to trigger auth error.
	stacksDir := "stacks/deploy"
	stackContent := `
vars:
  stage: error-propagation-test
import:
  - catalog/mock
components:
  terraform:
    error-propagation-test:
      metadata:
        component: mock_caller_identity
      auth:
        # Reference a nonexistent identity to trigger auth error
        identity: nonexistent-identity
`
	stackFile := filepath.Join(stacksDir, "error-propagation-test.yaml")
	err := os.WriteFile(stackFile, []byte(stackContent), 0o644)
	require.NoError(t, err)

	defer os.Remove(stackFile)

	// Attempt to execute terraform - should fail during auth pre-hook.
	info := schema.ConfigAndStacksInfo{
		ComponentFromArg: "error-propagation-test",
		Stack:            "error-propagation-test",
		ComponentType:    "terraform",
		SubCommand:       "plan",
	}

	// Redirect stderr to suppress error output during test.
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	err = ExecuteTerraform(info)

	// Restore stderr.
	w.Close()
	os.Stderr = oldStderr
	var buf bytes.Buffer
	buf.ReadFrom(r)

	// Verify error was returned (not just logged and ignored).
	// The key assertion: ExecuteTerraform MUST return an error when auth pre-hook fails.
	require.Error(t, err, "ExecuteTerraform must return error when auth pre-hook fails")

	// The error should be related to authentication/provider/identity.
	errMsg := err.Error()
	hasAuthError := strings.Contains(errMsg, "identity") ||
		strings.Contains(errMsg, "auth") ||
		strings.Contains(errMsg, "provider") ||
		strings.Contains(errMsg, "credential")
	assert.True(t, hasAuthError, "Expected auth-related error, got: %v", err)
}

// TestComponentEnvSectionConversion verifies that ComponentEnvSection is properly
// converted to ComponentEnvList. This is a unit test that proves the conversion logic
// works correctly when auth hooks populate ComponentEnvSection.
//
//nolint:dupl // Test logic is intentionally similar across terraform/helmfile/packer for consistency
func TestComponentEnvSectionConversion(t *testing.T) {
	tests := []struct {
		name            string
		envSection      map[string]any
		expectedEnvList map[string]string // map for easier checking
	}{
		{
			name: "converts AWS auth environment variables",
			envSection: map[string]any{
				"AWS_CONFIG_FILE":             "/path/to/config",
				"AWS_SHARED_CREDENTIALS_FILE": "/path/to/credentials",
				"AWS_PROFILE":                 "test-profile",
				"AWS_REGION":                  "us-east-1",
				"AWS_EC2_METADATA_DISABLED":   "true",
			},
			expectedEnvList: map[string]string{
				"AWS_CONFIG_FILE":             "/path/to/config",
				"AWS_SHARED_CREDENTIALS_FILE": "/path/to/credentials",
				"AWS_PROFILE":                 "test-profile",
				"AWS_REGION":                  "us-east-1",
				"AWS_EC2_METADATA_DISABLED":   "true",
			},
		},
		{
			name:            "handles empty ComponentEnvSection",
			envSection:      map[string]any{},
			expectedEnvList: map[string]string{},
		},
		{
			name: "converts mixed types to strings",
			envSection: map[string]any{
				"STRING_VAR": "value",
				"INT_VAR":    42,
				"BOOL_VAR":   true,
			},
			expectedEnvList: map[string]string{
				"STRING_VAR": "value",
				"INT_VAR":    "42",
				"BOOL_VAR":   "true",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test ConfigAndStacksInfo with ComponentEnvSection populated.
			info := schema.ConfigAndStacksInfo{
				ComponentEnvSection: tt.envSection,
				ComponentEnvList:    []string{},
			}

			// Call the production conversion function.
			ConvertComponentEnvSectionToList(&info)

			// Verify all expected environment variables are in ComponentEnvList.
			envListMap := make(map[string]string)
			for _, envVar := range info.ComponentEnvList {
				parts := strings.SplitN(envVar, "=", 2)
				if len(parts) == 2 {
					envListMap[parts[0]] = parts[1]
				}
			}

			// Check that all expected vars are present with correct values.
			for key, expectedValue := range tt.expectedEnvList {
				actualValue, exists := envListMap[key]
				assert.True(t, exists, "Expected environment variable %s to be in ComponentEnvList", key)
				assert.Equal(t, expectedValue, actualValue,
					"Environment variable %s should have value %s, got %s", key, expectedValue, actualValue)
			}

			// Verify count matches (no extra vars).
			assert.Equal(t, len(tt.expectedEnvList), len(envListMap),
				"ComponentEnvList should contain exactly %d variables", len(tt.expectedEnvList))
		})
	}
}
