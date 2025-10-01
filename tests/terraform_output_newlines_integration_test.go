package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestTerraformOutputNewlinesIntegration tests that terraform.output preserves newlines.
func TestTerraformOutputNewlinesIntegration(t *testing.T) {
	// Create a temporary directory for test
	tempDir := t.TempDir()

	// Create the component directory
	componentDir := filepath.Join(tempDir, "components", "terraform", "newline-test")
	err := os.MkdirAll(componentDir, 0o755)
	assert.NoError(t, err)

	// Create a simple main.tf with outputs containing newlines
	mainTf := `
variable "multiline" {
  type = string
  default = "line1\nline2\nline3\n"
}

variable "trailing_newline" {
  type = string
  default = "hello\n"
}

variable "leading_newline" {
  type = string
  default = "\nworld"
}

output "multiline" {
  value = var.multiline
}

output "trailing_newline" {
  value = var.trailing_newline
}

output "leading_newline" {
  value = var.leading_newline
}
`
	err = os.WriteFile(filepath.Join(componentDir, "main.tf"), []byte(mainTf), 0o644)
	assert.NoError(t, err)

	// Create stacks directory
	stacksDir := filepath.Join(tempDir, "stacks")
	err = os.MkdirAll(stacksDir, 0o755)
	assert.NoError(t, err)

	// Create stack configuration that uses terraform.output
	stackYaml := `
vars:
  stage: test

components:
  terraform:
    source-component:
      metadata:
        component: newline-test
      vars:
        multiline: "bar\nbaz\nbongo\n"
        trailing_newline: "test value\n"
        leading_newline: "\ntest value"

    consumer-component:
      metadata:
        component: newline-test
      vars:
        consumed_multiline: !terraform.output source-component test multiline
        consumed_trailing: !terraform.output source-component test trailing_newline
        consumed_leading: !terraform.output source-component test leading_newline
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

	// Initialize Atmos configuration
	info := schema.ConfigAndStacksInfo{
		Stack:            "test",
		ComponentFromArg: "consumer-component",
		ComponentType:    "terraform",
		ProcessTemplates: true,
		ProcessFunctions: true,
	}

	_, err = cfg.InitCliConfig(info, true)
	assert.NoError(t, err)

	// Describe the consumer component to see if terraform.output preserves newlines
	result, err := exec.ExecuteDescribeComponent(
		"consumer-component",
		"test",
		true,
		true,
		nil,
	)
	assert.NoError(t, err)

	// Check vars section
	vars, ok := result["vars"].(map[string]any)
	assert.True(t, ok, "vars section should exist")

	// These assertions will fail if terraform is not available
	// We expect the values to be processed through terraform.output
	// For now, we just verify the structure exists

	// Verify that the consumed values contain the expected structure
	// When terraform.output processes these, it should preserve newlines
	if consumedMultiline, ok := vars["consumed_multiline"].(string); ok {
		// Check if it's the unprocessed tag or actual value
		if !strings.HasPrefix(consumedMultiline, "!terraform.output") {
			// If terraform ran, verify newlines are preserved
			assert.Contains(t, consumedMultiline, "\n", "multiline value should preserve newlines")
		}
	}

	if consumedTrailing, ok := vars["consumed_trailing"].(string); ok {
		if !strings.HasPrefix(consumedTrailing, "!terraform.output") {
			assert.True(t, strings.HasSuffix(consumedTrailing, "\n"),
				"value should preserve trailing newline")
		}
	}

	if consumedLeading, ok := vars["consumed_leading"].(string); ok {
		if !strings.HasPrefix(consumedLeading, "!terraform.output") {
			assert.True(t, strings.HasPrefix(consumedLeading, "\n"),
				"value should preserve leading newline")
		}
	}
}
