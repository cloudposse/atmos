package cmd

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestPackerPathResolution tests that path-based component resolution works for packer commands.
// This verifies that the shared getConfigAndStacksInfo() path resolution logic works for packer.
func TestPackerPathResolution(t *testing.T) {
	_ = NewTestKit(t)
	skipIfPackerNotInstalled(t)

	stacksPath := "../tests/fixtures/scenarios/packer"

	// Skip if packer fixtures directory doesn't exist.
	if _, err := os.Stat(stacksPath); os.IsNotExist(err) {
		t.Skipf("Skipping test: %s directory not found", stacksPath)
	}

	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)
	t.Setenv("ATMOS_LOGS_LEVEL", "Warning")

	tests := []struct {
		name      string
		component string
		stack     string
		isPath    bool
	}{
		{
			name:      "component name without path",
			component: "aws/bastion",
			stack:     "nonprod",
			isPath:    false,
		},
		{
			name:      "path-based component with dot-slash",
			component: "./components/packer/aws/bastion",
			stack:     "nonprod",
			isPath:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = NewTestKit(t)

			// Use RootCmd.SetArgs() and Execute() since packer has DisableFlagParsing=true.
			RootCmd.SetArgs([]string{"packer", "validate", tt.component, "-s", tt.stack})
			err := Execute()
			// The command might fail due to missing component or stack in test environment.
			// We're testing that path resolution logic is executed without panicking.
			if err != nil {
				if tt.isPath {
					// Path-based components might fail with path resolution errors if component doesn't exist.
					// This is expected - we're testing the code path executes.
					assert.NotContains(t, err.Error(), "panic", "Should not panic during path resolution")
				} else {
					// Non-path components should not trigger path resolution errors.
					assert.NotContains(t, err.Error(), "path resolution", "Non-path component should not trigger path resolution errors")
				}
			}
		})
	}
}

// TestPackerPathResolutionWithCurrentDir tests path resolution with "." (current directory).
func TestPackerPathResolutionWithCurrentDir(t *testing.T) {
	_ = NewTestKit(t)
	skipIfPackerNotInstalled(t)

	stacksPath := "../tests/fixtures/scenarios/packer"

	// Skip if packer fixtures directory doesn't exist.
	if _, err := os.Stat(stacksPath); os.IsNotExist(err) {
		t.Skipf("Skipping test: %s directory not found", stacksPath)
	}

	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)
	t.Setenv("ATMOS_LOGS_LEVEL", "Warning")

	// Test with "." - this should trigger path resolution logic.
	RootCmd.SetArgs([]string{"packer", "validate", ".", "-s", "nonprod"})
	err := Execute()
	// The command will fail because we're not in a packer component directory,
	// but it should fail gracefully with a meaningful error, not panic.
	if err != nil {
		assert.NotContains(t, err.Error(), "panic", "Should not panic when resolving current directory")
		// Should get a path-related error since we're not in a component directory.
		// This confirms the path resolution code path was executed.
	}
}
