package telemetry

import (
	"bytes"
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	log "github.com/cloudposse/atmos/pkg/logger"
)

// captureTelemetryLogsAtLevel runs fn with the Atmos logger redirected to a buffer at the
// given level, restoring the previous output and level afterward. It returns everything the
// logger wrote while fn ran.
func captureTelemetryLogsAtLevel(t *testing.T, level log.Level, fn func()) string {
	t.Helper()

	var logs bytes.Buffer
	prevLevel := log.GetLevel()
	log.SetOutput(&logs)
	log.SetLevel(level)
	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
		log.SetLevel(prevLevel)
	})

	fn()
	return logs.String()
}

// missingProfileName is the nonexistent profile the tests request so config loading fails with
// ErrProfileNotFound. It is distinctive enough to appear verbatim in diagnostics.
const missingProfileName = "this-profile-does-not-exist"

// chdirToProfileLessDirWithProfileSet points ATMOS_PROFILE at a profile that does not exist
// and changes into an empty directory, reproducing the real-world scenario where a user has
// ATMOS_PROFILE exported in their shell but runs Atmos from a directory with no such profile.
// In that state cfg.InitCliConfig fails with ErrProfileNotFound.
//
// It also neutralizes every other configuration source that could make InitCliConfig fail BEFORE
// profile resolution (an invalid home/XDG config or an ATMOS_CONFIG* / ATMOS_BASE_PATH pointing at
// bad config). Isolating them guarantees the failure under test is specifically "profile not found"
// and not an unrelated config error that would make these tests pass for the wrong reason.
func chdirToProfileLessDirWithProfileSet(t *testing.T) {
	t.Helper()
	isolateTelemetryCache(t)

	// Point HOME and XDG config discovery at empty temp dirs so no user-level atmos.yaml is found.
	empty := t.TempDir()
	t.Setenv("HOME", empty)
	t.Setenv("XDG_CONFIG_HOME", empty)
	t.Setenv("XDG_CONFIG_DIRS", empty)

	// Clear any inherited explicit config selection so nothing but defaults + the missing profile
	// can contribute (empty string reads as "unset" everywhere these are consumed).
	t.Setenv("ATMOS_CLI_CONFIG_PATH", "")
	t.Setenv("ATMOS_CONFIG", "")
	t.Setenv("ATMOS_CONFIG_PATH", "")
	t.Setenv("ATMOS_BASE_PATH", "")

	// Empty working directory: no atmos.yaml, no profiles/ anywhere to resolve against.
	t.Chdir(t.TempDir())
	// A profile name that cannot be found in the empty directory.
	t.Setenv("ATMOS_PROFILE", missingProfileName)
}

// TestGetTelemetryFromConfig_ProfileNotFoundDoesNotWarn reproduces the bug from
// `ATMOS_PROFILE=<missing> atmos version`: telemetry must NOT surface a user-facing
// "Could not load config" WARN when the only problem is a missing profile. Telemetry is
// best-effort; a command that actually needs config surfaces its own enriched error.
func TestGetTelemetryFromConfig_ProfileNotFoundDoesNotWarn(t *testing.T) {
	chdirToProfileLessDirWithProfileSet(t)

	var result *Telemetry
	logs := captureTelemetryLogsAtLevel(t, log.WarnLevel, func() {
		result = getTelemetryFromConfig()
	})

	// Config could not be loaded, so telemetry degrades to nil (disabled).
	assert.Nil(t, result, "telemetry should degrade to nil when config cannot be loaded")
	// The failure must not be surfaced as a WARN to the user.
	assert.NotContains(t, logs, "Could not load config",
		"telemetry config-load failure must not surface as a user-facing WARN")
}

// TestGetTelemetryFromConfig_ProfileNotFoundLogsAtDebug proves the diagnostic is preserved
// at DEBUG level — the failure is still observable for troubleshooting, just not at WARN.
func TestGetTelemetryFromConfig_ProfileNotFoundLogsAtDebug(t *testing.T) {
	chdirToProfileLessDirWithProfileSet(t)

	var result *Telemetry
	logs := captureTelemetryLogsAtLevel(t, log.DebugLevel, func() {
		result = getTelemetryFromConfig()
	})

	require.Nil(t, result, "telemetry should degrade to nil when config cannot be loaded")
	assert.Contains(t, logs, "Could not load config",
		"the config-load failure should still be logged at DEBUG for diagnostics")
	assert.Contains(t, logs, "profile not found",
		"the failure under test must be the missing profile, not an unrelated config error")
}

// TestAtmosProWorkspaceID_ProfileNotFoundDoesNotWarn covers the second config-load site used by
// command telemetry: resolving the Atmos Pro workspace ID must also degrade quietly on a missing
// profile instead of emitting a user-facing WARN.
func TestAtmosProWorkspaceID_ProfileNotFoundDoesNotWarn(t *testing.T) {
	chdirToProfileLessDirWithProfileSet(t)

	var id string
	logs := captureTelemetryLogsAtLevel(t, log.WarnLevel, func() {
		id = atmosProWorkspaceID()
	})

	assert.Empty(t, id, "workspace ID should be empty when config cannot be loaded")
	assert.NotContains(t, logs, "Could not load config",
		"workspace-ID config-load failure must not surface as a user-facing WARN")
}

// TestAtmosProWorkspaceID_ProfileNotFoundLogsAtDebug proves the workspace-ID path still records
// the failure at DEBUG for diagnostics.
func TestAtmosProWorkspaceID_ProfileNotFoundLogsAtDebug(t *testing.T) {
	chdirToProfileLessDirWithProfileSet(t)

	var id string
	logs := captureTelemetryLogsAtLevel(t, log.DebugLevel, func() {
		id = atmosProWorkspaceID()
	})

	assert.Empty(t, id, "workspace ID should be empty when config cannot be loaded")
	assert.Contains(t, logs, "Could not load config",
		"the config-load failure should still be logged at DEBUG for diagnostics")
	assert.Contains(t, logs, "profile not found",
		"the failure under test must be the missing profile, not an unrelated config error")
}

// TestCaptureCmd_PublicAPIDisabledTelemetry exercises the public CaptureCmd entry point with
// telemetry disabled. With telemetry off, Capture() early-returns without any network call, so
// this covers the wrapper (CaptureCmd -> captureCmd -> captureCmdString) end to end. Both the
// no-error and with-error forms of the variadic argument are exercised.
func TestCaptureCmd_PublicAPIDisabledTelemetry(t *testing.T) {
	isolateTelemetryCache(t)
	// Empty working directory with telemetry disabled: no network, no profile resolution.
	t.Chdir(t.TempDir())
	t.Setenv("ATMOS_TELEMETRY_ENABLED", "false")

	cmd := &cobra.Command{Use: "version"}
	root := &cobra.Command{Use: "atmos"}
	root.AddCommand(cmd)

	// Must not panic and must not surface a user-facing WARN for either variadic form.
	logs := captureTelemetryLogsAtLevel(t, log.WarnLevel, func() {
		CaptureCmd(cmd)
		CaptureCmd(cmd, assert.AnError)
	})

	assert.NotContains(t, logs, "Could not load config",
		"disabled telemetry must not surface config warnings")
}
