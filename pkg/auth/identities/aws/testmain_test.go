package aws

// Package-level test setup for pkg/auth/identities/aws.
//
// This TestMain installs safe, deterministic defaults for every webflow
// package-level override variable so no test can accidentally perform a
// real browser launch, read real stdin, or construct a real bubbletea
// program. Individual tests that need specific behavior save and restore
// these variables as usual; their "original" is this safe default.
//
// Why it exists:
//   Windows GitHub Actions runners were hanging in this package for 30+
//   minutes. The root cause was that several tests could reach code paths
//   that call exec.Command("rundll32", ...) (via the default openURLFunc)
//   or bufio.Scanner.Scan(os.Stdin) (via the default stdin reader) — both
//   of which can block indefinitely on Windows and cannot be safely
//   cancelled. By neutralizing every such code path at the package level,
//   we make the test suite completely hermetic regardless of host OS.
//
// This follows CLAUDE.md's "cross-platform subprocess helpers in tests"
// pattern: wrap dangerous subprocess/stdin behavior behind a test-mode
// guard.

import (
	"fmt"
	"os"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestMain(m *testing.M) {
	// Force pkg/browser into its GO_TEST short-circuit as a belt-and-
	// suspenders safety net in case any code somehow bypasses openURLFunc.
	//nolint:lintroller // TestMain has no *testing.T; os.Setenv is the only option.
	_ = os.Setenv("GO_TEST", "1")

	// Install safe, deterministic defaults for every webflow override var.
	// Tests that need specific behavior override these and restore them
	// (the "original" they restore is THIS default, which is still safe).
	//
	// - openURLFunc: no-op so no test can spawn a real browser process.
	// - webflowIsTTYFunc: return false so no test can enter the bubbletea
	//   spinner branch by default. Tests that want the spinner override
	//   this explicitly.
	// - webflowStdinIsReadableFunc: return false so no test can start the
	//   blocking stdin scanner goroutine on real os.Stdin. Tests that
	//   exercise the stdin branch call overrideStdinReadable(t).
	// - runSpinnerProgramFunc: fail-loud so any test that accidentally
	//   reaches the real spinner path fails fast with a clear diagnostic
	//   rather than hanging or producing unpredictable output.
	openURLFunc = func(_ string) error { return nil }
	webflowIsTTYFunc = func() bool { return false }
	webflowStdinIsReadableFunc = func() bool { return false }
	runSpinnerProgramFunc = func(model webflowSpinnerModel) (tea.Model, error) {
		return model, fmt.Errorf("runSpinnerProgramFunc default reached in test: a test entered the TTY branch without overriding runSpinnerProgramFunc")
	}

	os.Exit(m.Run())
}
