package aws

// Package-level test setup for pkg/auth/identities/aws.
//
// This TestMain exists primarily as a Windows CI guardrail for the webflow
// tests: several tests reach browserWebflow / browserWebflowInteractive and
// rely on either telemetry.IsCI() returning true OR on individual tests
// mocking openURLFunc to avoid the real browser launch. On Windows, the
// default openURLFunc ultimately calls
// `rundll32 url.dll,FileProtocolHandler <url>` via exec.Command — which can
// hang GitHub Actions runners when no default browser is configured.
//
// Setting GO_TEST=1 here makes pkg/browser's defaultOpener.Open
// short-circuit and return nil before ever touching exec.Command, so no
// individual test can accidentally spawn a real browser process regardless
// of whether it remembered to mock openURLFunc.
//
// This follows CLAUDE.md's "cross-platform subprocess helpers in tests"
// pattern: wrap dangerous subprocess behavior behind a test-mode guard.

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// Force pkg/browser into its test short-circuit regardless of whether
	// the CI environment provides CI=true / GITHUB_ACTIONS=true. This is a
	// safety net for tests that reach browserWebflowInteractive without
	// explicitly mocking openURLFunc; on Windows CI a real browser launch
	// attempt can hang the test runner.
	//
	// os.Setenv is used here (not t.Setenv) because TestMain does not
	// receive a *testing.T, so t.Setenv is not an option. The variable is
	// set process-wide for the lifetime of the test binary, which is the
	// desired scope.
	//nolint:lintroller // TestMain has no *testing.T; os.Setenv is the only option.
	_ = os.Setenv("GO_TEST", "1")

	os.Exit(m.Run())
}
