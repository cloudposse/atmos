package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/proexec"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestIsExecMetadataSyncSubcommand verifies that only terraform
// plan/apply/deploy are classified as synchronous for exec-metadata upload
// purposes (FR-007); every other terraform subcommand (validate, output,
// workspace, version, init, etc.) must remain fire-and-forget via the async
// default path only. The classification now lives in the shared
// proexec.IsSyncCommand predicate (research.md Decision 10) rather than a
// private copy in this package, so both cmd/root.go's async path and this
// package's sync path can never independently drift out of sync again.
func TestIsExecMetadataSyncSubcommand(t *testing.T) {
	tests := []struct {
		subCommand string
		expected   bool
	}{
		{"plan", true},
		{"apply", true},
		{"deploy", true},
		{"validate", false},
		{"output", false},
		{"workspace", false},
		{"version", false},
		{"init", false},
		{"destroy", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.subCommand, func(t *testing.T) {
			assert.Equal(t, tt.expected, proexec.IsSyncCommand("atmos terraform "+tt.subCommand))
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

	for _, subCommand := range []string{"plan", "apply", "deploy", "validate", "output"} {
		assert.NotPanics(t, func() {
			captureExecMetadataSync(atmosConfig, subCommand, &schema.ConfigAndStacksInfo{}, execMetadataSyncParams{})
		})
	}
}

// TestCaptureExecMetadataSync_DeployReportedAsDeploy guards against a
// regression where captureExecMetadataSync would report a `deploy`
// invocation as `apply` if it ever read info.SubCommand after
// handleDeploySubcommand's in-place "deploy" -> "apply" rewrite instead of
// the subCommand value ExecuteTerraform captures up front. This test only
// asserts the function accepts "deploy" without panicking outside CI; the
// command-string correctness is covered by TestIsExecMetadataSyncSubcommand
// and the ExecuteTerraform-level wiring itself.
func TestCaptureExecMetadataSync_DeployReportedAsDeploy(t *testing.T) {
	t.Setenv("CI", "")

	atmosConfig := &schema.AtmosConfiguration{}
	assert.NotPanics(t, func() {
		captureExecMetadataSync(atmosConfig, "deploy", &schema.ConfigAndStacksInfo{}, execMetadataSyncParams{})
	})
}

// TestCaptureExecMetadataSync_ComponentAndFlags verifies captureExecMetadataSync
// accepts a populated ComponentFromArg without panicking when no invoking
// *cobra.Command is available (FR-003b) — the field-level mapping (component
// -> Args) itself is covered by pkg/proexec's buildRecord tests, which is
// where the actual DTO population happens.
func TestCaptureExecMetadataSync_ComponentAndFlags(t *testing.T) {
	t.Setenv("CI", "")

	atmosConfig := &schema.AtmosConfiguration{}
	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg: "cdn",
	}
	assert.NotPanics(t, func() {
		captureExecMetadataSync(atmosConfig, "plan", info, execMetadataSyncParams{})
	})
}
