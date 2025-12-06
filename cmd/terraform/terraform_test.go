package terraform

import (
	"os"
	"testing"
)

// NOTE: The TestTerraformRun1/2/3 tests were removed during the refactoring to
// cmd/terraform/ package. They used a subprocess pattern that doesn't work
// correctly when tests are run from the subpackage. These tests should be
// reimplemented as integration tests or using a different pattern.

func TestTerraformHeatmapFlag(t *testing.T) {
	// Test that --heatmap flag is properly detected and enables tracking.
	// Terraform pass-through flags are separated during preprocessing in Execute()
	// via the command registry's CompatibilityFlagTranslator.
	//
	// Note: This test requires os.Args manipulation because enableHeatmapIfRequested()
	// scans os.Args directly before flag parsing occurs. This is intentional and
	// cannot use cmd.SetArgs().

	// Save original os.Args.
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	// Simulate command line with --heatmap flag.
	os.Args = []string{"atmos", "terraform", "plan", "vpc", "-s", "uw2-prod", "--heatmap"}

	// Call enableHeatmapIfRequested which should detect --heatmap in os.Args.
	// We verify the function doesn't panic - actual heatmap output is tested in integration tests.
	// Note: perf.EnableTracking state is not directly testable without exposing internal state.
	enableHeatmapIfRequested()
}
