package exec

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// TestHCLSyntaxErrorReturnsProperError tests that when a terraform component has
// invalid HCL syntax, the error message properly identifies it as an HCL parsing
// issue rather than returning a misleading "component not found" error.
//
// This is a regression test for https://github.com/cloudposse/atmos/issues/1864
func TestHCLSyntaxErrorReturnsProperError(t *testing.T) {
	// Change to the test cases directory where atmos.yaml is located.
	workDir := "../../tests/test-cases/invalid-hcl-syntax"
	t.Chdir(workDir)

	// Attempt to describe the component with invalid HCL.
	_, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		Component:            "testme",
		Stack:                "test",
		ProcessTemplates:     false,
		ProcessYamlFunctions: false,
		Skip:                 nil,
		AuthManager:          nil,
	})

	// The error should NOT be ErrInvalidComponent (component not found).
	// Instead, it should be ErrFailedToLoadTerraformComponent (HCL parsing error).
	require.Error(t, err, "Expected an error for invalid HCL syntax")

	// Assert that the error is about HCL/Terraform component loading, not about
	// the component being missing.
	assert.ErrorIs(t, err, errUtils.ErrFailedToLoadTerraformComponent,
		"Expected ErrFailedToLoadTerraformComponent for HCL syntax error, got: %v", err)

	// The error should NOT be about "component not found".
	assert.NotErrorIs(t, err, errUtils.ErrInvalidComponent,
		"Should NOT get ErrInvalidComponent for HCL syntax error - the component exists in the stack manifest")
}

// TestHCLSyntaxErrorContainsHCLDiagnostic tests that the error message includes
// the underlying HCL diagnostic information from terraform-config-inspect.
func TestHCLSyntaxErrorContainsHCLDiagnostic(t *testing.T) {
	workDir := "../../tests/test-cases/invalid-hcl-syntax"
	t.Chdir(workDir)

	_, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		Component:            "testme",
		Stack:                "test",
		ProcessTemplates:     false,
		ProcessYamlFunctions: false,
		Skip:                 nil,
		AuthManager:          nil,
	})

	require.Error(t, err)
	errMsg := err.Error()

	// The error should contain the HCL diagnostic from terraform-config-inspect.
	// The specific error for "]" instead of "}" is about block definition.
	assert.True(t, strings.Contains(errMsg, "block definition") ||
		strings.Contains(errMsg, "Argument") ||
		strings.Contains(errMsg, "required"),
		"Error should contain HCL diagnostic details, got: %s", errMsg)

	// The error should be wrapped with our sentinel error message.
	assert.True(t, strings.Contains(errMsg, "failed to load terraform component"),
		"Error should be wrapped with our sentinel error message, got: %s", errMsg)
}

// TestHCLSyntaxErrorFormattedOutput tests that the formatted error output
// includes helpful context like component name, file location, and hints.
// This uses the error formatter to get the full rich error output.
func TestHCLSyntaxErrorFormattedOutput(t *testing.T) {
	workDir := "../../tests/test-cases/invalid-hcl-syntax"
	t.Chdir(workDir)

	_, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		Component:            "testme",
		Stack:                "test",
		ProcessTemplates:     false,
		ProcessYamlFunctions: false,
		Skip:                 nil,
		AuthManager:          nil,
	})

	require.Error(t, err)

	// Use the error formatter to get the full rich error output.
	formattedErr := errUtils.Format(err, errUtils.DefaultFormatterConfig())

	// The formatted error should include the component name.
	assert.True(t, strings.Contains(formattedErr, "testme"),
		"Formatted error should mention the component name 'testme', got: %s", formattedErr)

	// The formatted error should include file location (main.tf).
	assert.True(t, strings.Contains(formattedErr, "main.tf"),
		"Formatted error should include file name 'main.tf', got: %s", formattedErr)

	// The formatted error should include hint about terraform validate.
	assert.True(t, strings.Contains(formattedErr, "terraform validate") ||
		strings.Contains(formattedErr, "atmos terraform validate"),
		"Formatted error should include hint about 'terraform validate', got: %s", formattedErr)
}

// TestHCLSyntaxErrorWithContext tests ExecuteDescribeComponentWithContext
// returns proper errors for HCL syntax issues.
func TestHCLSyntaxErrorWithContext(t *testing.T) {
	workDir := "../../tests/test-cases/invalid-hcl-syntax"
	t.Chdir(workDir)

	result, err := ExecuteDescribeComponentWithContext(DescribeComponentContextParams{
		AtmosConfig:          nil, // Will be initialized internally.
		Component:            "testme",
		Stack:                "test",
		ProcessTemplates:     false,
		ProcessYamlFunctions: false,
		Skip:                 nil,
		AuthManager:          nil,
	})

	require.Error(t, err, "Expected an error for invalid HCL syntax")
	assert.Nil(t, result, "Result should be nil when error occurs")

	assert.ErrorIs(t, err, errUtils.ErrFailedToLoadTerraformComponent,
		"Expected ErrFailedToLoadTerraformComponent for HCL syntax error")
	assert.NotErrorIs(t, err, errUtils.ErrInvalidComponent,
		"Should NOT get ErrInvalidComponent for HCL syntax error")
}

// TestNonExistentComponentStillReturnsInvalidComponent tests that when a component
// genuinely doesn't exist in the stack manifest, we still get ErrInvalidComponent.
// This ensures the fix for HCL errors doesn't break the "component not found" path.
func TestNonExistentComponentStillReturnsInvalidComponent(t *testing.T) {
	workDir := "../../tests/test-cases/invalid-hcl-syntax"
	t.Chdir(workDir)

	// Try to describe a component that doesn't exist in the stack manifest.
	_, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		Component:            "nonexistent-component",
		Stack:                "test",
		ProcessTemplates:     false,
		ProcessYamlFunctions: false,
		Skip:                 nil,
		AuthManager:          nil,
	})

	require.Error(t, err, "Expected an error for non-existent component")

	// This should be ErrInvalidComponent since the component doesn't exist.
	assert.ErrorIs(t, err, errUtils.ErrInvalidComponent,
		"Expected ErrInvalidComponent for non-existent component, got: %v", err)

	// It should NOT be ErrFailedToLoadTerraformComponent.
	assert.NotErrorIs(t, err, errUtils.ErrFailedToLoadTerraformComponent,
		"Should NOT get ErrFailedToLoadTerraformComponent for non-existent component")
}
