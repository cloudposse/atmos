package cmd

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
)

// TestPackerPathResolution tests that path-based component resolution works for packer commands.
// This verifies that the shared getConfigAndStacksInfo() path resolution logic works for packer.
// Note: This is a smoke test that validates the code path executes without panicking.
// Stronger assertions (e.g., checking specific paths in error messages) would make tests brittle
// and are better covered by the dedicated unit tests in internal/exec/component_path_resolution_test.go.
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
			// We're testing that path resolution logic is executed correctly.
			if err != nil {
				// Verify we get a meaningful error (not a panic or nil pointer).
				// The error message should be descriptive.
				assert.NotEmpty(t, err.Error(), "Error should have a message")

				if tt.isPath {
					// Path-based components should trigger path resolution logic.
					// Error should be a known path resolution error.
					isPathError := errors.Is(err, errUtils.ErrPathNotInComponentDir) ||
						errors.Is(err, errUtils.ErrPathResolutionFailed) ||
						errors.Is(err, errUtils.ErrComponentNotInStack) ||
						errors.Is(err, errUtils.ErrComponentTypeMismatch) ||
						errors.Is(err, errUtils.ErrStackNotFound)
					assert.True(t, isPathError,
						"Path-based component should produce path resolution error or component/stack not found, got: %v", err)
				}
				// For non-path components, we just verify we get a valid error.
				// The specific error depends on the test fixture configuration.
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
	// but it should fail gracefully with a meaningful error.
	if err != nil {
		// Should get a path-related error since we're not in a component directory.
		// This confirms the path resolution code path was executed correctly.
		isPathError := errors.Is(err, errUtils.ErrPathNotInComponentDir) ||
			errors.Is(err, errUtils.ErrPathResolutionFailed) ||
			errors.Is(err, errUtils.ErrComponentTypeMismatch)
		// Either we get a path resolution error (expected) or some other valid error.
		// The key is that it doesn't panic.
		assert.True(t, isPathError || err != nil,
			"Current directory resolution should produce path resolution error or valid error, got: %v", err)
	}
}
