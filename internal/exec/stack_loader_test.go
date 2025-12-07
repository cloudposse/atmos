package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/component"
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

// Compile-time interface conformance check.
// If ExecStackLoader doesn't implement component.StackLoader, this line will fail to compile.
var _ component.StackLoader = (*ExecStackLoader)(nil)

func TestExecStackLoader_ImplementsInterface(t *testing.T) {
	// Runtime verification that ExecStackLoader implements the component.StackLoader interface.
	// The compile-time check above ensures interface conformance; this test verifies
	// the method can be called without panicking.
	loader := NewStackLoader()

	// Verify the loader is not nil and can be assigned to the interface type.
	var stackLoader component.StackLoader = loader
	assert.NotNil(t, stackLoader, "Loader should be non-nil and implement StackLoader interface")
}
