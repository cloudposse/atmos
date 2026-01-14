package cmd

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
)

// TestHelmfilePathResolution tests that path-based component resolution works for helmfile commands.
// This verifies that the shared getConfigAndStacksInfo() path resolution logic works for helmfile.
// Note: Since we don't have helmfile fixtures with components, this test verifies the path resolution
// code path executes without panicking, even when components don't exist.
func TestHelmfilePathResolution(t *testing.T) {
	_ = NewTestKit(t)
	skipIfHelmfileNotInstalled(t)

	// Use stack-templates fixture which has a valid atmos.yaml.
	stacksPath := "../tests/fixtures/scenarios/stack-templates"

	// Skip if fixture directory doesn't exist.
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
			component: "echo-server",
			stack:     "dev",
			isPath:    false,
		},
		{
			name:      "component name with slash",
			component: "apps/echo-server",
			stack:     "dev",
			isPath:    false,
		},
		{
			name:      "path-based component with dot-slash",
			component: "./components/helmfile/echo-server",
			stack:     "dev",
			isPath:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = NewTestKit(t)

			// Use RootCmd.SetArgs() and Execute() since helmfile has DisableFlagParsing=true.
			RootCmd.SetArgs([]string{"helmfile", "diff", tt.component, "-s", tt.stack})
			err := Execute()
			// The command will fail because helmfile components don't exist in test fixtures.
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

// TestHelmfilePathResolutionWithCurrentDir tests path resolution with "." (current directory).
func TestHelmfilePathResolutionWithCurrentDir(t *testing.T) {
	_ = NewTestKit(t)
	skipIfHelmfileNotInstalled(t)

	// Use stack-templates fixture which has a valid atmos.yaml.
	stacksPath := "../tests/fixtures/scenarios/stack-templates"

	// Skip if fixture directory doesn't exist.
	if _, err := os.Stat(stacksPath); os.IsNotExist(err) {
		t.Skipf("Skipping test: %s directory not found", stacksPath)
	}

	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)
	t.Setenv("ATMOS_LOGS_LEVEL", "Warning")

	// Test with "." - this should trigger path resolution logic.
	RootCmd.SetArgs([]string{"helmfile", "diff", ".", "-s", "dev"})
	err := Execute()
	// The command will fail because we're not in a helmfile component directory,
	// but it should fail gracefully with a meaningful error.
	if err != nil {
		// Should get a path-related error since we're not in a component directory.
		// This confirms the path resolution code path was executed correctly.
		isPathError := errors.Is(err, errUtils.ErrPathNotInComponentDir) ||
			errors.Is(err, errUtils.ErrPathResolutionFailed) ||
			errors.Is(err, errUtils.ErrComponentTypeMismatch)
		// Assert that we specifically get a path resolution error.
		// This validates the path resolution code path was executed.
		assert.True(t, isPathError,
			"Current directory resolution should produce a path resolution error, got: %v", err)
	}
}
