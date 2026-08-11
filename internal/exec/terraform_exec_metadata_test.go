package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestIsExecMetadataSyncSubcommand verifies that only terraform plan/apply
// are classified as synchronous for exec-metadata upload purposes (FR-007);
// every other terraform subcommand (validate, output, workspace, version,
// init, etc.) must remain fire-and-forget via the async default path only.
func TestIsExecMetadataSyncSubcommand(t *testing.T) {
	tests := []struct {
		subCommand string
		expected   bool
	}{
		{"plan", true},
		{"apply", true},
		{"validate", false},
		{"output", false},
		{"workspace", false},
		{"version", false},
		{"init", false},
		{"deploy", false},
		{"destroy", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.subCommand, func(t *testing.T) {
			assert.Equal(t, tt.expected, isExecMetadataSyncSubcommand(tt.subCommand))
		})
	}
}

// TestCaptureExecMetadataSync_NoOpOutsideCI verifies captureExecMetadataSync
// does not panic and returns promptly for both allowlisted and
// non-allowlisted subcommands when the CI+Pro gate is closed (the common
// case for local `atmos test` runs).
func TestCaptureExecMetadataSync_NoOpOutsideCI(t *testing.T) {
	t.Setenv("CI", "")

	atmosConfig := &schema.AtmosConfiguration{}

	for _, subCommand := range []string{"plan", "apply", "validate", "output"} {
		info := &schema.ConfigAndStacksInfo{SubCommand: subCommand}
		assert.NotPanics(t, func() {
			captureExecMetadataSync(atmosConfig, info, nil)
		})
	}
}
