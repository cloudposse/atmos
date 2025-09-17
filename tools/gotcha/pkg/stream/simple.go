package stream

import (
	"fmt"
	"strings"

	"github.com/cloudposse/atmos/tools/gotcha/internal/logger"
	"github.com/cloudposse/atmos/tools/gotcha/internal/tui"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/config"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/utils"
)

// RunSimpleStream runs tests with simple non-interactive streaming output.
func RunSimpleStream(testPackages []string, testArgs, outputFile, coverProfile, showFilter string, alert bool, verbosityLevel string) int {
	// Configure colors and initialize styles for stream mode
	profile := tui.ConfigureColors()

	// Debug: Log the detected color profile in CI
	if config.IsCI() {
		logger.GetLogger().Debug("Color profile detected", "profile", tui.ProfileName(profile), "CI", config.IsCI(), "GITHUB_ACTIONS", config.IsGitHubActions())
	}

	// Build the go test command
	args := []string{"test", "-json"}

	// Add coverage if requested
	if coverProfile != "" {
		args = append(args, fmt.Sprintf("-coverprofile=%s", coverProfile))
	}

	// Add verbose flag
	args = append(args, "-v")

	// Add timeout and other test arguments
	if testArgs != "" {
		// Parse testArgs string into individual arguments
		extraArgs := strings.Fields(testArgs)
		args = append(args, extraArgs...)
	}

	// Add packages to test
	args = append(args, testPackages...)

	// Run the tests
	exitCode := RunTestsWithSimpleStreaming(args, outputFile, showFilter, verbosityLevel)

	// Emit alert at completion
	utils.EmitAlert(alert)
	return exitCode
}
