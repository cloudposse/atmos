package exec

import (
	"errors"
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
	// Instead, it should be ErrFailedToLoadTerraformModule (HCL parsing error).
	require.Error(t, err, "Expected an error for invalid HCL syntax")

	// Assert that the error is about HCL/Terraform module loading, not about
	// the component being missing.
	assert.True(t,
		errors.Is(err, errUtils.ErrFailedToLoadTerraformModule),
		"Expected ErrFailedToLoadTerraformModule for HCL syntax error, got: %v", err)

	// The error should NOT be about "component not found".
	assert.False(t,
		errors.Is(err, errUtils.ErrInvalidComponent),
		"Should NOT get ErrInvalidComponent for HCL syntax error - the component exists in the stack manifest")
}
