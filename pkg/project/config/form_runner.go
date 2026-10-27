package config

import (
	"github.com/charmbracelet/huh"

	"github.com/cloudposse/atmos/pkg/perf"
)

// SetFormRunnerForTest replaces the shared scaffold questionnaire runner and returns a restore function.
// Callers can exercise real forms with accessible, scripted input without a live terminal.
func SetFormRunnerForTest(run func(*huh.Form) error) func() {
	defer perf.Track(nil, "config.SetFormRunnerForTest")()

	previous := runFormInteraction
	runFormInteraction = run
	return func() { runFormInteraction = previous }
}
