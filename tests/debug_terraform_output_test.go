package tests

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestDebugTerraformOutput debugs the terraform.output processing.
func TestDebugTerraformOutput(t *testing.T) {
	// Create a temporary directory
	tempDir := t.TempDir()

	// Create simple component
	componentDir := filepath.Join(tempDir, "components", "terraform", "test")
	err := os.MkdirAll(componentDir, 0o755)
	assert.NoError(t, err)

	mainTf := `
variable "test" {
  type = string
  default = "default"
}

output "test" {
  value = var.test
}
`
	err = os.WriteFile(filepath.Join(componentDir, "main.tf"), []byte(mainTf), 0o644)
	assert.NoError(t, err)

	// Create stack with multiline value
	stacksDir := filepath.Join(tempDir, "stacks")
	err = os.MkdirAll(stacksDir, 0o755)
	assert.NoError(t, err)

	stackYaml := `
vars:
  stage: test

components:
  terraform:
    test-component:
      metadata:
        component: test
      vars:
        # Testing the exact format from the issue: escaped newlines in double quotes
        test: "bar\nbaz\nbongo\n"
`
	err = os.WriteFile(filepath.Join(stacksDir, "test.yaml"), []byte(stackYaml), 0o644)
	assert.NoError(t, err)

	// Create atmos.yaml
	atmosYaml := `
base_path: "./"

components:
  terraform:
    base_path: "components/terraform"

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

	// Initialize configuration
	info := schema.ConfigAndStacksInfo{
		Stack:            "test",
		ComponentFromArg: "test-component",
		ComponentType:    "terraform",
		ProcessTemplates: true,
		ProcessFunctions: true,
	}

	_, err = cfg.InitCliConfig(info, true)
	assert.NoError(t, err)

	// Describe the component to check the value
	result, err := exec.ExecuteDescribeComponent(
		"test-component",
		"test",
		true,
		true,
		nil,
	)
	assert.NoError(t, err)

	// Check the vars
	vars, ok := result["vars"].(map[string]any)
	assert.True(t, ok)

	test, ok := vars["test"].(string)
	assert.True(t, ok)

	// Debug output
	fmt.Printf("test value: %q\n", test)
	fmt.Printf("test length: %d\n", len(test))
	fmt.Printf("test bytes: %v\n", []byte(test))

	// The escaped newlines in double quotes should be converted to actual newlines
	assert.Contains(t, test, "\n", "Should contain newlines")
	assert.Equal(t, "bar\nbaz\nbongo\n", test, "Should preserve multiline format")
}
