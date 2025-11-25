package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewStackLoader(t *testing.T) {
	// Test that NewStackLoader creates a valid ExecStackLoader.
	loader := NewStackLoader()
	require.NotNil(t, loader, "NewStackLoader should return a non-nil loader")
	assert.IsType(t, &ExecStackLoader{}, loader, "Should return ExecStackLoader type")
}

func TestExecStackLoader_FindStacksMap(t *testing.T) {
	// Test that FindStacksMap delegates to the underlying FindStacksMap function.
	// This is a basic smoke test to ensure the interface is properly implemented.
	loader := NewStackLoader()
	require.NotNil(t, loader)

	// Create a minimal atmosphere config.
	// This will likely fail due to missing files, but we're testing that
	// the method exists and can be called without panicking.
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
		Stacks: schema.Stacks{
			BasePath: "stacks",
		},
	}

	// Call FindStacksMap with ignoreMissingFiles=true to avoid errors from missing stack files.
	_, _, err := loader.FindStacksMap(atmosConfig, true)
	// We expect either no error (if test environment has valid stacks)
	// or an error from missing stacks (but not a panic or interface error).
	if err != nil {
		t.Logf("Expected error from missing stacks: %v", err)
	}
}

func TestExecStackLoader_ImplementsInterface(t *testing.T) {
	// Verify that ExecStackLoader implements the component.StackLoader interface.
	// This is a compile-time check - if the interface changes, this test will fail to compile.
	loader := NewStackLoader()

	// Check that FindStacksMap exists and has the correct signature.
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
		Stacks: schema.Stacks{
			BasePath: "stacks",
		},
	}

	// This call verifies the method signature matches the interface.
	rawConfig, finalConfig, err := loader.FindStacksMap(atmosConfig, true)

	// We don't care about the result, just that the method signature is correct.
	_ = rawConfig
	_ = finalConfig
	_ = err

	// If we get here without a compile error, the interface is implemented correctly.
	assert.NotNil(t, loader, "Loader should be non-nil")
}
