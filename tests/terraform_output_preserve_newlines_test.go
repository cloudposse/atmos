package tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestTerraformOutputPreservesNewlines verifies that the terraform.output YAML function
// correctly preserves newlines when passing values between components.
// This test addresses the issue where newlines were being stripped when using
// terraform.output to reference values from another stack (DEV-2982).
func TestTerraformOutputPreservesNewlines(t *testing.T) {
	// Skip if we don't have terraform/tofu
	// Check if tofu or terraform is available
	tofuPath := "/opt/homebrew/bin/tofu"
	terraformPath := "/usr/local/bin/terraform"

	hasTool := false
	if _, err := os.Stat(tofuPath); err == nil {
		hasTool = true
	} else if _, err := os.Stat(terraformPath); err == nil {
		hasTool = true
	} else if _, err := os.Stat("/usr/bin/terraform"); err == nil {
		hasTool = true
	}

	if !hasTool {
		t.Skipf("Skipping test: neither terraform nor tofu is available")
	}

	// Create a temporary directory
	tempDir := t.TempDir()

	// Create component directory
	componentDir := filepath.Join(tempDir, "components", "terraform", "issue-component")
	err := os.MkdirAll(componentDir, 0o755)
	assert.NoError(t, err)

	// Create main.tf exactly as described in the issue
	mainTf := `
variable "foo" {
  type = string
  default = "default"
}

output "foo" {
  value = var.foo
}
`
	err = os.WriteFile(filepath.Join(componentDir, "main.tf"), []byte(mainTf), 0o644)
	assert.NoError(t, err)

	// Create stacks directory
	stacksDir := filepath.Join(tempDir, "stacks")
	err = os.MkdirAll(stacksDir, 0o755)
	assert.NoError(t, err)

	// Stack #1 (a) - outputs value with newlines
	stackA := `
vars:
  stage: a

components:
  terraform:
    component-a:
      metadata:
        component: issue-component
      vars:
        # This is the exact value from the issue
        foo: "bar\nbaz\nbongo\n"
`
	err = os.WriteFile(filepath.Join(stacksDir, "a.yaml"), []byte(stackA), 0o644)
	assert.NoError(t, err)

	// Stack #2 (b) - consumes the value from stack a
	stackB := `
vars:
  stage: b

components:
  terraform:
    component-b:
      metadata:
        component: issue-component
      vars:
        # This uses terraform.output to get the value from stack a
        foo: !terraform.output component-a a foo
`
	err = os.WriteFile(filepath.Join(stacksDir, "b.yaml"), []byte(stackB), 0o644)
	assert.NoError(t, err)

	// Create atmos.yaml
	atmosYaml := `
base_path: "./"

components:
  terraform:
    base_path: "components/terraform"
    apply_auto_approve: true

stacks:
  base_path: "stacks"
  included_paths:
    - "*.yaml"
  name_template: "{{ .vars.stage }}"
`
	err = os.WriteFile(filepath.Join(tempDir, "atmos.yaml"), []byte(atmosYaml), 0o644)
	assert.NoError(t, err)

	// Change to temp directory
	originalDir, err := os.Getwd()
	assert.NoError(t, err)
	defer func() {
		err := os.Chdir(originalDir)
		assert.NoError(t, err)
	}()

	err = os.Chdir(tempDir)
	assert.NoError(t, err)

	// First, deploy component-a to create the output
	info := schema.ConfigAndStacksInfo{
		Stack:            "a",
		ComponentFromArg: "component-a",
		ComponentType:    "terraform",
		ProcessTemplates: true,
		ProcessFunctions: true,
		SubCommand:       "apply",
	}

	// Initialize configuration
	_, err = cfg.InitCliConfig(info, true)
	assert.NoError(t, err)

	// Apply component-a first (this will create the terraform output)
	err = exec.ExecuteTerraform(info)
	if err != nil {
		// If terraform apply fails (e.g., no terraform installed), skip this integration test
		t.Skipf("Skipping integration test: terraform apply failed: %v", err)
	}

	// Now describe component-b to verify the value is preserved with newlines
	result, err := exec.ExecuteDescribeComponent(
		"component-b",
		"b",
		true, // process templates
		true, // process yaml functions
		nil,
	)
	assert.NoError(t, err)

	// Verify the vars section
	vars, ok := result["vars"].(map[string]any)
	assert.True(t, ok, "vars section should exist")

	// Check if foo contains the value with preserved newlines
	foo, ok := vars["foo"].(string)
	assert.True(t, ok, "foo variable should exist and be a string")

	// The expected value should have newlines preserved: "bar\nbaz\nbongo\n"
	expected := "bar\nbaz\nbongo\n"
	assert.Equal(t, expected, foo,
		"The terraform.output value should preserve all newlines exactly as they were in the original output")

	// Additional assertions to be very clear about what we're testing
	assert.Contains(t, foo, "\n", "The value should contain newline characters")
	assert.Equal(t, 3, countNewlines(foo),
		"The value should contain exactly 3 newlines (after bar, baz, and bongo)")
}

func countNewlines(s string) int {
	count := 0
	for _, r := range s {
		if r == '\n' {
			count++
		}
	}
	return count
}
